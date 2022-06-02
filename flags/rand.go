package flags

import (
	"github.com/stripe/goforit/internal/safepool"
)

type RandFloater interface {
	Float64() float64
}

type pooledRandFloater struct {
	// Rand is not concurrency safe, so keep a pool of them for goroutine-independent use
	rndPool *safepool.RandPool
}

func (prf *pooledRandFloater) Float64() float64 {
	rnd := prf.rndPool.Get()
	defer prf.rndPool.Put(rnd)
	return rnd.Float64()
}

func NewPooledRandomFloater() RandFloater {
	return &pooledRandFloater{
		rndPool: safepool.NewRandPool(),
	}
}
