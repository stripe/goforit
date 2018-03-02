package refactor

import (
	"log"
	"time"
)

// TODO: test all this

// An Option can be passed to New
type Option func(*Goforit)

// Tags specifies default tags
func Tags(tags map[string]string) Option {
	return func(gi *Goforit) {
		gi.AddDefaultTags(tags)
	}
}

// MaxStaleness specifies the maximum age of flag data, before we yield errors
func MaxStaleness(duration time.Duration) Option {
	return func(gi *Goforit) {
		gi.maxStaleness = duration
	}
}

// OnError specifies a callback for errors
func OnError(h ErrorHandler) Option {
	return func(gi *Goforit) {
		if h == nil {
			gi.errorHandler = func(error) {}
		} else {
			gi.errorHandler = h
		}
	}
}

// OnAge specifies a callback for flag data age
func OnAge(h AgeCallback) Option {
	return func(gi *Goforit) {
		gi.ageCallback = h
	}
}

// OnCheck specifies a callback for every called to Enabled
func OnCheck(h CheckCallback) Option {
	return func(gi *Goforit) {
		gi.checkCallback = h
	}
}

// Seed specifies a random number seed, for repeatable runs
func Seed(seed int64) Option {
	return func(gi *Goforit) {
		gi.random = newRandom(seed)
	}
}

// Override overrides a number of values
func Override(args ...interface{}) Option {
	if len(args)%2 != 0 {
		panic("Override takes a list of pairs")
	}
	return func(gi *Goforit) {
		var flag string
		for i, arg := range args {
			if i%2 == 0 {
				flag = arg.(string)
			} else {
				gi.overrides[flag] = arg.(bool)
			}
		}
	}
}

// LogErrors sets a logger as the error handler
func LogErrors(logger *log.Logger) Option {
	return func(gi *Goforit) {
		gi.setLogger(logger)
	}
}
