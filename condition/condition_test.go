package condition

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/goforit"
)

func TestConditionSample(t *testing.T) {
	t.Parallel()

	var iters float64 = 10000
	testCases := []float64{1, 0, 0.01, 0.5, 0.8}
	for _, rate := range testCases {
		name := fmt.Sprintf("%d", int(100*rate))
		t.Run(name, func(t *testing.T) {
			rnd := rand.New(rand.NewSource(0))
			cond := &Sample{Rate: rate}
			cond.Init()
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

	rnd := rand.New(rand.NewSource(0))
	cond := &Sample{
		Rate: 0.5,
		Tags: []string{"a", "b"},
	}
	cond.Init()

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
			tags := map[string]string{"b": string(b), "a": string(a), "c": "b"}
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
	assert.IsType(t, goforit.ErrMissingTag{}, err)
}

func TestConditionInList(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	tagValues := [][]string{{}, {"x"}, {"x", "y"}, {"x", "y", "z"}}
	testCases := []struct {
		Tag     string
		Results []bool
	}{
		{"x", []bool{false, true, true, true}},
		{"y", []bool{false, false, true, true}},
		{"z", []bool{false, false, false, true}},
		{"o", []bool{false, false, false, false}},
	}
	for _, tc := range testCases {
		for i, values := range tagValues {
			cond := &InList{Tag: "a", Values: values}
			cond.Init()
			match, err := cond.Match(rnd, "test", map[string]string{"a": tc.Tag, "b": "x"})
			assert.NoError(t, err)
			assert.Equal(t, tc.Results[i], match)
		}
	}

	// If a tag is missing, that's an error
	cond := &InList{Tag: "a", Values: []string{}}
	tags := map[string]string{"b": "foo"}
	match, err := cond.Match(rnd, "test", tags)
	assert.False(t, match)
	assert.Error(t, err)
	assert.IsType(t, goforit.ErrMissingTag{}, err)
}

func TestConditionFlag(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	// Simplest flag ever
	flag := &Flag{
		FlagName: "test",
		Active:   true,
		Conditions: []Info{
			{
				OnMatch:   ActionFlagEnabled,
				OnMiss:    ActionFlagDisabled,
				Condition: &Sample{Rate: 1},
			},
		},
	}
	flag.Init()
	en, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, en)

	// Use of Next and whitelists
	flag = &Flag{
		FlagName: "test",
		Active:   true,
		Conditions: []Info{
			{
				OnMatch:   ActionFlagEnabled,
				OnMiss:    ActionNext,
				Condition: &InList{Tag: "a", Values: []string{"x", "y"}},
			},
			{
				OnMatch:   ActionFlagEnabled,
				OnMiss:    ActionFlagDisabled,
				Condition: &InList{Tag: "b", Values: []string{"u", "v"}},
			},
		},
	}
	flag.Init()
	en, err = flag.Enabled(rnd, map[string]string{"a": "e", "b": "e"})
	assert.NoError(t, err)
	assert.False(t, en)
	en, err = flag.Enabled(rnd, map[string]string{"a": "x", "b": "e"})
	assert.NoError(t, err)
	assert.True(t, en)
	en, err = flag.Enabled(rnd, map[string]string{"a": "e", "b": "u"})
	assert.NoError(t, err)
	assert.True(t, en)
	en, err = flag.Enabled(rnd, map[string]string{"a": "x", "b": "u"})
	assert.NoError(t, err)
	assert.True(t, en)
	en, err = flag.Enabled(rnd, map[string]string{"a": "FAKE"})
	assert.Error(t, err)
	assert.IsType(t, goforit.ErrMissingTag{}, err)
	assert.False(t, en)

	// When we get to the end, it's considered disabled
	flag = &Flag{
		FlagName:   "test",
		Active:     true,
		Conditions: []Info{},
	}
	flag.Init()
	en, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, en)

	// There exist some pretty pointless action combinations, but they still work
	flag = &Flag{
		FlagName: "test",
		Active:   true,
		Conditions: []Info{
			{
				// Whatever happens, this is disabled!
				OnMatch:   ActionFlagDisabled,
				OnMiss:    ActionFlagDisabled,
				Condition: &Sample{Rate: 1},
			},
		},
	}
	flag.Init()
	en, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, en)

	// Inactive flags are never on
	flag = &Flag{
		FlagName: "test",
		Active:   false,
		Conditions: []Info{
			{
				OnMatch:   ActionFlagEnabled,
				OnMiss:    ActionFlagDisabled,
				Condition: &Sample{Rate: 1},
			},
		},
	}
	flag.Init()
	en, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, en)

	// Can use randomness
	flag = &Flag{
		FlagName: "test",
		Active:   true,
		Conditions: []Info{
			{
				OnMatch:   ActionNext,
				OnMiss:    ActionFlagDisabled,
				Condition: &Sample{Rate: 0.8},
			},
			{
				OnMatch:   ActionFlagEnabled,
				OnMiss:    ActionFlagDisabled,
				Condition: &Sample{Rate: 0.3},
			},
		},
	}
	flag.Init()
	count := 0
	for i := 0; i < 10000; i++ {
		en, err = flag.Enabled(rnd, map[string]string{"a": "e", "b": "e"})
		assert.NoError(t, err)
		if en {
			count++
		}
	}
	assert.InDelta(t, 2400, count, 200)
}
