package refactor

import (
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

// Flagset allows checking for flag status
type Flagset struct {
	mtx         sync.RWMutex
	backend     Backend
	overrides   map[string]bool
	defaultTags map[string]string

	// These are immutable
	random        *rand.Rand
	maxStaleness  time.Duration
	errorHandler  ErrorHandler
	ageCallback   AgeCallback
	checkCallback CheckCallback

	// TODO: Special options for:
	// - Use statsd for checks and ages (and errors?)
	// - Use statsd for errors
	// - Use sentry for errors?
}

func defaultLogger() *log.Logger {
	return log.New(os.Stderr, "goforit error", log.LstdFlags)
}

// New creates a new Flagset
func New(backend Backend, opts ...Option) *Flagset {
	fs := &Flagset{
		overrides:   map[string]bool{},
		defaultTags: map[string]string{},
		random:      newRandom(time.Now().UnixNano()),
	}
	fs.setLogger(defaultLogger())
	for _, opt := range opts {
		opt(fs)
	}
	fs.setBackend(backend)
	return fs
}

func (fs *Flagset) setLogger(logger *log.Logger) {
	fs.errorHandler = func(err error) {
		logger.Print(err)
	}
}

func (fs *Flagset) setBackend(backend Backend) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	fs.backend = backend

	// Connect our handlers to the backend
	backend.SetErrorHandler(fs.errorHandler)
	backend.SetAgeCallback(func(at AgeType, age time.Duration) {
		fs.checkAge(at, age)
	})
}

// Close releases any resources held
func (fs *Flagset) Close() error {
	return fs.backend.Close()
}

// AddDefaultTags adds tags that will be automatically added to every call to Enabled.
// This is useful for properties of the current host or process, which never change.
func (fs *Flagset) AddDefaultTags(tags map[string]string) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	for k, v := range tags {
		fs.defaultTags[k] = v
	}
}

// Override forces the status of a flag on or off. It's mainly useful for testing.
func (fs *Flagset) Override(name string, enabled bool) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	fs.overrides[name] = enabled
}

// Enabled checks whether a flag is enabled, given a set of tags.
// Flags can potentially vary their status according to the tags.
func (fs *Flagset) Enabled(name string, tags map[string]string) bool {
	enabled, lastMod := fs.enabled(name, tags)
	if !lastMod.IsZero() {
		go fs.checkAge(AgeBackend, time.Now().Sub(lastMod))
	}
	if fs.checkCallback != nil {
		go fs.checkCallback(name, enabled)
	}
	return enabled
}

func (fs *Flagset) lockedValues(name string) (backend Backend, defaults map[string]string, hasOverride, override bool) {
	fs.mtx.RLock()
	defer fs.mtx.RUnlock()

	if override, hasOverride = fs.overrides[name]; hasOverride {
		return
	}

	backend = fs.backend
	defaults = map[string]string{}
	for k, v := range fs.defaultTags {
		defaults[k] = v
	}
	return
}

func (fs *Flagset) enabled(name string, tags map[string]string) (bool, time.Time) {
	backend, mergedTags, hasOverride, override := fs.lockedValues(name)
	if hasOverride {
		return override, time.Time{}
	}

	flag, lastMod, err := backend.Flag(name)
	if err != nil {
		go fs.errorHandler(err)
	}
	if flag == nil {
		go fs.errorHandler(ErrUnknownFlag{name})
		return false, lastMod
	}

	for k, v := range tags {
		mergedTags[k] = v
	}
	enabled, err := flag.Enabled(fs.random, mergedTags)
	if err != nil {
		go fs.errorHandler(err)
	}
	return enabled, lastMod
}

func (fs *Flagset) checkAge(at AgeType, age time.Duration) {
	if fs.maxStaleness > 0 && fs.maxStaleness < age {
		go fs.errorHandler(ErrDataStale{age, fs.maxStaleness})
	}
	if fs.ageCallback != nil {
		fs.ageCallback(at, age)
	}
}
