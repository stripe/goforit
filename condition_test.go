package goforit

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConditionSample(t *testing.T) {
	t.Parallel()

	var iters float64 = 10000
	testCases := []float64{1, 0, 0.01, 0.5, 0.8}
	for _, rate := range testCases {
		name := fmt.Sprintf("%d", int(100*rate))
		t.Run(name, func(t *testing.T) {
			rnd := rand.New(rand.NewSource(0))
			cond := &ConditionSample{Rate: rate}
			count := 0
			for i := 0; i < int(iters); i++ {
				match, err := cond.Match(rnd, "test", nil)
				assert.NoError(t, err)
				if match {
					count++
				}
			}
			assert.InDelta(t, iters*rate, count, iters*0.02)
		})
	}
}

// sample-by + consistency
// in-list

// multiple conditions in a flag
// default actions

// errors: missing tags
