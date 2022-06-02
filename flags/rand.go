package flags

import (
	"github.com/stripe/goforit/internal/safepool"
)

// Rand is a source of pseudo-random floating point numbers between [0, 1.0).
type Rand interface {
	// Float64 returns, as a float64, a pseudo-random number in the half-open interval [0.0,1.0).
	Float64() float64
}

type pooledRand struct {
	// Rand is not concurrency safe, so keep a pool of them for goroutine-independent use
	rndPool *safepool.RandPool
}

func (pr *pooledRand) Float64() float64 {
	rnd := pr.rndPool.Get()
	defer pr.rndPool.Put(rnd)
	return rnd.Float64()
}

func NewRand() Rand {
	return &pooledRand{
		rndPool: safepool.NewRandPool(),
	}
}
