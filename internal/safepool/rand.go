package safepool

import (
	"math/rand"
	"sync"
)

type RandPool struct {
	p sync.Pool
}

func NewRandPool(newFn func() *rand.Rand) *RandPool {
	return &RandPool{
		p: sync.Pool{
			New: func() interface{} {
				return newFn()
			},
		},
	}
}

func (p *RandPool) Get() *rand.Rand {
	return p.p.Get().(*rand.Rand)
}

func (p *RandPool) Put(item *rand.Rand) {
	p.p.Put(item)
}
