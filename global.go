package goforit

import (
	"log"
	"sync"
	"time"
)

const defaultUninitializedLog = time.Hour

var globalMtx sync.RWMutex
var globalFlagset *Flagset

// This is only here for tests
var globalLogger *throttledLogger

// A logger for messages, but throttled to once every interval.
// This way we can log that we're not initialized, but not spam all over
// the logs.
type throttledLogger struct {
	mtx        sync.Mutex
	interval   time.Duration
	logger     *log.Logger
	lastLogged time.Time
}

func (tl *throttledLogger) log(err error) {
	tl.mtx.Lock()
	defer tl.mtx.Unlock()
	if time.Now().Sub(tl.lastLogged) > tl.interval {
		tl.logger.Print(err)
		tl.lastLogged = time.Now()
	}
}

// ErrUninitialized is used when goforit hasn't been initialized
type ErrUninitialized struct{}

func (e ErrUninitialized) Error() string {
	return "Goforit uninitialized, but feature flags are being checked"
}

type uninitializedBackend struct {
	BackendBase
}

func (*uninitializedBackend) Flag(name string) (Flag, time.Time, error) {
	return SampleFlag{FlagName: name, Rate: 0}, time.Time{}, ErrUninitialized{}
}

func swapGlobalFlagset(fs *Flagset) error {
	var old *Flagset
	func() {
		globalMtx.Lock()
		defer globalMtx.Unlock()

		if fs == nil {
			// A nice default that does ~nothing, and logs every so often
			globalLogger = &throttledLogger{
				logger:   defaultLogger(),
				interval: defaultUninitializedLog,
			}
			fs = New(&uninitializedBackend{}, OnError(globalLogger.log))
		}

		old = globalFlagset
		globalFlagset = fs
	}()

	if old != nil && old != fs {
		return old.Close()
	}
	return nil
}

func getGlobalFlagset() *Flagset {
	globalMtx.RLock()
	defer globalMtx.RUnlock()
	return globalFlagset
}

func init() {
	swapGlobalFlagset(nil)
}

// Init initializes the global Flagset
func Init(backend Backend, opts ...Option) {
	fs := New(backend, opts...)
	swapGlobalFlagset(fs)
}

// Close closes the global Flagset
func Close() error {
	return swapGlobalFlagset(nil)
}

// AddDefaultTags adds tags that will be automatically added to every call to Enabled.
// This is useful for properties of the current host or process, which never change.
func AddDefaultTags(tags map[string]string) {
	getGlobalFlagset().AddDefaultTags(tags)
}

// Override forces the status of a flag on or off. It's mainly useful for testing.
func Override(name string, enabled bool) {
	getGlobalFlagset().Override(name, enabled)
}

// Enabled checks whether a flag is enabled, given a set of tags.
// Flags can potentially vary their status according to the tags.
func Enabled(name string, tags map[string]string) bool {
	return getGlobalFlagset().Enabled(name, tags)
}
