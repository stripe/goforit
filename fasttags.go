package goforit

import (
	"sync"
	"sync/atomic"
)

// fastTags is a structure for fast access to read-mostly default tags.
// It supports lockless reads and synchronized updates.
type fastTags struct {
	tags atomic.Value

	writerLock sync.Mutex
}

func newFastTags() *fastTags {
	ft := new(fastTags)
	ft.tags.Store(make(map[string]string))
	return ft
}

// Load returns a map of default tags.  This map MUST only be read, not written to.
func (ft *fastTags) Load() map[string]string {
	return ft.tags.Load().(map[string]string)
}

// Set replaces the default tags.
func (ft *fastTags) Set(tags map[string]string) {
	ft.writerLock.Lock()
	defer ft.writerLock.Unlock()

	// copy argument into a new map to ensure caller can't easily mistakenly
	// hold on to a reference and cause a concurrent map modification panic
	newTags := make(map[string]string)
	for k, v := range tags {
		newTags[k] = v
	}

	ft.tags.Store(newTags)
}
