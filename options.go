package goforit

import (
	"log"
	"time"
)

// An Option can be passed to New
type Option func(*Flagset)

// Tags specifies default tags
func Tags(tags map[string]string) Option {
	return func(fs *Flagset) {
		fs.AddDefaultTags(tags)
	}
}

// MaxStaleness specifies the maximum age of flag data, before we yield errors
func MaxStaleness(duration time.Duration) Option {
	return func(fs *Flagset) {
		fs.maxStaleness = duration
	}
}

// OnError specifies a callback for errors
// Pass nil to remove all error handlers
func OnError(h ErrorHandler) Option {
	return func(fs *Flagset) {
		fs.addErrHandler(h)
	}
}

// OnAge specifies a callback for flag data age
func OnAge(h AgeCallback) Option {
	return func(fs *Flagset) {
		fs.ageCallbacks = append(fs.ageCallbacks, h)
	}
}

// OnCheck specifies a callback for every called to Enabled
func OnCheck(h CheckCallback) Option {
	return func(fs *Flagset) {
		fs.checkCallbacks = append(fs.checkCallbacks, h)
	}
}

// Seed specifies a random number seed, for repeatable runs
func Seed(seed int64) Option {
	return func(fs *Flagset) {
		fs.random = newRandom(seed)
	}
}

// Override overrides a number of values
func OverrideFlags(args ...interface{}) Option {
	if len(args)%2 != 0 {
		panic("Override takes a list of pairs")
	}
	return func(fs *Flagset) {
		var flag string
		for i, arg := range args {
			if i%2 == 0 {
				flag = arg.(string)
			} else {
				fs.overrides[flag] = arg.(bool)
			}
		}
	}
}

// LogErrors sets a logger as the error handler
// Pass nil to use a default logger
func LogErrors(logger *log.Logger) Option {
	return func(fs *Flagset) {
		fs.setLogger(logger)
	}
}

// SuppressErrors causes errors to be hidden
func SuppressErrors() Option {
	return OnError(nil)
}
