package goforit

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test our concurrent random source
func TestRand(t *testing.T) {
	t.Parallel()

	rnd := newRandom(0)

	threshold := 0.8
	iters := 10000

	// Run a bunch of threads simultaneously, they should not interfere
	var wg sync.WaitGroup
	results := make(chan int)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			low := 0
			for i := 0; i < iters; i++ {
				if rnd.Float64() < threshold {
					low++
				}
			}
			results <- low
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	// Check that results of each thread are as expected
	for result := range results {
		assert.InEpsilon(t, threshold*float64(iters), result, 0.1)
	}
}
