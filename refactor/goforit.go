package refactor

import (
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

// TODO: test all this

// Goforit allows checking for flag status
type Goforit struct {
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

// New creates a new Goforit
func New(backend Backend, opts ...Option) *Goforit {
	gi := &Goforit{
		overrides:   map[string]bool{},
		defaultTags: map[string]string{},
		random:      newRandom(time.Now().UnixNano()),
	}
	gi.setLogger(log.New(os.Stderr, "goforit error", log.LstdFlags))
	for _, opt := range opts {
		opt(gi)
	}
	gi.setBackend(backend)
	return gi
}

func (gi *Goforit) setLogger(logger *log.Logger) {
	gi.errorHandler = func(err error) {
		logger.Print(err)
	}
}

func (gi *Goforit) setBackend(backend Backend) {
	gi.mtx.Lock()
	defer gi.mtx.Unlock()

	gi.backend = backend

	// Connect our handlers to the backend
	backend.SetErrorHandler(gi.errorHandler)
	backend.SetAgeCallback(func(at AgeType, age time.Duration) {
		gi.checkAge(at, age)
	})
}

// Close releases any resources held
func (gi *Goforit) Close() error {
	return gi.backend.Close()
}

// AddDefaultTags adds tags that will be automatically added to every call to Enabled.
// This is useful for properties of the current host or process, which never change.
func (gi *Goforit) AddDefaultTags(tags map[string]string) {
	gi.mtx.Lock()
	defer gi.mtx.Unlock()

	for k, v := range tags {
		gi.defaultTags[k] = v
	}
}

// Override forces the status of a flag on or off. It's mainly useful for testing.
func (gi *Goforit) Override(name string, enabled bool) {
	gi.mtx.Lock()
	defer gi.mtx.Unlock()

	gi.overrides[name] = enabled
}

// Enabled checks whether a flag is enabled, given a set of tags.
// Flags can potentially vary their status according to the tags.
func (gi *Goforit) Enabled(name string, tags map[string]string) bool {
	enabled, lastMod := gi.enabled(name, tags)
	if !lastMod.IsZero() {
		go gi.checkAge(AgeBackend, time.Now().Sub(lastMod))
	}
	if gi.checkCallback != nil {
		go gi.checkCallback(name, enabled)
	}
	return enabled
}

func (gi *Goforit) lockedValues(name string) (backend Backend, defaults map[string]string, hasOverride, override bool) {
	gi.mtx.RLock()
	defer gi.mtx.RUnlock()

	if override, hasOverride = gi.overrides[name]; hasOverride {
		return
	}

	backend = gi.backend
	defaults = map[string]string{}
	for k, v := range gi.defaultTags {
		defaults[k] = v
	}
	return
}

func (gi *Goforit) enabled(name string, tags map[string]string) (bool, time.Time) {
	backend, mergedTags, hasOverride, override := gi.lockedValues(name)
	if hasOverride {
		return override, time.Time{}
	}

	flag, lastMod, err := backend.Flag(name)
	if err != nil {
		go gi.errorHandler(err)
	}
	if flag == nil {
		go gi.errorHandler(ErrUnknownFlag{name})
		return false, lastMod
	}

	for k, v := range tags {
		mergedTags[k] = v
	}
	enabled, err := flag.Enabled(gi.random, mergedTags)
	if err != nil {
		go gi.errorHandler(err)
	}
	return enabled, lastMod
}

func (gi *Goforit) checkAge(at AgeType, age time.Duration) {
	if gi.maxStaleness > 0 && gi.maxStaleness < age {
		go gi.errorHandler(ErrDataStale{age, gi.maxStaleness})
	}
	if gi.ageCallback != nil {
		gi.ageCallback(at, age)
	}
}
