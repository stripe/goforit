package goforit

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/stripe/goforit/flags"
)

// fastFlags is a structure for fast access to read-mostly feature flags.
// It supports lockless reads and synchronized updates.
type fastFlags struct {
	flags atomic.Value

	writerLock sync.Mutex
}

type flagMap map[string]*flagHolder

// newFastFlags returns a new, empty fastFlags instance.
func newFastFlags() *fastFlags {
	ff := new(fastFlags)
	ff.flags.Store(make(flagMap))
	return ff
}

func (ff *fastFlags) load() flagMap {
	return ff.flags.Load().(flagMap)
}

func (ff *fastFlags) Get(key string) (flagHolder, bool) {
	if f, ok := ff.flags.Load().(flagMap)[key]; ok && f != nil {
		return *f, ok
	} else {
		return flagHolder{}, false
	}
}

func (ff *fastFlags) Update(refreshedFlags []flags.Flag, enabledTickerInterval time.Duration) {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	newHolder := func(flag flags.Flag, ticker *time.Ticker) *flagHolder {
		if ticker == nil {
			ticker = time.NewTicker(enabledTickerInterval)
		}

		return &flagHolder{
			flag:          flag,
			clamp:         flag.Clamp(),
			enabledTicker: ticker,
		}
	}

	oldFlags := ff.load()
	newFlags := make(flagMap)
	for _, flag := range refreshedFlags {
		name := flag.FlagName()
		if oldFlagHolder, ok := oldFlags[name]; ok {
			if oldFlagHolder.flag.Equal(flag) {
				newFlags[name] = oldFlagHolder
			} else {
				newFlags[name] = newHolder(flag, oldFlagHolder.enabledTicker)
			}
		} else {
			newFlags[name] = newHolder(flag, nil)
		}
	}

	// we've built the newFlags, now iterate over the list of old flags:
	// stop the ticker for any oldFlags that aren't in the new map
	for name, oldFlagHolder := range oldFlags {
		if _, found := newFlags[name]; !found {
			oldFlagHolder.enabledTicker.Stop()
		}
	}

	ff.flags.Store(newFlags)
}

func (ff *fastFlags) storeForTesting(key string, value flagHolder) {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	oldFlags := ff.load()
	newFlags := make(flagMap)
	for k, v := range oldFlags {
		newFlags[k] = v
	}

	newFlags[key] = &value

	ff.flags.Store(newFlags)
}

func (ff *fastFlags) deleteForTesting(keyToDelete string) {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	oldFlags := ff.load()
	newFlags := make(flagMap)
	for k, v := range oldFlags {
		if k != keyToDelete {
			newFlags[k] = v
		}
	}

	ff.flags.Store(newFlags)
}

func (ff *fastFlags) Close() {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	flags := ff.load()
	for _, flag := range flags {
		flag.enabledTicker.Stop()
	}
}
