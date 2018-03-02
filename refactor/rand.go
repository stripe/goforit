package refactor

import (
	"math/rand"
	"sync"
)

// A random source that's safe for multi-threaded use
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
