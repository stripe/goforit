package goforit

import (
	"github.com/stripe/goforit/flags"
	"sync"
	"sync/atomic"
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

func (ff *fastFlags) Update(refreshedFlags []flags.Flag) {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	oldFlags := ff.load()
	newFlags := make(flagMap)
	for _, flag := range refreshedFlags {
		name := flag.FlagName()
		var holder *flagHolder
		if oldFlagHolder, ok := oldFlags[name]; ok && oldFlagHolder.flag.Equal(flag) {
			holder = oldFlagHolder
		} else {
			holder = &flagHolder{flag, flag.Clamp()}
		}
		newFlags[name] = holder
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
}
