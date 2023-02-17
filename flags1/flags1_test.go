package flags1

import (
	"fmt"
	"go.uber.org/goleak"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stripe/goforit/flags"
)

func TestMatchListRule(t *testing.T) {
	r := MatchListRule{"host_name", []string{"apibox_123", "apibox_456", "apibox_789"}}

	// return false and error if empty properties map passed
	res, err := r.Handle(nil, "test", map[string]string{})
	assert.False(t, res)
	assert.NotNil(t, err)

	// return false and error if props map passed without specific property needed
	res, err = r.Handle(nil, "test", map[string]string{"host_type": "qa-east", "db": "mongo-prod"})
	assert.False(t, res)
	assert.NotNil(t, err)

	// return false and no error if props map contains property but value not in list
	res, err = r.Handle(nil, "test", map[string]string{"host_name": "apibox_001", "db": "mongo-prod"})
	assert.False(t, res)
	assert.Nil(t, err)

	// return true and no error if property value is in list
	res, err = r.Handle(nil, "test", map[string]string{"host_name": "apibox_456", "db": "mongo-prod"})
	assert.True(t, res)
	assert.Nil(t, err)

	r = MatchListRule{"host_name", []string{}}

	// return false and no error if list of values is empty
	res, err = r.Handle(nil, "test", map[string]string{"host_name": "apibox_456", "db": "mongo-prod"})
	assert.False(t, res)
	assert.Nil(t, err)
}

func TestRateRule(t *testing.T) {
	t.Parallel()

	// test normal sample rule (no properties) at different rates
	// by calling Handle() 10,000 times and comparing actual rate
	// to expected rate
	rnd := flags.NewRand()
	testCases := []float64{1, 0, 0.01, 0.5, 0.8}
	for _, rate := range testCases {
		iterations := 10000
		r := RateRule{Rate: rate}
		count := 0
		for i := 0; i < iterations; i++ {
			match, err := r.Handle(rnd, "test", map[string]string{})
			assert.Nil(t, err)
			if match {
				count++
			}
		}

		actualRate := float64(count) / float64(iterations)
		assert.InDelta(t, rate, actualRate, 0.02)
	}

	// test rate_by (w/ property) by generating random multi-dimension props
	// and memoizing their Enabled checks(), and confirming same results
	// when running Enabled again. Also checks if actual rate ~= expected rate
	type resultKey struct{ a, b int }
	matches := 0
	misses := 0
	results := map[resultKey]bool{}
	r := RateRule{0.5, []string{"a", "b", "c"}}
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			props := map[string]string{"a": fmt.Sprint(a), "b": fmt.Sprint(b), "c": "a"}
			match, err := r.Handle(rnd, "test", props)
			assert.Nil(t, err)
			if match {
				matches++
			} else {
				misses++
			}
			results[resultKey{a, b}] = match
		}
	}
	assert.InDelta(t, misses, matches, 200)

	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			props := map[string]string{"a": fmt.Sprint(a), "b": fmt.Sprint(b), "c": "a"}
			match, err := r.Handle(rnd, "test", props)
			assert.Nil(t, err)
			assert.Equal(t, results[resultKey{a, b}], match)
		}
	}

	// results should depend on flag name
	// try a different flag name and check the same properties. we expect 50% overlap
	disagree := 0
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			props := map[string]string{"a": fmt.Sprint(a), "b": fmt.Sprint(b), "c": "a"}
			match, err := r.Handle(rnd, "test2", props)
			assert.Nil(t, err)
			if results[resultKey{a, b}] != match {
				disagree++
			}
		}
	}
	assert.InDelta(t, 100*100/2, disagree, 200)

	// If a tag is missing, that's an error
	tags := map[string]string{"a": "foo"}
	match, err := r.Handle(rnd, "test", tags)
	assert.False(t, match)
	assert.Error(t, err)
}

type (
	OnRule  struct{}
	OffRule struct{}
)

func (r *OnRule) Handle(rnd flags.Rand, flag string, props map[string]string) (bool, error) {
	return true, nil
}

func (r *OffRule) Handle(rnd flags.Rand, flag string, props map[string]string) (bool, error) {
	return false, nil
}

type dummyRulesBackend struct{}

func (b *dummyRulesBackend) Refresh() ([]flags.Flag, time.Time, error) {
	flags := []flags.Flag{}
	return flags, time.Time{}, nil
}

func TestCascadingRules(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		active   bool
		rules    []RuleInfo
		expected bool
	}{
		{
			"test match on, miss off single rule",
			true,
			[]RuleInfo{
				{&OnRule{}, flags.RuleOn, flags.RuleOff},
			},
			true,
		},
		{
			"test match off, miss on single rule",
			true,
			[]RuleInfo{
				{&OnRule{}, flags.RuleOff, flags.RuleOn},
			},
			false,
		},
		{
			"test match on, miss continue",
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOn, flags.RuleContinue},
				{&OnRule{}, flags.RuleOn, flags.RuleOff},
			},
			true,
		},
		{
			"test match on, miss off",
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOn, flags.RuleOff},
				{&OnRule{}, flags.RuleOn, flags.RuleOff},
			},
			false,
		},
		{
			"test match continue",
			true,
			[]RuleInfo{
				{&OnRule{}, flags.RuleContinue, flags.RuleOn},
				{&OffRule{}, flags.RuleOn, flags.RuleOff},
			},
			false,
		},
		{
			"test 3 rules -- 2nd rule off",
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOff, flags.RuleContinue},
				{&OffRule{}, flags.RuleContinue, flags.RuleOff},
				{&OnRule{}, flags.RuleOn, flags.RuleOff},
			},
			false,
		},
		{
			"test cascade to last rule (continue to last rule)",
			// must match both 2nd and 3rd rule
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOff, flags.RuleContinue},
				{&OnRule{}, flags.RuleContinue, flags.RuleOff},
				{&OnRule{}, flags.RuleOn, flags.RuleOff},
			},
			true,
		},
		{
			"test cascade to last rule (continue to last rule)",
			// must match either 2nd rule or 3rd rule, only 3rd on
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOff, flags.RuleContinue},
				{&OffRule{}, flags.RuleOn, flags.RuleContinue},
				{&OnRule{}, flags.RuleOn, flags.RuleOff},
			},
			true,
		},
		{
			"test cascade to last rule (continue to last rule)",
			// must match either 2nd or 3rd, all 3 off
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOff, flags.RuleContinue},
				{&OffRule{}, flags.RuleOn, flags.RuleContinue},
				{&OffRule{}, flags.RuleOn, flags.RuleOff},
			},
			false,
		},
		{
			"test default behavior is off if all rules are continue",
			true,
			[]RuleInfo{
				{&OffRule{}, flags.RuleOn, flags.RuleContinue},
				{&OffRule{}, flags.RuleOn, flags.RuleOff},
				{&OnRule{}, flags.RuleContinue, flags.RuleOff},
			},
			false,
		},
		{
			"test default on if no rules and active = true",
			true,
			[]RuleInfo{},
			true,
		},
		{
			"test return false categorically if active = false",
			false,
			[]RuleInfo{
				{&OnRule{}, flags.RuleOn, flags.RuleOn},
			},
			false,
		},
	}

	for _, tc := range testCases {
		flag := Flag1{tc.name, tc.active, tc.rules}
		enabled, err := flag.Enabled(nil, map[string]string{})
		assert.NoError(t, err)
		assert.Equal(t, tc.expected, enabled, tc.name)
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
