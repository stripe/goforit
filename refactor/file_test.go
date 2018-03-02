package refactor

import (
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileBackendInitial(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	path := filepath.Join("fixtures", "flags_example.csv")
	backend := NewFileBackend(path, CsvFileFormat{}, time.Hour)
	defer backend.Close()

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

	flag, lastMod, err = backend.Flag("fake")
	assert.NoError(t, err)
	assert.Nil(t, flag)
	assert.False(t, lastMod.IsZero())
}

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

func TestFileBackendRefresh(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	path := filepath.Join("fixtures", "flags_example.csv")
	start := time.Now()
	backend := NewFileBackend(path, &mockCountFormat{}, 10*time.Millisecond)
	defer backend.Close()

	flag, _, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Nil(t, flag)

	checks := 0
	for {
		if time.Now().Sub(start) > 150*time.Millisecond {
			assert.FailNow(t, "never saw a flag")
		}
		flag, _, err = backend.Flag("test")
		assert.NoError(t, err)
		if flag != nil {
			assert.True(t, time.Now().Sub(start) > 50*time.Millisecond)
			break
		}
		time.Sleep(5 * time.Millisecond)
		checks++
	}

	assert.True(t, checks > 5)
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, enabled)
}

func atomicWriteFile(t *testing.T, f *os.File, s string) {
	tmp, err := ioutil.TempFile(filepath.Dir(f.Name()), "goforit-")
	require.NoError(t, err)
	defer tmp.Close()
	defer os.Remove(tmp.Name())

	_, err = tmp.WriteString(s)
	require.NoError(t, err)
	err = tmp.Close()
	require.NoError(t, err)
	err = os.Rename(tmp.Name(), f.Name())
	require.NoError(t, err)
}

func TestFileBackendFileRefresh(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	rnd := rand.New(rand.NewSource(0))
	backend := NewFileBackend(file.Name(), &CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	flag, lastMod, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Nil(t, flag)
	prevMod := lastMod

	atomicWriteFile(t, file, "go.sun.money,0\n")
	time.Sleep(80 * time.Millisecond)
	flag, lastMod, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.True(t, lastMod.After(prevMod))
	require.NotNil(t, flag)
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, enabled)
	prevMod = lastMod

	atomicWriteFile(t, file, "go.sun.money,1\n")
	time.Sleep(80 * time.Millisecond)
	flag, lastMod, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.True(t, lastMod.After(prevMod))
	require.NotNil(t, flag)
	enabled, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, enabled)
}

func TestFileBackendClose(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

	flag, lastMod, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Nil(t, flag)
	prevMod := lastMod

	// After a close, we stop refreshing
	backend.Close()
	time.Sleep(80 * time.Millisecond)
	atomicWriteFile(t, file, "go.sun.money,0\n")
	time.Sleep(80 * time.Millisecond)
	flag, lastMod, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.Equal(t, lastMod, prevMod)
	require.Nil(t, flag)
}

func TestFileBackendMissing(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())
	atomicWriteFile(t, file, "go.sun.money,0\n")

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
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

	// Flag should still be there
	flag, _, err = backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.NotNil(t, flag)
}

func TestFileBackendParseError(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())
	atomicWriteFile(t, file, "go.sun.money,0\n")

	backend := NewFileBackend(file.Name(), CsvFileFormat{}, 10*time.Millisecond)
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
	atomicWriteFile(t, file, "go.sun.money,foo\n")
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

func TestFileBackendAge(t *testing.T) {
	t.Parallel()

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	backend := NewFileBackend(file.Name(), &CsvFileFormat{}, 10*time.Millisecond)
	defer backend.Close()

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

	time.Sleep(80 * time.Millisecond)
	err = os.Chtimes(file.Name(), time.Now(), time.Now()) // touch
	assert.NoError(t, err)
	time.Sleep(200 * time.Millisecond)
	close(done)

	var lastAge time.Duration
	ageShrink := 0
	for a := range ages {
		if a < lastAge {
			ageShrink++
		}
		lastAge = a
	}
	// Ages shrink as many times as we touch
	assert.Equal(t, ageShrink, 1)
	// If we don't touch for awhile, times should get "big"
	assert.True(t, lastAge > 80*time.Millisecond)
}

type mockAgeFormat struct {
	t time.Time
}

func (m mockAgeFormat) Read(io.Reader) ([]Flag, time.Time, error) {
	return []Flag{}, m.t, nil
}

// Test that the format can specify a last-modified time
func TestFileBackendFormatAge(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_example.csv")
	lastMod := time.Now().Add(-900 * 1000 * time.Hour) // takes priority over the file mod time
	backend := NewFileBackend(path, mockAgeFormat{lastMod}, 10*time.Millisecond)
	defer backend.Close()

	backend.SetAgeCallback(func(ag AgeType, age time.Duration) {
		assert.InEpsilon(t, 900*1000*time.Hour, age, 0.1)
	})
	time.Sleep(80 * time.Millisecond)
}
