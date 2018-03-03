package goforit

import (
	"math/rand"
	"sync"
)

// The default rand.Source is not thread-safe. Here's one with a mutex, so we can use it
// concurrently.
type concurrentSource struct {
	src rand.Source
	mtx sync.Mutex
}

func (cs *concurrentSource) Int63() int64 {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	return cs.src.Int63()
}

func (cs *concurrentSource) Seed(s int64) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.src.Seed(s)
}

func newRandom(seed int64) *rand.Rand {
	return rand.New(&concurrentSource{src: rand.NewSource(seed)})
}
