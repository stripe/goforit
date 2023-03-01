package goforit

import (
	"sync"
	"sync/atomic"
)

// fastTags is a structure for fast access to read-mostly default tags.
// It supports lockless reads and synchronized updates.
type fastTags struct {
	tags atomic.Pointer[map[string]string]

	writerLock sync.Mutex
}

func newFastTags() *fastTags {
	ft := new(fastTags)
	empty := make(map[string]string)
	ft.tags.Store(&empty)
	return ft
}

// Load returns a map of default tags.  This map MUST only be read, not written to.
func (ft *fastTags) Load() map[string]string {
	if tags := ft.tags.Load(); tags != nil {
		return *tags
	}
	return nil
}

// Set replaces the default tags.
func (ft *fastTags) Set(tags map[string]string) {
	ft.writerLock.Lock()
	defer ft.writerLock.Unlock()

	// copy argument into a new map to ensure caller can't mistakenly
	// hold on to a reference and cause a concurrent map modification panic
	newTags := make(map[string]string, len(tags))
	for k, v := range tags {
		newTags[k] = v
	}

	ft.tags.Store(&newTags)
}
