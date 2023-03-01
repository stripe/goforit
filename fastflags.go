package goforit

import (
	"github.com/stripe/goforit/flags2"
	"sync"
	"sync/atomic"
)

// fastFlags is a structure for fast access to read-mostly feature flags.
// It supports lockless reads and synchronized updates.
type fastFlags struct {
	flags atomic.Pointer[flagMap]

	writerLock sync.Mutex
}

type flagMap map[string]*flagHolder

// newFastFlags returns a new, empty fastFlags instance.
func newFastFlags() *fastFlags {
	return new(fastFlags)
}

func (ff *fastFlags) load() flagMap {
	if flags := ff.flags.Load(); flags != nil {
		return *flags
	}
	return nil
}

func (ff *fastFlags) Get(key string) (*flagHolder, bool) {
	if f, ok := ff.load()[key]; ok && f != nil {
		return f, ok
	} else {
		return nil, false
	}
}

func (ff *fastFlags) Update(refreshedFlags []*flags2.Flag2) {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	changed := false

	oldFlags := ff.load()
	newFlags := make(flagMap)
	for _, flag := range refreshedFlags {
		name := flag.FlagName()
		var holder *flagHolder
		if oldFlagHolder, ok := oldFlags[name]; ok && oldFlagHolder.flag.Equal(flag) {
			holder = oldFlagHolder
		} else {
			changed = true
			holder = &flagHolder{
				flag:  flag,
				clamp: flag.Clamp(),
			}
		}
		newFlags[name] = holder
	}
	if len(oldFlags) != len(newFlags) {
		changed = true
	}

	// avoid storing the new map if it is the same as the old one.
	// this is largely for tests in gocode which compare if flags
	// are deeply equal in tests.
	if changed {
		ff.flags.Store(&newFlags)
	}

	return
}

func (ff *fastFlags) storeForTesting(key string, value *flagHolder) {
	ff.writerLock.Lock()
	defer ff.writerLock.Unlock()

	oldFlags := ff.load()
	newFlags := make(flagMap)
	for k, v := range oldFlags {
		newFlags[k] = v
	}

	newFlags[key] = value

	ff.flags.Store(&newFlags)
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

	ff.flags.Store(&newFlags)
}

func (ff *fastFlags) Close() {
}
