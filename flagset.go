package goforit

import (
	"fmt"
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
	overrides   map[string]bool // overridden flags
	defaultTags map[string]string

	// These are immutable after options are applied
	random                *rand.Rand
	maxStaleness          time.Duration
	noDefaultErrorHandler bool
	errorHandlers         []ErrorHandler
	ageCallbacks          []AgeCallback
	checkCallbacks        []CheckCallback
}

// Create a default logger
func defaultLogger() *log.Logger {
	return log.New(os.Stderr, "goforit error: ", log.LstdFlags)
}

// New creates a new Flagset
func New(backend Backend, opts ...Option) *Flagset {
	fs := &Flagset{
		overrides:   map[string]bool{},
		defaultTags: map[string]string{},
		random:      newRandom(time.Now().UnixNano()),
	}

	// Apply options
	for _, opt := range opts {
		opt(fs)
	}

	// If there's no error handler added, add a default one that just logs errors.
	if !fs.noDefaultErrorHandler && len(fs.errorHandlers) == 0 {
		fs.setLogger(defaultLogger())
	}

	// Initialize the backend
	fs.setBackend(backend)
	return fs
}

// Add an error handler to our list
func (fs *Flagset) addErrHandler(handler ErrorHandler) {
	fs.errorHandlers = append(fs.errorHandlers, handler)
}

// Set a logger as an error handler
func (fs *Flagset) setLogger(logger *log.Logger) {
	fs.addErrHandler(func(err error) {
		logger.Print(err)
	})
}

// Initialize the backend
func (fs *Flagset) setBackend(backend Backend) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	fs.backend = backend

	// Connect our handlers to the backend
	backend.SetErrorHandler(fs.handleError)
	backend.SetAgeCallback(fs.checkAge)
}

// Close releases any resources held
func (fs *Flagset) Close() error {
	return fs.backend.Close()
}

// AddDefaultTags adds tags that will be automatically added to every call to Enabled.
// This is useful for properties of the current host or process, which never change.
func (fs *Flagset) AddDefaultTags(args ...interface{}) error {
	tags, err := mergeTags(args...)
	if err != nil {
		return err
	}

	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	for k, v := range tags {
		fs.defaultTags[k] = v
	}
	return nil
}

// Override forces the status of a flag on or off. It's mainly useful for testing.
func (fs *Flagset) Override(name string, enabled bool) {
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	fs.overrides[name] = enabled
}

// Helper to create an ErrInvalidTagList
func invalidTags(f string, args ...interface{}) error {
	return ErrInvalidTagList{fmt.Sprintf(f, args...)}
}

// mergeTags takes a list of arguments, and turns them into a tag-map.
// See docs at Enabled.
func mergeTags(args ...interface{}) (map[string]string, error) {
	tags := map[string]string{}

	var key string
	for _, arg := range args {
		if key != "" {
			// Look for a value string
			if value, ok := arg.(string); ok {
				tags[key] = value
				key = ""
			} else {
				return nil, invalidTags("Key '%s' must be followed by a string value, not %T\n", key, arg)
			}
		} else {
			// Look for the start of a sequence
			switch a := arg.(type) {
			case string:
				key = a
			case map[string]string:
				for k, v := range a {
					tags[k] = v
				}
			default:
				return nil, invalidTags("Unknown tag argument %q of type %T", arg, arg)
			}
		}
	}

	if key != "" {
		return nil, invalidTags("Key '%s' must be followed by a string value, not end of list", key)
	}
	return tags, nil
}

// Enabled checks whether a flag is enabled.
// Flags can potentially vary their status according to the tags provided.
//
// To specify tags, provide either a map[string]string, or key-value pairs. You can also mix the
// two, and they'll be merged. Eg:
//
//    Enabled("myflag", map[string]string{"foo": "A", "bar": "B"})
//    Enabled("myflag", "foo", "A", "bar", "B")
//    Enabled("myflag", map[string]string{"foo": "A"}, "bar", "B", map[string]string{"iggy": C"})
//
func (fs *Flagset) Enabled(name string, args ...interface{}) bool {
	// Check the flag
	enabled, lastMod := fs.enabled(name, args...)

	// Register the new age metric
	if !lastMod.IsZero() {
		go fs.checkAge(AgeBackend, time.Now().Sub(lastMod))
	}

	// Apply check callbacks
	for _, cb := range fs.checkCallbacks {
		go cb(name, enabled)
	}
	return enabled
}

// Helper for Enabled, to fetch all the values that are lock-protected
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

// Helper for Enabled. This does all of the work, except age/check callbacks.
func (fs *Flagset) enabled(name string, args ...interface{}) (bool, time.Time) {
	// Figure out what tags were passed in
	tags, err := mergeTags(args...)
	if err != nil {
		fs.handleError(err)
		return false, time.Time{}
	}

	// Get all values protected by a lock
	backend, mergedTags, hasOverride, override := fs.lockedValues(name)
	if hasOverride {
		return override, time.Time{}
	}

	// Check if we have a Flag
	flag, lastMod, err := backend.Flag(name)
	if err != nil { // it's valid to have both an error and a flag, this is more like a warning
		fs.handleError(err)
	}
	if flag == nil {
		fs.handleError(ErrUnknownFlag{name})
		return false, lastMod
	}

	// Merge our tags with the defaults
	for k, v := range tags {
		mergedTags[k] = v
	}

	// Ask the flag if it's on or off
	enabled, err := flag.Enabled(fs.random, mergedTags)
	if err != nil {
		fs.handleError(err)
	}
	return enabled, lastMod
}

// Respond to an age metric
func (fs *Flagset) checkAge(at AgeType, age time.Duration) {
	// Check if it's stale, and error
	if fs.maxStaleness > 0 && fs.maxStaleness < age {
		fs.handleError(ErrDataStale{age, fs.maxStaleness})
	}
	// Apply callbacks
	for _, cb := range fs.ageCallbacks {
		go cb(at, age)
	}
}

// Apply error handlers
func (fs *Flagset) handleError(err error) {
	for _, handler := range fs.errorHandlers {
		go handler(err)
	}
}
