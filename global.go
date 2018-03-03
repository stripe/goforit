package goforit

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Minimum amount of time between logging that we're not initialized.
// If a program doesn't have a backend, we don't want to log on each call to Enabled,
// that would be nuts.
const defaultUninitializedInterval = time.Hour

// The global flagset
var globalMtx sync.RWMutex
var globalFlagset *Flagset
var globalLogger atomic.Value // for tests

// ErrUninitialized is used when goforit hasn't been initialized
type ErrUninitialized struct{}

func (e ErrUninitialized) Error() string {
	return "Goforit uninitialized, but feature flags are being checked"
}

// A backend that always returns an empty flag. Every so often, it alerts that we're
// not initialized.
type uninitializedBackend struct {
	BackendBase

	mtx       sync.Mutex
	interval  time.Duration
	lastError time.Time
}

func (u *uninitializedBackend) Flag(name string) (Flag, time.Time, error) {
	u.mtx.Lock()
	defer u.mtx.Unlock()

	var err error

	// Only return an error if it's been a long time
	t := time.Now()
	if t.Sub(u.lastError) > u.interval {
		err = ErrUninitialized{}
		u.lastError = t
	}

	return SampleFlag{FlagName: name, Rate: 0}, time.Time{}, err
}

// Helper to change the global Flagset. Pass nil to revert to the default
func swapGlobalFlagset(fs *Flagset) error {
	var old *Flagset

	// Swap in the new Flagset
	func() {
		globalMtx.Lock()
		defer globalMtx.Unlock()

		if fs == nil {
			// Use a backend that warns every hour.
			// Use an error handler that can swap out its logger, for testability.
			globalLogger.Store(defaultLogger())
			fs = New(&uninitializedBackend{interval: defaultUninitializedInterval}, OnError(func(err error) {
				globalLogger.Load().(*log.Logger).Print(err)
			}))
		}

		old = globalFlagset
		globalFlagset = fs
	}()

	// Make sure to close the old one
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

// Close closes the global Flagset, by reverting to the default
func Close() error {
	return swapGlobalFlagset(nil)
}

// AddDefaultTags adds tags that will be automatically added to every call to Enabled.
// This is useful for properties of the current host or process, which never change.
func AddDefaultTags(args ...interface{}) error {
	return getGlobalFlagset().AddDefaultTags(args...)
}

// Override forces the status of a flag on or off. It's mainly useful for testing.
func Override(name string, enabled bool) {
	getGlobalFlagset().Override(name, enabled)
}

// Enabled checks whether a flag is enabled, given a set of tags.
// Flags can potentially vary their status according to the tags.
func Enabled(name string, args ...interface{}) bool {
	return getGlobalFlagset().Enabled(name, args...)
}
