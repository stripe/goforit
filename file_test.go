package goforit

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/goforit/internal"
)

// Test a file backend's initial values, without refreshing it
func TestFileBackendInitial(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	path := filepath.Join("fixtures", "flags_example.csv")
	backend := NewFileBackend(path, CsvFileFormat{}, time.Hour)
	defer backend.Close()

	// For each flag, fetch the Flag and then the rate
	flag, lastMod, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	assert.Equal(t, "go.sun.money", flag.Name())
	sf, ok := flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 0.0, sf.Rate)
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, enabled)

	flag, lastMod, err = backend.Flag("go.moon.mercury")
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	assert.Equal(t, "go.moon.mercury", flag.Name())
	sf, ok = flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 1.0, sf.Rate)
	enabled, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, enabled)

	flag, lastMod, err = backend.Flag("go.stars.money")
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	assert.Equal(t, "go.stars.money", flag.Name())
	sf, ok = flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 0.5, sf.Rate)
	enabled, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)

	// Fake flags return no error, but also no flag.
	flag, lastMod, err = backend.Flag("fake")
	assert.NoError(t, err)
	assert.Nil(t, flag)
	assert.False(t, lastMod.IsZero())
}

// Test a file with multiple definitions of the same flag.
// The last one should take precedence.
func TestFileBackendMultipleDefinitions(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_multiple_definitions.csv")
	backend := NewFileBackend(path, CsvFileFormat{}, time.Hour)
	defer backend.Close()

	flag, _, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	sf, ok := flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 0.7, sf.Rate)
}

// A FileFormat that counts calls. It's empty, until the fifth refresh, when it
// spontaneously adds a flag.
type mockCountFormat struct {
	count int
}

func (m *mockCountFormat) Read(io.Reader) ([]Flag, time.Time, error) {
	m.count++
	flags := []Flag{}
	if m.count > 5 {
		flags = append(flags, SampleFlag{"test", 1})
	}
	return flags, time.Time{}, nil
}

// Test refreshing a FileFormat
func TestFileBackendRefresh(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	path := filepath.Join("fixtures", "flags_example.csv")
	start := time.Now()
	backend := NewFileBackend(path, &mockCountFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Initially, the flag isn't here
	flag, _, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Nil(t, flag)

	// Keep checking until the flag shows up
	checks := 0
	for {
		if time.Now().Sub(start) > 150*time.Millisecond {
			assert.FailNow(t, "waited far too long, never saw a flag")
		}

		flag, _, err = backend.Flag("test")
		assert.NoError(t, err)
		if flag != nil {
			// Found the flag! We should have waited until after five refreshes
			assert.True(t, time.Now().Sub(start) > 50*time.Millisecond)
			break
		}

		time.Sleep(5 * time.Millisecond) // pause a bit, so we don't peg CPU
		checks++
	}

	// Check the the flag looks good, and we saw five refreshes
	assert.True(t, checks > 5)
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, enabled)
}

// Test refreshing a read file backend
func TestFileBackendFileRefresh(t *testing.T) {
	t.Parallel()

	// Create a temp file, and a backend
	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	rnd := rand.New(rand.NewSource(0))
	backend := NewFileBackend(file.Name(), &CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Our flag isn't here yet
	flag, lastMod, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Nil(t, flag)
	prevMod := lastMod

	// Go sometimes writes both temp files with the same mtime. Prevent that by waiting a bit.
	time.Sleep(10 * time.Millisecond)

	// Write some flags, and wait for refresh
	internal.AtomicWriteFile(t, file, "go.sun.money,0\n")
	time.Sleep(100 * time.Millisecond)

	// Now our flag should be here
	flag, lastMod, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.True(t, lastMod.After(prevMod)) // should have read a different file from before
	require.NotNil(t, flag)
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, enabled)
	prevMod = lastMod

	// Write a new flag value, and check that it's taken effect
	internal.AtomicWriteFile(t, file, "go.sun.money,1\n")
	time.Sleep(100 * time.Millisecond)
	flag, lastMod, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.True(t, lastMod.After(prevMod))
	require.NotNil(t, flag)
	enabled, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, enabled)
}

// Test that closing a backend stops it refreshing
func TestFileBackendClose(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Fetch a flag and get the last time
	flag, lastMod, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Nil(t, flag)
	prevMod := lastMod

	// After a close, we should stop refreshing
	backend.Close()
	time.Sleep(80 * time.Millisecond) // ensure it's not currently starting to fetch
	internal.AtomicWriteFile(t, file, "go.sun.money,0\n")
	time.Sleep(80 * time.Millisecond)
	// Verify that nothing is changing
	flag, lastMod, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Equal(t, lastMod, prevMod)
	require.Nil(t, flag)
}

// Test a file going missing
func TestFileBackendMissing(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())
	internal.AtomicWriteFile(t, file, "go.sun.money,0\n")

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Save and count errors
	var mtx sync.Mutex
	var fileErr error
	var errorCount int
	backend.SetErrorHandler(func(err error) {
		mtx.Lock()
		defer mtx.Unlock()
		fileErr = err
		errorCount++
	})

	// Initially, we have a file
	time.Sleep(80 * time.Millisecond)
	func() {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Nil(t, fileErr)
	}()
	flag, _, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.NotNil(t, flag)

	// Remove the file, errors should occur
	err = os.Remove(file.Name())
	assert.NoError(t, err)
	time.Sleep(80 * time.Millisecond)
	func() {
		mtx.Lock()
		defer mtx.Unlock()
		assert.True(t, errorCount > 1)
		assert.NotNil(t, fileErr)
		assert.IsType(t, ErrFileMissing{}, fileErr)
	}()

	// Flag should still be there, errors don't overwrite with an empty flag list
	flag, _, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.NotNil(t, flag)
}

// Test file parse errors
func TestFileBackendParseError(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())
	internal.AtomicWriteFile(t, file, "go.sun.money,0\n")

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Save and count errors
	var mtx sync.Mutex
	var fileErr error
	var errorCount int
	backend.SetErrorHandler(func(err error) {
		mtx.Lock()
		defer mtx.Unlock()
		fileErr = err
		errorCount++
	})

	// Initially, we have a file
	time.Sleep(80 * time.Millisecond)
	func() {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Nil(t, fileErr)
	}()
	flag, _, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.NotNil(t, flag)

	// Break the file, errors should occur
	internal.AtomicWriteFile(t, file, "go.sun.money,foo\n")
	time.Sleep(80 * time.Millisecond)
	func() {
		mtx.Lock()
		defer mtx.Unlock()
		assert.True(t, errorCount > 1)
		assert.NotNil(t, fileErr)
		assert.IsType(t, ErrFileFormat{}, fileErr)
	}()

	// Flag should still be there
	flag, _, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.NotNil(t, flag)
}

// Test starting with a file that's missing
func TestFileBackendInitialError(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "does_not_exist.csv")
	backend := NewFileBackend(path, CsvFileFormat{}, time.Hour)
	defer backend.Close()

	var mtx sync.Mutex
	var fileErr error
	var errorCount int
	backend.SetErrorHandler(func(err error) {
		mtx.Lock()
		defer mtx.Unlock()
		fileErr = err
		errorCount++
	})

	time.Sleep(80 * time.Millisecond)
	mtx.Lock()
	defer mtx.Unlock()
	assert.NotNil(t, fileErr)
	assert.Equal(t, 1, errorCount)
}

// Test logging the age of the source data
func TestFileBackendAge(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	backend := NewFileBackend(file.Name(), &CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Each time we refresh, throw our age into a channel
	done := make(chan interface{})
	ages := make(chan time.Duration, 100)
	backend.SetAgeCallback(func(at AgeType, age time.Duration) {
		require.Equal(t, AgeSource, at)
		select {
		case <-done:
			close(ages)
		case ages <- age:
		default:
		}
	})

	// Allow some refreshes; then renew data; then refresh more
	time.Sleep(80 * time.Millisecond)
	err = os.Chtimes(file.Name(), time.Now(), time.Now()) // touch
	assert.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	close(done)

	// Look at each age we logged
	var lastAge time.Duration
	ageShrink := 0 // how many times does our age decrease?
	for a := range ages {
		if a < lastAge {
			ageShrink++
		}
		lastAge = a
	}

	// We renewed data once, so we should see one decrease in age
	assert.Equal(t, ageShrink, 1)
	// If we don't touch for awhile, times should get "big"
	assert.True(t, lastAge > 150*time.Millisecond)
}

// A FileFormat that yields a preset age
type mockAgeFormat struct {
	t time.Time
}

func (m mockAgeFormat) Read(io.Reader) ([]Flag, time.Time, error) {
	return []Flag{}, m.t, nil
}

// Test that the format can specify a last-modified time, instead of letting it
// come from the file's mtime.
func TestFileBackendFormatAge(t *testing.T) {
	t.Parallel()

	// The file's mtime is relatively modern
	path := filepath.Join("fixtures", "flags_example.csv")
	// Setup a backend that claims it's 100 years old
	lastMod := time.Now().Add(-900 * 1000 * time.Hour)
	backend := NewFileBackend(path, mockAgeFormat{lastMod}, 10*time.Millisecond)
	defer backend.Close()

	backend.SetAgeCallback(func(ag AgeType, age time.Duration) {
		// The backend's claim should override the mtime
		assert.InEpsilon(t, 900*1000*time.Hour, age, 0.1)
	})
	time.Sleep(80 * time.Millisecond) // let a refresh or two run
}

// Test what happens when we get a drastic decrease in the number of flags
func TestShrink(t *testing.T) {
	t.Parallel()

	// Write a flag file with 20 items
	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		_, err = fmt.Fprintf(file, "test.%d,%d\n", i, 0)
		require.NoError(t, err)
	}
	err = file.Sync()
	require.NoError(t, err)

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	// Save and count errors
	var mtx sync.Mutex
	var fileErr error
	var errorCount int
	backend.SetErrorHandler(func(err error) {
		mtx.Lock()
		defer mtx.Unlock()
		fileErr = err
		errorCount++
	})

	// Go down to just one flag!
	internal.AtomicWriteFile(t, file, "test.1,0")
	time.Sleep(80 * time.Millisecond)

	// We should see precisely one error, there was only one decrease
	func() {
		mtx.Lock()
		defer mtx.Unlock()
		assert.Equal(t, 1, errorCount)
		assert.NotNil(t, fileErr)
		assert.IsType(t, ErrFlagsShrunk{}, fileErr)
	}()
}
