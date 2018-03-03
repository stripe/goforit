package goforit

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/goforit/internal"
)

// A mock backend for testing
type mockBackend struct {
	BackendBase

	lastMod time.Time       // what lastMod time should we return?
	err     error           // what err should we return when fetching a flag?
	flags   map[string]Flag // hard-coded flags to return

	lastTags map[string]string // save the last tags seen by any of our flags
}

func (m *mockBackend) Flag(name string) (Flag, time.Time, error) {
	var flag Flag
	m.lastTags = nil
	if m.flags != nil {
		flag = m.flags[name]
	}
	return flag, m.lastMod, m.err
}

// A mock flag to store in mockBackend
type mbFlag struct {
	err   error // what error should Enabled return?
	value bool  // what value should Enabled return?

	name string
	be   *mockBackend // link back to mockBackend, so we can save our tags there
}

func (f mbFlag) Name() string {
	return f.name
}

func (f mbFlag) Enabled(rnd *rand.Rand, tags map[string]string) (bool, error) {
	// When called, save the tags in the mockBackend
	f.be.lastTags = tags
	return f.value, f.err
}

// Add a flag to a mockBackend
func (m *mockBackend) setFlag(name string, flag mbFlag) {
	flag.name = name
	flag.be = m // link it to the mockBackend, for saving tags
	if m.flags == nil {
		m.flags = map[string]Flag{}
	}
	m.flags[name] = flag
}

// Basic testing of a Flagset
func TestFlagset(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("foo", mbFlag{value: true})
	mb.setFlag("bar", mbFlag{value: false})

	fs := New(mb, SuppressErrors())
	defer fs.Close()

	// Flags can be on. We can have empty tags.
	en := fs.Enabled("foo")
	assert.True(t, en)
	assert.Equal(t, map[string]string{}, mb.lastTags)

	// Flags can be off. We can have non-empty tags.
	en = fs.Enabled("bar", map[string]string{"a": "b"})
	assert.False(t, en)
	assert.Equal(t, map[string]string{"a": "b"}, mb.lastTags)

	// Flags can be missing. These are never enabled.
	en = fs.Enabled("iggy", map[string]string{})
	assert.False(t, en)
	assert.Nil(t, mb.lastTags)
}

// Test overriding flags
func TestFlagsetOverrides(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})
	mb.setFlag("b", mbFlag{value: false})
	mb.setFlag("c", mbFlag{value: false})

	// Can override during initialization
	fs := New(mb, OverrideFlags("a", false, "d", true))
	defer fs.Close()
	assert.False(t, fs.Enabled("a"))
	assert.False(t, fs.Enabled("b"))
	assert.False(t, fs.Enabled("c"))
	assert.True(t, fs.Enabled("d")) // can override flags that didn't exist before

	// Can override later on. Can even stomp other overrides
	fs.Override("c", true)
	assert.True(t, fs.Enabled("c"))
	fs.Override("c", false)
	assert.False(t, fs.Enabled("c"))
}

// Test setting default tags
func TestFlagsetDefaultTags(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})

	// Can set default tags at initialization
	fs := New(mb, Tags(map[string]string{"cluster": "south", "hosttype": "goforit"}))
	defer fs.Close()

	// Default tags are added to explicit tags
	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit"}, mb.lastTags)
	fs.Enabled("a", map[string]string{"user": "bob"})
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit", "user": "bob"},
		mb.lastTags)
	// Explicit tags override default tags ('hosttype')
	fs.Enabled("a", map[string]string{"user": "bob", "hosttype": "k8s"})
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "k8s", "user": "bob"},
		mb.lastTags)

	// Adding tags can error, if we specify a bad tag-list. In this case, we're missing a value for "c".
	// When this errors, it's as if we never called it--"a": "b" is not retained.
	err := fs.AddDefaultTags("a", "b", "c")
	assert.Error(t, err)
	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit"}, mb.lastTags)

	// Can add more default tags later on. They're merged in.
	err = fs.AddDefaultTags(map[string]string{"extra": "42"})
	assert.NoError(t, err)
	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit", "extra": "42"},
		mb.lastTags)

	// Can also add default tags as key, value list, rather than map
	err = fs.AddDefaultTags("a", "1", "b", "2")
	assert.NoError(t, err)
	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit", "extra": "42",
		"a": "1", "b": "2"},
		mb.lastTags)
}

// Store an error, thread-safely
type mockErrStorage struct {
	mtx sync.Mutex
	err error
}

func (m *mockErrStorage) set(err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.err = err
}

func (m *mockErrStorage) get() error {
	// Wait long enough for error handler goroutines to run
	time.Sleep(20 * time.Millisecond)
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.err
}

// Test receiving errors when we ask for an unknown/missing flag
func TestFlagsetUnknownFlag(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("myflag", mbFlag{value: true})

	me := &mockErrStorage{}
	fs := New(mb, OnError(me.set))
	defer fs.Close()

	// This exists, all is well
	en := fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())

	// Doesn't exist! Our handler gets an error.
	en = fs.Enabled("otherflag")
	assert.False(t, en)
	err := me.get()
	assert.Error(t, err)
	assert.IsType(t, ErrUnknownFlag{}, err)
	// Error has a message.
	assert.Contains(t, err.Error(), "otherflag")

	// Different errors have different messages.
	en = fs.Enabled("yaflag")
	assert.False(t, en)
	err = me.get()
	assert.Error(t, err)
	assert.IsType(t, ErrUnknownFlag{}, err)
	assert.Contains(t, err.Error(), "yaflag")

	// We can go back to asking for flags that exist, no permanent damage done
	me.set(nil)
	en = fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())
}

// Test what happens when flag data gets stale
func TestFlagsetStale(t *testing.T) {
	t.Parallel()

	// Make a backend that claims to be super stale
	mb := &mockBackend{lastMod: time.Now().Add(-time.Hour)}
	mb.setFlag("myflag", mbFlag{value: true})

	me := &mockErrStorage{}
	fs := New(mb, OnError(me.set))
	defer fs.Close()

	// We never set max-staleness, so this is fine. There's no reasonable default for
	// max-staleness, it's perfectly plausible for this to be operator-directed.
	en := fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())

	// Let's try again with max-staleness on.
	fs.Close()
	fs = New(mb, OnError(me.set), MaxStaleness(time.Minute+2*time.Second))

	// Check a flag...
	en = fs.Enabled("myflag")
	assert.True(t, en) // Even though data is stale, existing flags keep working
	err := me.get()
	// Aha! Data is stale
	assert.Error(t, err)
	assert.IsType(t, ErrDataStale{}, err)
	assert.Contains(t, err.Error(), "1m2s") // we see max-staleness in the message

	// The backend claims to be updated
	mb.lastMod = time.Now()
	me.set(nil)
	en = fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get()) // now no more error, data is recent
}

// Store multiple errors, thread-safely
type mockMultiErrStorage struct {
	mtx  sync.Mutex
	errs []error
}

func (m *mockMultiErrStorage) set(err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.errs = append(m.errs, err)
}

func (m *mockMultiErrStorage) clear() {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.errs = []error{}
}

func (m *mockMultiErrStorage) get() []error {
	// Wait long enough for error handler goroutines to run
	time.Sleep(20 * time.Millisecond)
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.errs
}

// Test a variety of error types from the backend
func TestFlagsetErrors(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{err: errors.New("errA")}) // a flag that errors
	mb.setFlag("b", mbFlag{value: true})
	// A flag that's enabled, but still errors. Eg: some sort of in-band refresh failed,
	// but the last value was true. This is more like a warning than a true error.
	mb.setFlag("c", mbFlag{err: errors.New("errC"), value: true})
	mb.setFlag("d", mbFlag{value: false})

	me := &mockMultiErrStorage{}
	fs := New(mb, OnError(me.set))
	defer fs.Close()

	// Check a flag that errors
	en := fs.Enabled("a")
	assert.False(t, en)
	errs := me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "errA")
	me.clear()

	// A flag can both error, and be enabled
	en = fs.Enabled("c")
	assert.True(t, en)
	errs = me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "errC")
	me.clear()

	// Set the backend to error when we fetch the flag, before asking if it's enabled.
	mb.err = errors.New("backendErr")
	en = fs.Enabled("b")
	assert.True(t, en) // again, error + enabled is an ok combination
	errs = me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "backendErr")
	me.clear()

	// Error + disabled is ok too
	en = fs.Enabled("d")
	assert.False(t, en)
	errs = me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "backendErr")
	me.clear()

	// Multiple errors from a single call. Flag is both unknown, and backend yields an error.
	en = fs.Enabled("e")
	assert.False(t, en)
	errs = me.get()
	assert.Equal(t, 2, len(errs))
	assert.Error(t, errs[0])
	assert.Error(t, errs[1])
	if _, ok := errs[0].(ErrUnknownFlag); ok {
		assert.Contains(t, errs[1].Error(), "backendErr")
	} else {
		assert.Contains(t, errs[0].Error(), "backendErr")
		assert.IsType(t, ErrUnknownFlag{}, errs[1])
	}
}

// A log entry of the result of calling Enabled()
type enabledLog struct {
	name   string
	result bool
}

// Test the callback on each check (aka Enabled() call)
func TestFlagsetCheckCallbacks(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})
	mb.setFlag("b", mbFlag{value: false})

	var mtx sync.Mutex
	results := map[enabledLog]int{}

	fs := New(mb, OverrideFlags("c", true), SuppressErrors(), OnCheck(func(f string, e bool) {
		// Save results in a hash
		mtx.Lock()
		defer mtx.Unlock()
		r := enabledLog{f, e}
		results[r] += 1
	}))
	defer fs.Close()

	// Do a bunch of checks
	fs.Enabled("a")
	fs.Enabled("b")
	fs.Enabled("b")
	fs.Enabled("c")
	fs.Enabled("d")
	mb.setFlag("b", mbFlag{value: true}) // Change a flag value in the middle
	fs.Enabled("b")

	// Wait for handler goroutines, and see if results are as expected
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 5, len(results))
	assert.Equal(t, 1, results[enabledLog{"a", true}])
	assert.Equal(t, 2, results[enabledLog{"b", false}])
	assert.Equal(t, 1, results[enabledLog{"b", true}])
	assert.Equal(t, 1, results[enabledLog{"c", true}])
	assert.Equal(t, 1, results[enabledLog{"d", false}])
}

// Test the age callback
func TestFlagsetAge(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})

	var mtx sync.Mutex
	ages := []time.Duration{}

	fs := New(mb, OverrideFlags("c", true), OnAge(func(ag AgeType, age time.Duration) {
		// Record all the ages we see
		mtx.Lock()
		defer mtx.Unlock()
		assert.Equal(t, AgeBackend, ag)
		ages = append(ages, age)
	}))
	defer fs.Close()

	// When lastMod is zero, no ages recorded
	fs.Enabled("a")
	time.Sleep(20 * time.Millisecond)
	assert.Empty(t, ages)

	// When backend yields a real lastMod, we log ages
	mb.lastMod = time.Now().Add(-10 * time.Second)
	fs.Enabled("a")
	fs.Enabled("a")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 2, len(ages))
	assert.InDelta(t, 10, ages[1].Seconds(), 2) // ages are ~10s

	// When the backend yields a more recent lastMod, ages also get recent
	mb.lastMod = time.Now()
	fs.Enabled("a")
	fs.Enabled("a")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 4, len(ages))
	assert.InDelta(t, 0, ages[3].Seconds(), 2) // ages are ~zero
}

// Test the callbacks that link the backend to the Flagset
func TestFlagsetBackendCallbacks(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{lastMod: time.Now()}
	mb.setFlag("a", mbFlag{value: true})

	me := &mockMultiErrStorage{}

	// Log ages, by category
	var mtx sync.Mutex
	ages := map[AgeType][]time.Duration{}

	fs := New(mb, OverrideFlags("c", true), OnError(me.set), MaxStaleness(10*time.Second),
		OnAge(func(ag AgeType, age time.Duration) {
			mtx.Lock()
			defer mtx.Unlock()
			ages[ag] = append(ages[ag], age)
		}))
	defer fs.Close()

	// Each time we call enabled, it'll log an AgeBackend
	fs.Enabled("a")
	fs.Enabled("a")
	fs.Enabled("a")

	// When we trigger callbacks on the backend, it'll trigger things on the Flagset
	mb.handleError(errors.New("foo"))
	mb.handleAge(2 * time.Second)     // logs an AgeSource
	time.Sleep(40 * time.Millisecond) // wait for goroutines to finish, so these are in order
	mb.handleAge(200 * time.Second)   // logs an AgeSource

	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, 3, len(ages[AgeBackend])) // we have 3 AgeBackend, from Enabled
	assert.Equal(t, 2, len(ages[AgeSource]))  // we have 2 AgeSource, from handleAge
	errs := me.get()
	assert.Equal(t, 2, len(errs))             // two errors: backend error + staleness error
	assert.IsType(t, ErrDataStale{}, errs[1]) // AgeSource on the backend triggered staleness error
}

// Test that we can add a logger to a Flagset
func TestFlagsetLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	mb := &mockBackend{}
	fs := New(mb, LogErrors(log.New(&buf, "myprefix", log.LstdFlags)))
	defer fs.Close()

	fs.Enabled("fakeflag")
	time.Sleep(80 * time.Millisecond)
	s := string(buf.Bytes())

	assert.Contains(t, s, "myprefix")
	assert.Contains(t, s, "fakeflag")
}

// Test a realistic Flagset, with a refreshing CSV backend
func TestFlagsetEndToEnd(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := log.New(&buf, "myprefix", log.LstdFlags)

	tmp, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	os.Remove(tmp.Name())
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	backend := NewCsvBackend(tmp.Name(), 10*time.Millisecond)
	fs := New(backend, LogErrors(logger))
	defer fs.Close()
	// No file yet, we should get file-missing errors

	internal.AtomicWriteFile(t, tmp, "myflag,XXX")
	time.Sleep(80 * time.Millisecond)
	// Now we should get parse-failure messages

	// Write CSV data, and read a flag
	internal.AtomicWriteFile(t, tmp, "myflag,0")
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, false, fs.Enabled("myflag"))

	// Change CSV data, and read a flag
	internal.AtomicWriteFile(t, tmp, "myflag,1")
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, true, fs.Enabled("myflag"))
	// Read a flag that doesn't exist
	assert.Equal(t, false, fs.Enabled("fakeflag"))
	time.Sleep(80 * time.Millisecond) // wait for goroutines to finish

	// Read log to see what happened
	s := strings.TrimRight(buf.String(), "\n")
	lines := strings.Split(s, "\n")
	// Count parse errors and file-missing errors
	missing := 0
	parsing := 0
	var i int
	for i = 0; i < len(lines); i++ {
		line := strings.ToLower(lines[i])
		if strings.Contains(line, "missing") {
			missing++
		} else if strings.Contains(line, "pars") {
			parsing++
		} else {
			break // stop when there are no more file errors
		}
	}
	// We should have at least one of each
	assert.True(t, missing > 0)
	assert.True(t, parsing > 0)

	// After file errors, should be a single line with an error about our misisng flag
	assert.Contains(t, lines[i], "fakeflag")
	i++
	assert.Equal(t, len(lines), i)
}

// A backend that always returns a 50% rate
type mockRateBackend struct {
	BackendBase
}

func (m *mockRateBackend) Flag(name string) (Flag, time.Time, error) {
	return SampleFlag{name, 0.5}, time.Time{}, nil
}

// Check that we can set a RNG seed
func TestFlagsetSeed(t *testing.T) {
	t.Parallel()

	// Two backends have the same seed, a third is different
	mb := &mockRateBackend{}
	seed := time.Now().UnixNano()
	gi1 := New(mb, Seed(seed))
	defer gi1.Close()
	gi2 := New(mb, Seed(seed))
	defer gi2.Close()
	gi3 := New(mb)
	defer gi3.Close()

	// Check how many times 1 & 2 match, and how many times 1 & 3 match
	match12 := 0
	match13 := 0
	for i := 0; i < 10000; i++ {
		e1 := gi1.Enabled("a")
		e2 := gi2.Enabled("a")
		e3 := gi3.Enabled("a")
		if e1 == e2 {
			match12++
		}
		if e1 == e3 {
			match13++
		}
	}
	assert.Equal(t, 10000, match12)         // same seed, should match all the time
	assert.InEpsilon(t, 5000, match13, 0.1) // different seed, should match half the time
}

// Test using multiple of the same sort of handler
func TestFlagsetMultipleHandlers(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{err: errors.New("errA")})

	// Add two error handlers
	me := &mockErrStorage{}
	me2 := &mockErrStorage{}
	fs := New(mb, OnError(me.set), OnError(me2.set))
	defer fs.Close()

	// One call for an unknown flag. Both handlers get called
	en := fs.Enabled("a")
	assert.False(t, en)
	assert.Contains(t, me.get().Error(), "errA")
	assert.Contains(t, me2.get().Error(), "errA")
}

// Test our tag-merging system
func TestMergeTags(t *testing.T) {
	t.Parallel()

	// Empty is fine
	tags, err := mergeTags()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{}, tags)

	// Can use list of keys and values
	tags, err = mergeTags("a", "b")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b"}, tags)

	// ...even long lists
	tags, err = mergeTags("a", "b", "c", "d")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	// Later value of "a" overrides earlier one
	tags, err = mergeTags("a", "b", "c", "d", "a", "e")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "e", "c": "d"}, tags)

	// Can use maps
	tags, err = mergeTags(map[string]string{})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{}, tags)

	// ...even long maps
	tags, err = mergeTags(map[string]string{"a": "b", "c": "d"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	// Can combine maps and lists
	tags, err = mergeTags(map[string]string{"a": "b"}, "c", "d")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	// ...in any order!
	tags, err = mergeTags("a", "b", map[string]string{"c": "d"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	// Later values override earlier ones, even if they're different types of args
	tags, err = mergeTags("a", "b", map[string]string{"a": "c"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "c"}, tags)

	// Can use multiple maps
	tags, err = mergeTags(map[string]string{"a": "b"}, map[string]string{"c": "d", "a": "e"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "e", "c": "d"}, tags)

	// Can even use multiple maps and lists
	tags, err = mergeTags(
		map[string]string{"a": "b", "c": "d"},
		"e", "f",
		map[string]string{"c": "g"},
		"h", "i", "c", "k",
	)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "k", "e": "f", "h": "i"}, tags)

	// A single key is useless without a value
	tags, err = mergeTags("a")
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTagList{}, err)
	assert.Contains(t, err.Error(), "end of list")

	// Still a single key, even if a map follows
	tags, err = mergeTags("a", map[string]string{"b": "c"})
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTagList{}, err)
	assert.Contains(t, err.Error(), "followed by")

	// Must be strings!
	tags, err = mergeTags(1, 2)
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTagList{}, err)
	assert.Contains(t, err.Error(), "Unknown tag argument")
}

// Benchmark calling Enabled, in a realistic setting
func BenchmarkFlagsetEnabled(b *testing.B) {
	path := filepath.Join("fixtures", "flags_example.csv")
	// Refresh frequently so that we're testing interference
	backend := NewCsvBackend(path, 50*time.Millisecond)
	fs := New(backend)

	for i := 0; i < b.N; i++ {
		en := fs.Enabled("go.moon.mercury", "tag", "value")
		assert.True(b, en)
	}
}
