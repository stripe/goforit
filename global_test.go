package goforit

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test that the throttled logger really is throttled
func TestGlobalThrottledLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tl := throttledLogger{
		logger:   log.New(&buf, "", log.LstdFlags),
		interval: 20 * time.Millisecond, // throttle to once every 20ms
	}

	// Attempt to log as fast as we can
	stop := time.After(200 * time.Millisecond)
LOOP:
	for {
		select {
		case <-stop:
			break LOOP
		default:
			tl.log(errors.New("testmsg"))
		}
	}

	// Should only have ~10 logs that are allowed through
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.InDelta(t, 10, len(lines), 2)
	for _, line := range lines {
		assert.Contains(t, line, "testmsg")
	}
}

// Reset the time of the global throttledLogger, so we can get new warnings without waiting an hour
func resetGlobalLoggerTime() {
	globalMtx.RLock()
	defer globalMtx.RUnlock()

	logger := globalLogger
	logger.mtx.Lock()
	defer logger.mtx.Unlock()
	logger.lastLogged = time.Time{}
}

// Capture the output of the global logger
func captureGlobalLogger() *bytes.Buffer {
	globalMtx.RLock()
	defer globalMtx.RUnlock()

	logger := globalLogger
	logger.mtx.Lock()
	defer logger.mtx.Unlock()

	buf := &bytes.Buffer{}
	logger.logger = log.New(buf, "", log.LstdFlags)
	return buf
}

// Test the behaviour of the global Flagset when uninitialized
func TestGlobalUninitialized(t *testing.T) {
	// No parallel, uses global state
	defer Close() // to reset state

	buf := captureGlobalLogger()

	// Can call enabled, and should always get false
	en := Enabled("a")
	assert.False(t, en)

	// Should get only one error per time period. These won't error.
	Enabled("b")
	Enabled("b")
	Enabled("b")
	time.Sleep(20 * time.Millisecond) // wait for goroutines

	// If we simulate another time period, we'll get another warning
	resetGlobalLoggerTime()
	Enabled("c")
	time.Sleep(20 * time.Millisecond) // wait for warnings

	// Overrides shouldn't log, even in a new time period
	resetGlobalLoggerTime()
	Override("d", true)
	Enabled("d")
	time.Sleep(20 * time.Millisecond) // wait for warnings

	// Two logged errors, from "a" and "c"
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 2, len(lines))
	for _, line := range lines {
		assert.Contains(t, line, "uninitialized")
	}
}

// Test that we can suppress errors globally
func TestGlobalSuppressErrors(t *testing.T) {
	// No parallel, changes global state
	defer Close() // to reset state

	buf := captureGlobalLogger()

	Init(&mockBackend{}, SuppressErrors())
	Enabled("a", nil)

	time.Sleep(20 * time.Millisecond) // wait for warnings
	assert.Equal(t, "", buf.String()) // get nothing
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
