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

func TestConditionSampleBy(t *testing.T) {
	t.Parallel()

	// can have multiple values
	// flag name matters
	// errors possible

	rnd := rand.New(rand.NewSource(0))
	cond := &ConditionSample{
		Rate: 0.5,
		Tags: []string{"a", "b"},
	}

	// Generate results for a bunch of tags
	type resultKey struct{ a, b int }
	matches := 0
	misses := 0
	results := map[resultKey]bool{}
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			tags := map[string]string{"a": string(a), "b": string(b), "c": "a"}
			match, err := cond.Match(rnd, "test", tags)
			assert.NoError(t, err)
			results[resultKey{a, b}] = match
			if match {
				matches++
			} else {
				misses++
			}
		}
	}
	// Rate should match
	assert.InDelta(t, misses, matches, 200)

	// Verify that the same listed tags yield the same results, even later on
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			tags := map[string]string{"a": string(a), "b": string(b), "c": "b"}
			match, err := cond.Match(rnd, "test", tags)
			assert.NoError(t, err)
			assert.Equal(t, results[resultKey{a, b}], match)
		}
	}

	// Verify that the flag name matters
	disagree := 0
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			tags := map[string]string{"a": string(a), "b": string(b)}
			match, err := cond.Match(rnd, "test2", tags)
			assert.NoError(t, err)
			if results[resultKey{a, b}] != match {
				disagree++
			}
		}
	}
	assert.InDelta(t, 100*100/2, disagree, 200)

	// If a tag is missing, that's an error
	tags := map[string]string{"a": "foo"}
	match, err := cond.Match(rnd, "test", tags)
	assert.False(t, match)
	assert.Error(t, err)
	assert.IsType(t, ErrMissingTag{}, err)
}

// sample-by + consistency
// in-list

// multiple conditions in a flag
// default actions

// errors: missing tags
