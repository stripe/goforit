package refactor

import "time"

type AgeType string

const (
	// AgeSource indicates the duration ago that a backend's source was updated.
	// The source could be a file, network resource, etc.
	AgeSource AgeType = "age.source"

	// AgeBackend indicates the duration ago that a backend's data was updated,
	// when Enabled is called.
	// The underlying source of that data may still be stale.
	AgeBackend = "age.backend"
)

type ErrorHandler func(error)
type AgeCallback func(AgeType, time.Duration)

// Error types
type ErrUnknownFlag (string)
type ErrFlagTooOld struct {
	flag string
	age  time.Duration
}
type ErrMissingTag struct {
	flag string
	tag  string
}

// Goforit allows checking for flag status
type Goforit struct {
	// New(backend, opts...)
	// Close()
	// Enabled(name, tags)

	// OverrideFlag (for tests)

	// Each of these is also an option:

	// AddDefaultTags(tags)
	// SetErrorHandler(func(error))
	// SetCheckCallback(func(name string, status bool))
	// SetAgeCallback(func(type, time.Duration))
	// SetMaxStaleness()

	// Special options for:
	// - Use statsd for checks and ages (and errors?)
	// - Use statsd for errors
	// - Use sentry for errors?
}

// Global equivalents of above

// Some built-in backends:
// func CsvBackend(path string, refresh time.Duration) (Backend, error)
// func JsonBackend(path string, refresh time.Duration) (Backend, error)
