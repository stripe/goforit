package safepool

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
)

// getSeed returns a cryptographically secure seed for calls to math/rand.NewSource.
func getSeed() int64 {
	buf := make([]byte, 8)
	if n, err := crand.Reader.Read(buf); err != nil || n != 8 {
		panic("failed reading from crypto/rand.Reader")
	}

	return int64(binary.LittleEndian.Uint64(buf))
}

// RandPool is a typed API over a sync.Pool of *math/rand.Rand instances.
type RandPool struct {
	p sync.Pool
}

// NewRandPool returns a new RandPool instance.
func NewRandPool() *RandPool {
	return &RandPool{
		p: sync.Pool{
			New: func() interface{} {
				return rand.New(rand.NewSource(getSeed()))
			},
		},
	}
}

// Get is safe for use from concurrent goroutines, but the returned *rand.Rand instance isn't.
func (p *RandPool) Get() *rand.Rand {
	return p.p.Get().(*rand.Rand)
}

// Put is safe for use from concurrent goroutines and returns a *rand.Rand instance to the pool.
func (p *RandPool) Put(item *rand.Rand) {
	p.p.Put(item)
}
