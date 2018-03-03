package goforit

import (
	"sync"
	"time"
)

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

// BackendBase implements common functionality that about every backend will need
type BackendBase struct {
	handlerMtx   sync.RWMutex
	errorHandler ErrorHandler
	ageCallback  AgeCallback
}

func (b *BackendBase) SetErrorHandler(h ErrorHandler) {
	b.handlerMtx.Lock()
	defer b.handlerMtx.Unlock()
	b.errorHandler = h
}

// Handle a new error, by passing it to a callback
func (b *BackendBase) handleError(err error) error {
	b.handlerMtx.RLock()
	defer b.handlerMtx.RUnlock()
	if b.errorHandler != nil {
		go b.errorHandler(err)
	}
	return nil
}

func (b *BackendBase) SetAgeCallback(cb AgeCallback) {
	b.handlerMtx.Lock()
	defer b.handlerMtx.Unlock()
	b.ageCallback = cb
}

// Handle a new age value, by passing it to a callback
func (b *BackendBase) handleAge(age time.Duration) {
	b.handlerMtx.RLock()
	defer b.handlerMtx.RUnlock()
	if b.ageCallback != nil {
		go b.ageCallback(AgeSource, age)
	}
}

func (b *BackendBase) Close() error {
	return nil
}
