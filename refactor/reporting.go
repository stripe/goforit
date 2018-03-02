package refactor

import (
	"fmt"
	"time"
)

// ErrUnknownFlag is used when someone asks about a flag we don't know about
type ErrUnknownFlag struct {
	Flag string
}

func (e ErrUnknownFlag) Error() string {
	return fmt.Sprintf("Unknown flag: flag=%s", e.Flag)
}

// ErrDataStale is used when data hasn't been updated in too long
type ErrDataStale struct {
	LastUpdatedAge time.Duration
	MaxStaleness   time.Duration
}

func (e ErrDataStale) Error() string {
	return fmt.Sprintf("Flag data is stale: age=%v maxAge=%v", e.LastUpdatedAge, e.MaxStaleness)
}

// TODO: We don't actually use this yet
type ErrBadTags struct {
	flag string
	tag  string
}

// TODO: Error for uninitialized global goforit

// AgeType is a type of age that could be reported
type AgeType string

const (
	// AgeSource indicates the duration ago that a backend's source was updated.
	// The source could be a file, network resource, etc.
	AgeSource AgeType = "age.source"

	// AgeBackend indicates the duration ago that a backend's data was updated,
	// when Enabled is called.
	// The underlying source of that data may still be stale.
	AgeBackend AgeType = "age.backend"
)

// ErrorHandler is called when an error is encountered which should not stop the world
type ErrorHandler func(error)

// AgeCallback is called to report the age of flags.
// This lets you make sure they're still valid.
type AgeCallback func(AgeType, time.Duration)

// CheckCallback is called each time a flag is checked, so you can record how often it's enabled.
type CheckCallback func(name string, enabled bool)
