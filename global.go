package goforit

import (
	"context"
	"time"
)

var global *Goforit

func init() {
	global = New(nil)
}

// Init initializes the flag backend, using the provided refresh function
// to update the internal cache of flags periodically, at the specified interval.
// When the Ticker returned by Init is closed, updates will stop.
func Init(interval time.Duration, backend Backend) *time.Ticker {
	global.backend = backend
	global.Init(interval)
	return global.ticker
}

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found
func Enabled(ctx context.Context, name string) (enabled bool) {
	return global.Enabled(ctx, name)
}

// RefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage.
func RefreshFlags(backend Backend) error {
	return global.RefreshFlags(backend)
}

func SetStalenessThreshold(threshold time.Duration) {
	global.SetStalenessThreshold(threshold)
}
