package goforit

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
)

// arbitrary but fixed for reproducible testing
const seed = 5194304667978865136

const ε = .02

type mockStatsd struct {
	lock            sync.RWMutex
	histogramValues map[string][]float64
}

func (m *mockStatsd) Gauge(string, float64, []string, float64) error {
	return nil
}

func (m *mockStatsd) Count(string, int64, []string, float64) error {
	return nil
}

func (m *mockStatsd) SimpleServiceCheck(string, statsd.ServiceCheckStatus) error {
	return nil
}

func (m *mockStatsd) Histogram(name string, value float64, tags []string, rate float64) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.histogramValues == nil {
		m.histogramValues = make(map[string][]float64)
	}
	m.histogramValues[name] = append(m.histogramValues[name], value)
	return nil
}

func (m *mockStatsd) getHistogramValues(name string) []float64 {
	m.lock.Lock()
	defer m.lock.Unlock()
	s := make([]float64, len(m.histogramValues[name]))
	copy(s, m.histogramValues[name])
	return s
}

var _ StatsdClient = &mockStatsd{}

// Build a goforit for testing
// Also return the log output
func testGoforit(interval time.Duration, backend Backend, enabledTickerInterval time.Duration, options ...Option) (*goforit, *bytes.Buffer) {
	g := newWithoutInit(enabledTickerInterval)
	g.rnd = rand.New(rand.NewSource(seed))
	var buf bytes.Buffer
	g.printf = log.New(&buf, "", 9).Printf
	g.stats = &mockStatsd{}

	if backend != nil {
		g.init(interval, backend, options...)
	}

	return g, &buf
}

func TestGlobal(t *testing.T) {
	// Not parallel, testing global behavior
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	globalGoforit.stats = &mockStatsd{} // prevent logging real metrics

	Init(DefaultInterval, backend)
	defer Close()

	assert.False(t, Enabled(nil, "go.sun.money", nil))
	assert.True(t, Enabled(nil, "go.moon.mercury", nil))
}

func TestGlobalInitOptions(t *testing.T) {
	// Not parallel, testing global behavior
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	stats := &mockStatsd{}
	Init(DefaultInterval, backend, Statsd(stats))
	defer Close()

	assert.Equal(t, stats, globalGoforit.stats)
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	const iterations = 100000

	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	g, _ := testGoforit(DefaultInterval, backend, enabledTickerInterval)
	defer g.Close()

	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.True(t, g.Enabled(context.Background(), "go.moon.mercury", nil))

	// nil is equivalent to empty context
	assert.False(t, g.Enabled(nil, "go.sun.money", nil))
	assert.True(t, g.Enabled(nil, "go.moon.mercury", nil))

	count := 0
	for i := 0; i < iterations; i++ {
		if g.Enabled(context.Background(), "go.stars.money", nil) {
			count++
		}
	}
	actualRate := float64(count) / float64(iterations)

	assert.InEpsilon(t, 0.5, actualRate, ε)
}

func TestMatchListRule(t *testing.T) {

	var r = MatchListRule{"host_name", []string{"apibox_123", "apibox_456", "apibox_789"}}

	// return false and error if empty properties map passed
	res, err := r.Handle("test", map[string]string{})
	assert.False(t, res)
	assert.NotNil(t, err)

	// return false and error if props map passed without specific property needed
	res, err = r.Handle("test", map[string]string{"host_type": "qa-east", "db": "mongo-prod"})
	assert.False(t, res)
	assert.NotNil(t, err)

	// return false and no error if props map contains property but value not in list
	res, err = r.Handle("test", map[string]string{"host_name": "apibox_001", "db": "mongo-prod"})
	assert.False(t, res)
	assert.Nil(t, err)

	// return true and no error if property value is in list
	res, err = r.Handle("test", map[string]string{"host_name": "apibox_456", "db": "mongo-prod"})
	assert.True(t, res)
	assert.Nil(t, err)

	r = MatchListRule{"host_name", []string{}}

	// return false and no error if list of values is empty
	res, err = r.Handle("test", map[string]string{"host_name": "apibox_456", "db": "mongo-prod"})
	assert.False(t, res)
	assert.Nil(t, err)

}

func TestRateRule(t *testing.T) {
	t.Parallel()

	// test normal sample rule (no properties) at different rates
	// by calling Handle() 10,000 times and comparing actual rate
	// to expected rate
	testCases := []float64{1, 0, 0.01, 0.5, 0.8}
	for _, rate := range testCases {
		var iterations = 10000
		var r = RateRule{Rate: rate}
		count := 0
		for i := 0; i < iterations; i++ {
			match, err := r.Handle("test", map[string]string{})
			assert.Nil(t, err)
			if match {
				count++
			}
		}

		actualRate := float64(count) / float64(iterations)
		assert.InDelta(t, rate, actualRate, 0.02)
	}

	//test rate_by (w/ property) by generating random multi-dimension props
	//and memoizing their Enabled checks(), and confirming same results
	//when running Enabled again. Also checks if actual rate ~= expected rate
	type resultKey struct{ a, b int }
	matches := 0
	misses := 0
	results := map[resultKey]bool{}
	var r = RateRule{0.5, []string{"a", "b", "c"}}
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			props := map[string]string{"a": string(a), "b": string(b), "c": "a"}
			match, err := r.Handle("test", props)
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
			props := map[string]string{"a": string(a), "b": string(b), "c": "a"}
			match, err := r.Handle("test", props)
			assert.Nil(t, err)
			assert.Equal(t, results[resultKey{a, b}], match)
		}
	}

	//results should depend on flag name
	//try a different flag name and check the same properties. we expect 50% overlap
	disagree := 0
	for a := 0; a < 100; a++ {
		for b := 0; b < 100; b++ {
			props := map[string]string{"a": string(a), "b": string(b), "c": "a"}
			match, err := r.Handle("test2", props)
			assert.Nil(t, err)
			if results[resultKey{a, b}] != match {
				disagree++
			}
		}
	}
	assert.InDelta(t, 100*100/2, disagree, 200)

	// If a tag is missing, that's an error
	tags := map[string]string{"a": "foo"}
	match, err := r.Handle("test", tags)
	assert.False(t, match)
	assert.Error(t, err)
}

type OnRule struct{}
type OffRule struct{}

func (r *OnRule) Handle(flag string, props map[string]string) (bool, error) {
	return true, nil
}

func (r *OffRule) Handle(flag string, props map[string]string) (bool, error) {
	return false, nil
}

type dummyRulesBackend struct{}

func (b *dummyRulesBackend) Refresh() ([]Flag, time.Time, error) {
	var flags = []Flag{
		{
			"test1",
			true,
			[]RuleInfo{
				{&OnRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test2",
			true,
			[]RuleInfo{
				{&OnRule{}, RuleOff, RuleOn},
			},
			nil,
		},
		{
			"test3",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOn, RuleContinue},
				{&OnRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test4",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOn, RuleOff},
				{&OnRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test5",
			true,
			[]RuleInfo{
				{&OnRule{}, RuleContinue, RuleOn},
				{&OffRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test6",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOff, RuleContinue},
				{&OffRule{}, RuleContinue, RuleOff},
				{&OnRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test7",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOff, RuleContinue},
				{&OnRule{}, RuleContinue, RuleOff},
				{&OnRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test8",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOff, RuleContinue},
				{&OffRule{}, RuleOn, RuleContinue},
				{&OnRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test9",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOff, RuleContinue},
				{&OffRule{}, RuleOn, RuleContinue},
				{&OffRule{}, RuleOn, RuleOff},
			},
			nil,
		},
		{
			"test10",
			true,
			[]RuleInfo{
				{&OffRule{}, RuleOn, RuleContinue},
				{&OffRule{}, RuleOn, RuleOff},
				{&OnRule{}, RuleContinue, RuleOff},
			},
			nil,
		},
		{
			"test11",
			true,
			[]RuleInfo{},
			nil,
		},
		{
			"test12",
			false,
			[]RuleInfo{
				{&OnRule{}, RuleOn, RuleOn},
			},
			nil,
		},
	}
	return flags, time.Time{}, nil
}

func TestCascadingRules(t *testing.T) {
	t.Parallel()

	g, _ := testGoforit(DefaultInterval, &dummyRulesBackend{}, enabledTickerInterval)
	defer g.Close()

	// test match on, miss off single rule
	assert.True(t, g.Enabled(context.Background(), "test1", nil))

	// test match off, miss on single rule
	assert.False(t, g.Enabled(context.Background(), "test2", nil))

	// test match on, miss continue
	assert.True(t, g.Enabled(context.Background(), "test3", nil))

	// test match on, miss off
	assert.False(t, g.Enabled(context.Background(), "test4", nil))

	// test match continue
	assert.False(t, g.Enabled(context.Background(), "test5", nil))

	// test 3 rules -- 2nd rule off
	assert.False(t, g.Enabled(context.Background(), "test6", nil))

	// test cascade to last rule (continue to last rule)
	// must match both 2nd and 3rd rule
	assert.True(t, g.Enabled(context.Background(), "test7", nil))

	// test cascade to last rule (continue to last rule)
	// must match either 2nd rule or 3rd rule, only 3rd on
	assert.True(t, g.Enabled(context.Background(), "test8", nil))

	// test cascade to last rule (continue to last rule)
	// must match either 2nd or 3rd, all 3 off
	assert.False(t, g.Enabled(context.Background(), "test9", nil))

	// test default behavior is off if all rules are "continue"
	assert.False(t, g.Enabled(context.Background(), "test10", nil))

	// test default on if no rules and active = true
	assert.True(t, g.Enabled(context.Background(), "test11", nil))

	// test return false categorically if active = false
	assert.False(t, g.Enabled(context.Background(), "test12", nil))
}

// dummyBackend lets us test the RefreshFlags
// by returning the flags only the second time the Refresh
// method is called
type dummyBackend struct {
	// tally how many times Refresh() has been called
	refreshedCount int
}

func (b *dummyBackend) Refresh() ([]Flag, time.Time, error) {
	defer func() {
		b.refreshedCount++
	}()

	if b.refreshedCount == 0 {
		return []Flag{}, time.Time{}, nil
	}

	f, err := os.Open(filepath.Join("fixtures", "flags_example.csv"))
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	return parseFlagsCSV(f)
}

func TestRefresh(t *testing.T) {
	t.Parallel()

	backend := &dummyBackend{}
	g, _ := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)

	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.False(t, g.Enabled(context.Background(), "go.moon.mercury", nil))

	defer g.Close()

	// ensure refresh runs twice to avoid race conditions
	// in which the Refresh method returns but the assertions get called
	// before the flags are actually updated
	for backend.refreshedCount < 2 {
		<-time.After(10 * time.Millisecond)
	}

	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.True(t, g.Enabled(context.Background(), "go.moon.mercury", nil))
}

func TestRefreshTicker(t *testing.T) {
	t.Parallel()

	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	g, _ := testGoforit(10*time.Second, backend, enabledTickerInterval)
	defer g.Close()

	earthTicker := time.NewTicker(time.Nanosecond)
	g.flags.Store("go.earth.money", Flag{"go.earth.money", true, nil, earthTicker})
	f, ok := g.flags.Load("go.moon.mercury")
	assert.True(t, ok)
	moonTicker := f.(Flag).enabledTicker
	g.flags.Delete("go.stars.money")
	// Give tickers time to run.
	time.Sleep(time.Millisecond)

	g.RefreshFlags(backend)

	_, ok = g.flags.Load("go.sun.money")
	assert.True(t, ok)
	_, ok = g.flags.Load("go.moon.mercury")
	assert.True(t, ok)
	_, ok = g.flags.Load("go.stars.money")
	assert.True(t, ok)
	_, ok = g.flags.Load("go.earth.money")
	assert.False(t, ok)

	// Make sure that the ticker was preserved.
	f, ok = g.flags.Load("go.moon.mercury")
	assert.True(t, ok)
	assert.Equal(t, moonTicker, f.(Flag).enabledTicker)

	// Make sure that the deleted flag's ticker was stopped.
	_, ok = <-earthTicker.C
	assert.True(t, ok)
	// If the ticker wasn't deleted, make sure it can run again.
	time.Sleep(time.Millisecond)
	select {
	case _, ok = <-earthTicker.C:
		// If the ticker was stopped, there's no way we'd get a 2nd tick.
		assert.False(t, ok)
	default:
	}
}

// BenchmarkEnabled50 runs a benchmark for a feature flag
// that is enabled for 50% of operations.
func BenchmarkEnabled50(b *testing.B) {
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	g, _ := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)
	defer g.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Enabled(context.Background(), "go.stars.money", nil)
	}
}

// BenchmarkEnabled100 runs a benchmark for a feature flag
// that is enabled for 100% of operations.
func BenchmarkEnabled100(b *testing.B) {
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	g, _ := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)
	defer g.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Enabled(context.Background(), "go.moon.mercury", nil)
	}
}

// assertFlagsEqual is a helper function for asserting
// that two maps of flags are equal
func assertFlagsEqual(t *testing.T, expected, actual []Flag) {
	assert.Equal(t, len(expected), len(actual))

	for k, v := range expected {
		assert.Equal(t, v, actual[k])
	}
}

type dummyDefaultFlagsBackend struct{}

func (b *dummyDefaultFlagsBackend) Refresh() ([]Flag, time.Time, error) {
	var testFlag = Flag{
		"test",
		true,
		[]RuleInfo{
			{&MatchListRule{"host_name", []string{"apibox_789"}}, RuleOff, RuleContinue},
			{&MatchListRule{"host_name", []string{"apibox_123", "apibox_456"}}, RuleOn, RuleContinue},
			{&RateRule{1, []string{"cluster", "db"}}, RuleOn, RuleOff},
		},
		time.NewTicker(time.Second),
	}
	return []Flag{testFlag}, time.Time{}, nil
}

func TestDefaultTags(t *testing.T) {
	t.Parallel()

	const iterations = 100000
	g, buf := testGoforit(DefaultInterval, &dummyDefaultFlagsBackend{}, enabledTickerInterval)
	defer g.Close()

	// if no properties passed, and no default tags added, then should return false
	assert.False(t, g.Enabled(context.Background(), "test", nil))

	// test match list rule by adding hostname to default tag
	g.AddDefaultTags(map[string]string{"host_name": "apibox_123", "env": "prod"})
	assert.True(t, g.Enabled(context.Background(), "test", nil))

	// test overriding global default in local props map
	assert.False(t, g.Enabled(context.Background(), "test", map[string]string{"host_name": "apibox_789"}))

	// if missing cluster+db, then rate rule should return false
	assert.False(t, g.Enabled(context.Background(), "test", map[string]string{"host_name": "apibox_001"}))

	// if only one of cluster and db, then rate rule should return false
	assert.False(t, g.Enabled(context.Background(), "test", map[string]string{"host_name": "apibox_001", "db": "mongo-prod"}))

	// test combination of global tag and local props
	g.AddDefaultTags(map[string]string{"cluster": "northwest-01"})
	assert.True(t, g.Enabled(context.Background(), "test", map[string]string{"host_name": "apibox_001", "db": "mongo-prod"}))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.True(t, len(lines) == 6)
	for i, line := range lines {
		if i%2 == 1 {
			assert.Contains(t, line, "No property")
			assert.Contains(t, line, "in properties map or default tags")
		}
	}
}

func TestOverride(t *testing.T) {
	t.Parallel()

	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	g, _ := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)
	defer g.Close()
	g.RefreshFlags(backend)

	// Empty context gets values from backend.
	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.True(t, g.Enabled(context.Background(), "go.moon.mercury", nil))
	assert.False(t, g.Enabled(context.Background(), "go.extra", nil))

	// Nil is equivalent to empty context.
	assert.False(t, g.Enabled(nil, "go.sun.money", nil))
	assert.True(t, g.Enabled(nil, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(nil, "go.extra", nil))

	// Can override to true in context.
	ctx := context.Background()
	ctx = Override(ctx, "go.sun.money", true)
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.True(t, g.Enabled(ctx, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(ctx, "go.extra", nil))

	// Can override to false.
	ctx = Override(ctx, "go.moon.mercury", false)
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.False(t, g.Enabled(ctx, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(ctx, "go.extra", nil))

	// Can override brand new flag.
	ctx = Override(ctx, "go.extra", true)
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.False(t, g.Enabled(ctx, "go.moon.mercury", nil))
	assert.True(t, g.Enabled(ctx, "go.extra", nil))

	// Can override an override.
	ctx = Override(ctx, "go.extra", false)
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.False(t, g.Enabled(ctx, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(ctx, "go.extra", nil))

	// Separate contexts don't interfere with each other.
	// This allows parallel tests that use feature flags.
	ctx2 := Override(context.Background(), "go.extra", true)
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.False(t, g.Enabled(ctx, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(ctx, "go.extra", nil))
	assert.False(t, g.Enabled(ctx2, "go.sun.money", nil))
	assert.True(t, g.Enabled(ctx2, "go.moon.mercury", nil))
	assert.True(t, g.Enabled(ctx2, "go.extra", nil))

	// Overrides apply to child contexts.
	child := context.WithValue(ctx, "foo", "bar")
	assert.True(t, g.Enabled(child, "go.sun.money", nil))
	assert.False(t, g.Enabled(child, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(child, "go.extra", nil))

	// Changes to child contexts don't affect parents.
	child = Override(child, "go.moon.mercury", true)
	assert.True(t, g.Enabled(child, "go.sun.money", nil))
	assert.True(t, g.Enabled(child, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(child, "go.extra", nil))
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.False(t, g.Enabled(ctx, "go.moon.mercury", nil))
	assert.False(t, g.Enabled(ctx, "go.extra", nil))
}

func TestOverrideWithoutInit(t *testing.T) {
	t.Parallel()

	g, _ := testGoforit(0, nil, enabledTickerInterval)

	// Everything is false by default.
	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.False(t, g.Enabled(context.Background(), "go.moon.mercury", nil))

	// Can override.
	ctx := Override(context.Background(), "go.sun.money", true)
	assert.True(t, g.Enabled(ctx, "go.sun.money", nil))
	assert.False(t, g.Enabled(ctx, "go.moon.mercury", nil))
}

type dummyAgeBackend struct {
	t   time.Time
	mtx sync.RWMutex
}

func (b *dummyAgeBackend) Refresh() ([]Flag, time.Time, error) {
	var testFlag = Flag{
		"go.sun.money",
		true,
		[]RuleInfo{},
		time.NewTicker(time.Nanosecond),
	}
	b.mtx.RLock()
	defer b.mtx.RUnlock()
	return []Flag{testFlag}, b.t, nil
}

// Test to see proper monitoring of age of the flags dump
func TestCacheFileMetric(t *testing.T) {
	t.Parallel()

	backend := &dummyAgeBackend{t: time.Now().Add(-10 * time.Minute)}
	g, _ := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)
	defer g.Close()

	time.Sleep(50 * time.Millisecond)
	func() {
		backend.mtx.Lock()
		defer backend.mtx.Unlock()
		backend.t = time.Now()
	}()
	time.Sleep(50 * time.Millisecond)

	// We expect something like: [600, 600.01, ..., 0.0, 0.01, ...]
	last := math.Inf(-1)
	old := 0
	recent := 0
	for _, v := range g.stats.(*mockStatsd).getHistogramValues("goforit.flags.cache_file_age_s") {
		if v > 300 {
			// Should be older than last time
			assert.True(t, v > last)
			// Should be about 10 minutes
			assert.InDelta(t, 600, v, 3)
			old++
			assert.Zero(t, recent, "Should never go from new -> old")
		} else {
			// Should be older (unless we just wrote the file)
			if recent > 0 {
				assert.True(t, v > last)
			}
			// Should be about zero
			assert.InDelta(t, 0, v, 3)
			recent++
		}
		last = v
	}
	assert.True(t, old > 2)
	assert.True(t, recent > 2)
}

// Test to see proper monitoring of refreshing the flags dump file from disc
func TestRefreshCycleMetric(t *testing.T) {
	t.Parallel()

	backend := &dummyAgeBackend{t: time.Now().Add(-10 * time.Minute)}
	g, _ := testGoforit(10*time.Millisecond, backend, time.Second)
	defer g.Close()

	tickerC := make(chan time.Time, 1)
	f, _ := g.flags.Load("go.sun.money")
	flag := f.(Flag)
	flag.enabledTicker = &time.Ticker{C: tickerC}
	g.flags.Store("go.sun.money", flag)

	iters := 30
	for i := 0; i < iters; i++ {
		tickerC <- time.Now()
		g.Enabled(nil, "go.sun.money", nil)
		time.Sleep(3 * time.Millisecond)
	}

	// want to stop ticker to simulate Refresh() hanging
	g.ticker.Stop()
	time.Sleep(3 * time.Millisecond)

	for i := 0; i < iters; i++ {
		tickerC <- time.Now()
		g.Enabled(nil, "go.sun.money", nil)
		time.Sleep(3 * time.Millisecond)
	}

	values := g.stats.(*mockStatsd).getHistogramValues("goforit.flags.last_refresh_s")
	// We expect something like: [0, 0.01, 0, 0.01, ..., 0, 0.01, 0.02, 0.03]
	for i := 0; i < iters; i++ {
		v := values[i]
		// Should be small. Really 10ms, but add a bit of wiggle room
		assert.True(t, v < 0.03)
	}

	last := math.Inf(-1)
	large := 0
	for i := iters; i < 2*iters; i++ {
		v := values[i]
		assert.True(t, v > last, fmt.Sprintf("%d: %v: %v", i, v, values))
		last = v
		if v > 0.03 {
			// At least some should be large now, since we're not refreshing
			large++
		}
	}
	assert.True(t, large > 2)
}

func TestStaleFile(t *testing.T) {
	t.Parallel()

	backend := &dummyAgeBackend{t: time.Now().Add(-1000 * time.Hour)}
	g, buf := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)
	defer g.Close()
	g.SetStalenessThreshold(10*time.Minute + 42*time.Second)

	time.Sleep(50 * time.Millisecond)

	// Should see staleness warnings for backend
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.True(t, len(lines) > 2)
	for _, line := range lines {
		assert.Contains(t, line, "10m42")
		assert.Contains(t, line, "Backend")
	}
}

func TestNoStaleFile(t *testing.T) {
	t.Parallel()

	backend := &dummyAgeBackend{t: time.Now().Add(-1000 * time.Hour)}
	g, buf := testGoforit(10*time.Millisecond, backend, enabledTickerInterval)
	defer g.Close()

	time.Sleep(50 * time.Millisecond)

	// Never set staleness, so no warnings
	assert.Zero(t, buf.String())
}

func TestStaleRefresh(t *testing.T) {
	t.Parallel()

	backend := &dummyBackend{}
	g, buf := testGoforit(10*time.Millisecond, backend, time.Nanosecond)
	g.SetStalenessThreshold(50 * time.Millisecond)

	// Simulate stopping refresh
	g.ticker.Stop()
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 10; i++ {
		g.Enabled(nil, "go.sun.money", nil)
	}

	// Should see just one staleness warning
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, 1, len(lines))
	assert.Contains(t, lines[0], "Refresh")
	assert.Contains(t, lines[0], "50ms")
}
