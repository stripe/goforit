package goforit

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reset the warning that we're not initialized, so we can get new warnings without waiting an hour
func resetUninitializedWarning() {
	fs := getGlobalFlagset()
	backend := fs.backend.(*uninitializedBackend)
	backend.mtx.Lock()
	defer backend.mtx.Unlock()
	backend.lastError = time.Time{}
}

// Count global log messages
func countGlobalLogs(t *testing.T, block func()) int {
	r, w := io.Pipe()

	origLogger := globalLogger.Load()
	globalLogger.Store(log.New(w, "", log.LstdFlags))
	defer globalLogger.Store(origLogger)

	go func() {
		block()
		time.Sleep(20 * time.Millisecond) // wait for handler goroutines
		globalLogger.Store(origLogger)
		w.Close()
	}()

	buf, err := ioutil.ReadAll(r)
	require.NoError(t, err)
	return bytes.Count(buf, []byte("\n"))
}

// Test that we don't warn many times
func TestGlobalUninitializedOnce(t *testing.T) {
	// No parallel, uses global state
	defer Close() // to reset state

	count := countGlobalLogs(t, func() {
		// Can call enabled, and should always get false
		en := Enabled("a")
		assert.False(t, en)

		// Should get only one error per time period. These won't error.
		Enabled("b")
		Enabled("b")
		Enabled("b")
	})
	assert.Equal(t, 1, count)
}

// Test that we warn again once time has elapsed
func TestGlobalUninitializedElapsed(t *testing.T) {
	// No parallel, uses global state
	defer Close() // to reset state

	count := countGlobalLogs(t, func() {
		// Can call enabled, and should always get false
		Enabled("a")
		Enabled("a")
		Enabled("a")

		// Simulate elapsed time, and log again
		resetUninitializedWarning()
		Enabled("b")
		Enabled("b")
		Enabled("b")
	})
	assert.Equal(t, 2, count)
}

// Test that overridden flags don't warn
func TestGlobalUninitializedOverrides(t *testing.T) {
	// No parallel, uses global state
	defer Close() // to reset state

	count := countGlobalLogs(t, func() {
		Override("d", true)
		Enabled("d")
	})
	assert.Equal(t, 0, count)
}

// Test that we can suppress errors globally
func TestGlobalSuppressErrors(t *testing.T) {
	// No parallel, changes global state
	defer Close() // to reset state

	count := countGlobalLogs(t, func() {
		Init(&mockBackend{}, SuppressErrors())
		Enabled("a", nil) // flag doesn't exist, would normally error
	})
	assert.Equal(t, 0, count)
}

// Test normal use of the global Flagset, the way this library should typically be used
func TestGlobal(t *testing.T) {
	// No parallel, changes global state
	defer Close() // to reset state

	mb := &mockBackend{}
	me := &mockErrStorage{}
	mb.setFlag("a", mbFlag{value: true})
	mb.setFlag("b", mbFlag{value: false})

	// Initialize! And add some misc settings.
	Init(mb, OnError(me.set))
	Override("c", true)
	err := AddDefaultTags(map[string]string{"t": "1"})
	assert.NoError(t, err)

	// Test getting some flags
	en := Enabled("a")
	assert.True(t, en)
	assert.Equal(t, map[string]string{"t": "1"}, mb.lastTags)
	assert.NoError(t, me.get())

	en = Enabled("b", map[string]string{"u": "2"})
	assert.False(t, en)
	assert.Equal(t, map[string]string{"t": "1", "u": "2"}, mb.lastTags)
	assert.NoError(t, me.get())

	mb.lastTags = nil
	en = Enabled("c")
	assert.True(t, en)
	assert.Nil(t, mb.lastTags)
	assert.NoError(t, me.get())

	en = Enabled("d")
	assert.False(t, en)
	assert.Nil(t, mb.lastTags)
	assert.Error(t, me.get()) // errors work in the global Flagset, too
}
