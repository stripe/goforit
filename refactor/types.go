package refactor

import (
	"io"
	"time"
)

// Error types
type ErrUnknownFlag (string)
type ErrFlagTooOld struct {
	string
	time.Duration
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
	// SetAgeCallback(func(time.Duration))
	// SetMaxStaleness()

	// Special options for:
	// - Use statsd for checks and ages (and errors?)
	// - Use statsd for errors
	// - Use sentry for errors?
}

// Global equivalents of above

// A Backend knows the current set of flags
type Backend interface {
	// GetFlag gets a flag for a name.
	// Also yields the last time it was updated (zero time if unknown)
	Flag(name string) (Flag, time.Time, error)

	// file is corrupt, file is missing, ...
	SetErrorHandler(func(error))

	Close() error
}

// A FileFormat knows how to read a file format
type FileFormat interface {
	Read(io.Reader) ([]Flag, time.Time, error)
}

type ErrFileMissing (string)
type ErrFileFormat struct {
	string
	error
}

// A FileBackend is a backend from a file.
type fileBackend struct {
	path    string
	format  FileFormat
	refresh time.Duration
	// ...
}

// Some built-in backends:
// func CsvBackend(path string, refresh time.Duration) (Backend, error)
// func JsonBackend(path string, refresh time.Duration) (Backend, error)
