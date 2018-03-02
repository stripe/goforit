package refactor

import "time"

// A Backend knows the current set of flags
type Backend interface {
	// Flags gets the flag with the given name.
	// Also yields the last time flags were updated (zero time if unknown)
	Flag(name string) (Flag, time.Time, error)

	// SetErrorHandler adds a handler that will be called if this backend encounters
	// errors.
	SetErrorHandler(ErrorHandler)

	// SetAgeCallback adds a handler that should be called whenever flags are updated.
	SetAgeCallback(AgeCallback)

	// Close releases any resources held by this backend
	Close() error
}

type BackendBase struct {
	errorHandler ErrorHandler
	ageCallback  AgeCallback
}

func (b *BackendBase) SetErrorHandler(h ErrorHandler) {
	b.errorHandler = h
}

func (b *BackendBase) SetAgeCallback(cb AgeCallback) {
	b.ageCallback = cb
}

func (b *BackendBase) Close() error {
	return nil
}
