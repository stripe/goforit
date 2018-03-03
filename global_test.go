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

func TestGlobalThrottledLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	tl := throttledLogger{
		logger:   log.New(&buf, "", log.LstdFlags),
		interval: 10 * time.Millisecond,
	}

	stop := time.After(35 * time.Millisecond)
LOOP:
	for {
		select {
		case <-stop:
			break LOOP
		default:
			tl.log(errors.New("testmsg"))
		}
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 4, len(lines))
	for _, line := range lines {
		assert.Contains(t, line, "testmsg")
	}
}

func resetGlobalLoggerTime() {
	globalMtx.RLock()
	defer globalMtx.RUnlock()

	logger := globalLogger
	logger.mtx.Lock()
	defer logger.mtx.Unlock()
	logger.lastLogged = time.Time{}
}

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

func TestGlobalUninitialized(t *testing.T) {
	// No parallel, uses global state
	defer Close() // to reset state

	buf := captureGlobalLogger()

	// Can call enabled, and get false
	en := Enabled("a", nil)
	assert.False(t, en)

	// Should get only one error per time period
	Enabled("b", nil)
	time.Sleep(20 * time.Millisecond) // wait for warnings
	resetGlobalLoggerTime()           // to prompt another warning
	Enabled("c", nil)
	time.Sleep(20 * time.Millisecond) // wait for warnings
	resetGlobalLoggerTime()           // to prompt another warning

	// Overrides shouldn't log
	Override("d", true)
	Enabled("d", nil)
	time.Sleep(20 * time.Millisecond) // wait for warnings

	// Two time periods, so two errors
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 2, len(lines))
	for _, line := range lines {
		assert.Contains(t, line, "uninitialized")
	}
}

func TestGlobalSuppressErrors(t *testing.T) {
	// No parallel, changes global state
	defer Close() // to reset state

	buf := captureGlobalLogger()

	Init(&mockBackend{}, SuppressErrors())
	Enabled("a", nil)

	time.Sleep(20 * time.Millisecond) // wait for warnings
	assert.Equal(t, "", buf.String())
}

func TestGlobal(t *testing.T) {
	// No parallel, changes global state
	defer Close() // to reset state

	mb := &mockBackend{}
	me := &mockErrStorage{}
	mb.setFlag("a", mbFlag{value: true})
	mb.setFlag("b", mbFlag{value: false})

	Init(mb, OnError(me.set))
	Override("c", true)
	AddDefaultTags(map[string]string{"t": "1"})

	en := Enabled("a", nil)
	assert.True(t, en)
	assert.Equal(t, map[string]string{"t": "1"}, mb.lastTags)
	assert.NoError(t, me.get())

	en = Enabled("b", map[string]string{"u": "2"})
	assert.False(t, en)
	assert.Equal(t, map[string]string{"t": "1", "u": "2"}, mb.lastTags)
	assert.NoError(t, me.get())

	mb.lastTags = nil
	en = Enabled("c", nil)
	assert.True(t, en)
	assert.Nil(t, mb.lastTags)
	assert.NoError(t, me.get())

	en = Enabled("d", nil)
	assert.False(t, en)
	assert.Nil(t, mb.lastTags)
	assert.Error(t, me.get())
}
