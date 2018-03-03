package goforit

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/goforit/internal"
)

type mbFlag struct {
	err   error
	value bool

	name string
	be   *mockBackend
}

func (f mbFlag) Name() string {
	return f.name
}

func (f mbFlag) Enabled(rnd *rand.Rand, tags map[string]string) (bool, error) {
	f.be.lastTags = tags
	return f.value, f.err
}

type mockBackend struct {
	BackendBase

	lastMod time.Time
	err     error
	flags   map[string]Flag

	lastTags map[string]string
}

func (m *mockBackend) Flag(name string) (Flag, time.Time, error) {
	var flag Flag
	m.lastTags = nil
	if m.flags != nil {
		flag = m.flags[name]
	}
	return flag, m.lastMod, m.err
}

func (m *mockBackend) setFlag(name string, flag mbFlag) {
	flag.name = name
	flag.be = m
	if m.flags == nil {
		m.flags = map[string]Flag{}
	}
	m.flags[name] = flag
}

func TestFlagset(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("foo", mbFlag{value: true})
	mb.setFlag("bar", mbFlag{value: false})

	fs := New(mb, SuppressErrors())
	defer fs.Close()

	en := fs.Enabled("foo")
	assert.True(t, en)
	assert.Equal(t, map[string]string{}, mb.lastTags)

	en = fs.Enabled("bar", map[string]string{"a": "b"})
	assert.False(t, en)
	assert.Equal(t, map[string]string{"a": "b"}, mb.lastTags)

	en = fs.Enabled("iggy", map[string]string{})
	assert.False(t, en)
	assert.Nil(t, mb.lastTags)
}

func TestFlagsetOverrides(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})
	mb.setFlag("b", mbFlag{value: false})
	mb.setFlag("c", mbFlag{value: false})

	// Can override with options
	fs := New(mb, OverrideFlags("a", false, "d", true))
	defer fs.Close()
	assert.False(t, fs.Enabled("a"))
	assert.False(t, fs.Enabled("b"))
	assert.False(t, fs.Enabled("c"))
	assert.True(t, fs.Enabled("d")) // including things that didn't exist before

	// Can override later on, including over other overrides
	fs.Override("c", true)
	assert.True(t, fs.Enabled("c"))
	fs.Override("c", false)
	assert.False(t, fs.Enabled("c"))
}

func TestFlagsetDefaultTags(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})

	fs := New(mb, Tags(map[string]string{"cluster": "south", "hosttype": "goforit"}))
	defer fs.Close()

	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit"}, mb.lastTags)
	fs.Enabled("a", map[string]string{"user": "bob"})
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit", "user": "bob"},
		mb.lastTags)
	fs.Enabled("a", map[string]string{"user": "bob", "hosttype": "k8s"})
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "k8s", "user": "bob"},
		mb.lastTags)

	err := fs.AddDefaultTags(map[string]string{"extra": "42"})
	assert.NoError(t, err)
	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit", "extra": "42"},
		mb.lastTags)

	err = fs.AddDefaultTags("a", "1", "b", "2")
	assert.NoError(t, err)
	fs.Enabled("a")
	assert.Equal(t, map[string]string{"cluster": "south", "hosttype": "goforit", "extra": "42",
		"a": "1", "b": "2"},
		mb.lastTags)
}

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
	// Wait long enough for goroutines to be scheduled
	time.Sleep(20 * time.Millisecond)
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.err
}

func TestFlagsetUnknownFlag(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("myflag", mbFlag{value: true})

	me := &mockErrStorage{}
	fs := New(mb, OnError(me.set))
	defer fs.Close()

	en := fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())

	en = fs.Enabled("otherflag")
	assert.False(t, en)
	err := me.get()
	assert.Error(t, err)
	assert.IsType(t, ErrUnknownFlag{}, err)
	assert.Contains(t, err.Error(), "otherflag")

	en = fs.Enabled("yaflag")
	assert.False(t, en)
	err = me.get()
	assert.Error(t, err)
	assert.IsType(t, ErrUnknownFlag{}, err)
	assert.Contains(t, err.Error(), "yaflag")

	me.set(nil)
	en = fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())
}

func TestFlagsetStale(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{lastMod: time.Now().Add(-time.Hour)}
	mb.setFlag("myflag", mbFlag{value: true})

	me := &mockErrStorage{}
	fs := New(mb, OnError(me.set))
	defer fs.Close()

	// Old times are fine if we have no maxStaleness
	en := fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())

	fs.Close()
	fs = New(mb, OnError(me.set), MaxStaleness(time.Minute+2*time.Second))

	en = fs.Enabled("myflag")
	assert.True(t, en) // stale data doesn't stop flags working
	err := me.get()
	assert.Error(t, err)
	assert.IsType(t, ErrDataStale{}, err)
	assert.Contains(t, err.Error(), "1m2s")

	me.set(nil)
	mb.lastMod = time.Now()
	en = fs.Enabled("myflag")
	assert.True(t, en)
	assert.NoError(t, me.get())
}

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
	// Wait long enough for goroutines to be scheduled
	time.Sleep(20 * time.Millisecond)
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.errs
}

func TestFlagsetErrors(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{err: errors.New("errA")})
	mb.setFlag("b", mbFlag{value: true})
	mb.setFlag("c", mbFlag{err: errors.New("errC"), value: true})
	mb.setFlag("d", mbFlag{value: false})

	me := &mockMultiErrStorage{}
	fs := New(mb, OnError(me.set))
	defer fs.Close()

	en := fs.Enabled("a")
	assert.False(t, en)
	errs := me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "errA")
	me.clear()

	en = fs.Enabled("c")
	assert.True(t, en) // Can be both error and have flag on. Eg: warnings
	errs = me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "errC")
	me.clear()

	mb.err = errors.New("backendErr")
	en = fs.Enabled("b")
	assert.True(t, en)
	errs = me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "backendErr")
	me.clear()

	en = fs.Enabled("d")
	assert.False(t, en)
	errs = me.get()
	assert.Equal(t, 1, len(errs))
	assert.Error(t, errs[0])
	assert.Contains(t, errs[0].Error(), "backendErr")
	me.clear()

	// Multiple errors!
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

type enabledResult struct {
	name   string
	result bool
}

func TestFlagsetCheckCallbacks(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})
	mb.setFlag("b", mbFlag{value: false})

	var mtx sync.Mutex
	results := map[enabledResult]int{}

	fs := New(mb, OverrideFlags("c", true), SuppressErrors(), OnCheck(func(f string, e bool) {
		mtx.Lock()
		defer mtx.Unlock()
		r := enabledResult{f, e}
		results[r] += 1
	}))
	defer fs.Close()

	fs.Enabled("a")
	fs.Enabled("b")
	fs.Enabled("b")
	fs.Enabled("c")
	fs.Enabled("d")
	mb.setFlag("b", mbFlag{value: true})
	fs.Enabled("b")

	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 5, len(results))
	assert.Equal(t, 1, results[enabledResult{"a", true}])
	assert.Equal(t, 2, results[enabledResult{"b", false}])
	assert.Equal(t, 1, results[enabledResult{"b", true}])
	assert.Equal(t, 1, results[enabledResult{"c", true}])
	assert.Equal(t, 1, results[enabledResult{"d", false}])
}

func TestFlagsetAge(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{value: true})

	var mtx sync.Mutex
	ages := []time.Duration{}

	fs := New(mb, OverrideFlags("c", true), OnAge(func(ag AgeType, age time.Duration) {
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

	mb.lastMod = time.Now().Add(-10 * time.Second)
	fs.Enabled("a")
	fs.Enabled("a")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 2, len(ages))
	assert.InDelta(t, 10, ages[1].Seconds(), 2)

	mb.lastMod = time.Now()
	fs.Enabled("a")
	fs.Enabled("a")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 4, len(ages))
	assert.InDelta(t, 0, ages[3].Seconds(), 2)
}

func TestFlagsetBackendCallbacks(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{lastMod: time.Now()}
	mb.setFlag("a", mbFlag{value: true})

	me := &mockMultiErrStorage{}

	var mtx sync.Mutex
	ages := map[AgeType][]time.Duration{}

	fs := New(mb, OverrideFlags("c", true), OnError(me.set), MaxStaleness(10*time.Second),
		OnAge(func(ag AgeType, age time.Duration) {
			mtx.Lock()
			defer mtx.Unlock()
			ages[ag] = append(ages[ag], age)
		}))
	defer fs.Close()

	fs.Enabled("a")
	fs.Enabled("a")
	fs.Enabled("a")
	mb.handleError(errors.New("foo"))
	mb.handleAge(2 * time.Second)
	time.Sleep(40 * time.Millisecond)
	mb.handleAge(200 * time.Second)

	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, 3, len(ages[AgeBackend]))
	assert.Equal(t, 2, len(ages[AgeSource]))
	errs := me.get()
	assert.Equal(t, 2, len(errs))
	assert.IsType(t, ErrDataStale{}, errs[1]) // backend can trigger staleness
}

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

	internal.AtomicWriteFile(t, tmp, "myflag,0")
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, false, fs.Enabled("myflag"))

	internal.AtomicWriteFile(t, tmp, "myflag,1")
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, true, fs.Enabled("myflag"))
	assert.Equal(t, false, fs.Enabled("fakeflag"))
	time.Sleep(80 * time.Millisecond)

	s := string(buf.Bytes())
	lines := strings.Split(s, "\n")

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
			break
		}
	}
	assert.True(t, missing > 0)
	assert.True(t, parsing > 0)
	assert.Contains(t, lines[i], "fakeflag")
	i++
	assert.Equal(t, "", lines[i])
	i++
	assert.Equal(t, len(lines), i)
}

type mockRateBackend struct {
	BackendBase
}

func (m *mockRateBackend) Flag(name string) (Flag, time.Time, error) {
	return SampleFlag{name, 0.5}, time.Time{}, nil
}

func TestFlagsetSeed(t *testing.T) {
	t.Parallel()

	mb := &mockRateBackend{}
	seed := time.Now().UnixNano()
	gi1 := New(mb, Seed(seed))
	defer gi1.Close()
	gi2 := New(mb, Seed(seed))
	defer gi2.Close()
	gi3 := New(mb)
	defer gi3.Close()

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
	assert.Equal(t, 10000, match12)
	assert.InEpsilon(t, 5000, match13, 0.1)
}

func TestFlagsetMultipleHandlers(t *testing.T) {
	t.Parallel()

	mb := &mockBackend{}
	mb.setFlag("a", mbFlag{err: errors.New("errA")})

	me := &mockErrStorage{}
	me2 := &mockErrStorage{}
	fs := New(mb, OnError(me.set), OnError(me2.set))
	defer fs.Close()

	en := fs.Enabled("a")
	assert.False(t, en)
	assert.Contains(t, me.get().Error(), "errA")
	assert.Contains(t, me2.get().Error(), "errA")
}

func TestMergeTags(t *testing.T) {
	t.Parallel()

	tags, err := mergeTags()
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{}, tags)

	tags, err = mergeTags("a", "b")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b"}, tags)

	tags, err = mergeTags("a", "b", "c", "d")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	tags, err = mergeTags("a", "b", "c", "d", "a", "e")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "e", "c": "d"}, tags)

	tags, err = mergeTags(map[string]string{})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{}, tags)

	tags, err = mergeTags(map[string]string{"a": "b", "c": "d"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	tags, err = mergeTags(map[string]string{"a": "b"}, "c", "d")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	tags, err = mergeTags("a", "b", map[string]string{"c": "d"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "d"}, tags)

	tags, err = mergeTags("a", "b", map[string]string{"a": "c"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "c"}, tags)

	tags, err = mergeTags(map[string]string{"a": "b"}, map[string]string{"c": "d", "a": "e"})
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "e", "c": "d"}, tags)

	tags, err = mergeTags(
		map[string]string{"a": "b", "c": "d"},
		"e", "f",
		map[string]string{"c": "g"},
		"h", "i", "c", "k",
	)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "b", "c": "k", "e": "f", "h": "i"}, tags)

	tags, err = mergeTags("a")
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTagList{}, err)
	assert.Contains(t, err.Error(), "end of list")

	tags, err = mergeTags("a", map[string]string{"b": "c"})
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTagList{}, err)
	assert.Contains(t, err.Error(), "followed by")

	tags, err = mergeTags(1, 2)
	assert.Error(t, err)
	assert.IsType(t, ErrInvalidTagList{}, err)
	assert.Contains(t, err.Error(), "Unknown tag argument")
}
