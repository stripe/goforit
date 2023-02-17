package goforit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/stripe/goforit/flags2"
	"go.uber.org/goleak"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stripe/goforit/clamp"
	"github.com/stripe/goforit/flags"
)

const ε = .02

type mockStatsd struct {
	lock            sync.RWMutex
	histogramValues map[string][]float64
}

func (m *mockStatsd) Close() error {
	return nil
}

func (m *mockStatsd) Gauge(string, float64, []string, float64) error {
	return nil
}

func (m *mockStatsd) Count(string, int64, []string, float64) error {
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

type logBuffer struct {
	buf bytes.Buffer
	mu  sync.Mutex
}

func (l *logBuffer) Write(b []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.Write(b)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.buf.String()
}

var _ io.Writer = &logBuffer{}

// Build a goforit for testing
// Also return the log output
func testGoforit(interval time.Duration, backend Backend, enabledTickerInterval time.Duration, options ...Option) (*goforit, *logBuffer) {
	g := newWithoutInit(enabledTickerInterval)
	g.rnd = newPooledRandomFloater()
	buf := new(logBuffer)
	g.printf = log.New(buf, "", 9).Printf
	g.stats = &mockStatsd{}

	if backend != nil {
		g.init(interval, backend, options...)
	}

	return g, buf
}

func TestEnabled(t *testing.T) {
	t.Parallel()

	const iterations = 100000

	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))
	g, _ := testGoforit(DefaultInterval, backend, stalenessCheckInterval)
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

type (
	OnRule  struct{}
	OffRule struct{}
)

func (r *OnRule) Handle(rnd *pooledRandFloater, flag string, props map[string]string) (bool, error) {
	return true, nil
}

func (r *OffRule) Handle(rnd *pooledRandFloater, flag string, props map[string]string) (bool, error) {
	return false, nil
}

// dummyBackend lets us test the RefreshFlags
// by returning the flags only the second time the Refresh
// method is called
type dummyBackend struct {
	// tally how many times Refresh() has been called
	refreshedCount int32 // read atomically
}

func (b *dummyBackend) Refresh() ([]flags.Flag, time.Time, error) {
	defer func() {
		atomic.AddInt32(&b.refreshedCount, 1)
	}()

	if atomic.LoadInt32(&b.refreshedCount) == 0 {
		return []flags.Flag{}, time.Time{}, nil
	}

	f, err := os.Open(filepath.Join("testdata", "flags2_example.json"))
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	return parseFlagsJSON2(f)
}

func TestRefresh(t *testing.T) {
	t.Parallel()

	backend := &dummyBackend{}
	g, _ := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)

	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.False(t, g.Enabled(context.Background(), "go.moon.mercury", nil))

	defer g.Close()

	// ensure refresh runs twice to avoid race conditions
	// in which the Refresh method returns but the assertions get called
	// before the flags are actually updated
	for atomic.LoadInt32(&backend.refreshedCount) < 2 {
		<-time.After(10 * time.Millisecond)
	}

	assert.False(t, g.Enabled(context.Background(), "go.sun.money", nil))
	assert.True(t, g.Enabled(context.Background(), "go.moon.mercury", nil))
}

func TestNonExistent(t *testing.T) {
	t.Parallel()

	backend := &dummyBackend{}
	g, _ := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
	defer g.Close()

	g.deletedCB = func(name string, enabled bool) {
		assert.False(t, enabled)
	}

	// if non-existent flags aren't handled correctly, this could panic
	assert.False(t, g.Enabled(context.Background(), "non.existent.tag", nil))
}

// errorBackend always returns an error for refreshes.
type errorBackend struct{}

func (e *errorBackend) Refresh() ([]flags.Flag, time.Time, error) {
	return []flags.Flag{}, time.Time{}, errors.New("read failed")
}

func TestTryRefresh(t *testing.T) {
	t.Parallel()

	backend := &errorBackend{}
	g, _ := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
	defer g.Close()

	err := g.TryRefreshFlags(backend)
	assert.Error(t, err)
}

func TestRefreshTicker(t *testing.T) {
	t.Parallel()

	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))
	g, _ := testGoforit(10*time.Second, backend, stalenessCheckInterval)
	defer g.Close()

	g.flags.storeForTesting("go.earth.money", flagHolder{flags2.Flag2{"go.earth.money", "seed", nil, false}, clamp.MayVary})
	g.flags.deleteForTesting("go.stars.money")
	// Give tickers time to run.
	time.Sleep(time.Millisecond)

	g.RefreshFlags(backend)

	_, ok := g.flags.Get("go.sun.money")
	assert.True(t, ok)
	_, ok = g.flags.Get("go.moon.mercury")
	assert.True(t, ok)
	_, ok = g.flags.Get("go.stars.money")
	assert.True(t, ok)
	_, ok = g.flags.Get("go.earth.money")
	assert.False(t, ok)
}

func BenchmarkEnabled(b *testing.B) {
	backends := []struct {
		name    string
		backend Backend
	}{
		{"json2", BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))},
	}
	flags := []struct {
		name string
		flag string
	}{
		{"50pct", "go.stars.money"},
		{"on", "go.moon.mercury"},
	}

	for _, backend := range backends {
		for _, flag := range flags {
			name := fmt.Sprintf("%s/%s", backend.name, flag.name)
			b.Run(name, func(b *testing.B) {
				g, _ := testGoforit(10*time.Microsecond, backend.backend, stalenessCheckInterval)
				defer g.Close()
				b.ResetTimer()
				b.ReportAllocs()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						_ = g.Enabled(context.Background(), flag.flag, nil)
					}
				})
			})
		}
	}
}

func BenchmarkEnabledWithArgs(b *testing.B) {
	backends := []struct {
		name    string
		backend Backend
	}{
		{"json2", BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))},
	}
	flags := []struct {
		name string
		flag string
	}{
		{"flag5", "flag5"},
	}
	defaultTags := []map[string]string{
		nil,
		{
			"foo": "a",
			"bar": "b",
		},
	}

	for _, backend := range backends {
		for _, flag := range flags {
			for _, tags := range defaultTags {
				name := fmt.Sprintf("%s/%s/%v", backend.name, flag.name, tags)
				b.Run(name, func(b *testing.B) {
					g, _ := testGoforit(10*time.Microsecond, backend.backend, stalenessCheckInterval)
					if tags != nil {
						g.AddDefaultTags(tags)
					}
					defer g.Close()
					b.ResetTimer()
					b.ReportAllocs()
					b.RunParallel(func(pb *testing.PB) {
						props := map[string]string{
							"token": "id_123",
						}
						for pb.Next() {
							_ = g.Enabled(context.Background(), flag.flag, props)
						}
					})
				})
			}
		}
	}
}

type dummyDefaultFlagsBackend struct{}

func (b *dummyDefaultFlagsBackend) Refresh() ([]flags.Flag, time.Time, error) {
	testFlag := flags2.Flag2{
		"test",
		"seed",
		[]flags2.Rule2{
			{
				HashBy:  flags2.HashByRandom,
				Percent: flags2.PercentOff,
				Predicates: []flags2.Predicate2{
					{
						Attribute: "host_name",
						Operation: flags2.OpIn,
						Values: map[string]bool{
							"apibox_789": true,
						},
					},
				},
			},
			{
				HashBy:  flags2.HashByRandom,
				Percent: flags2.PercentOn,
				Predicates: []flags2.Predicate2{
					{
						Attribute: "host_name",
						Operation: flags2.OpIn,
						Values: map[string]bool{
							"apibox_123": true,
							"apibox_456": true,
						},
					},
				},
			},
			{
				HashBy:  flags2.HashByRandom,
				Percent: flags2.PercentOn,
				Predicates: []flags2.Predicate2{
					{
						Attribute: "cluster",
						Operation: flags2.OpIn,
						Values: map[string]bool{
							"northwest-01": true,
						},
					},
					{
						Attribute: "db",
						Operation: flags2.OpIn,
						Values: map[string]bool{
							"mongo-prod": true,
						},
					},
				},
			},
		},
		false,
	}
	return []flags.Flag{testFlag}, time.Time{}, nil
}

func TestDefaultTags(t *testing.T) {
	t.Parallel()

	g, _ := testGoforit(DefaultInterval, &dummyDefaultFlagsBackend{}, stalenessCheckInterval)
	defer func() { _ = g.Close() }()

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
	assert.False(t, g.Enabled(context.Background(), "test", map[string]string{"host_name": "apibox_001", "db": "mongo-qa"}))
}

func TestOverride(t *testing.T) {
	t.Parallel()

	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))
	g, _ := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
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

	g, _ := testGoforit(0, nil, stalenessCheckInterval)
	defer func() { _ = g.Close() }()

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

func (b *dummyAgeBackend) Refresh() ([]flags.Flag, time.Time, error) {
	testFlag := flags2.Flag2{
		Name: "go.sun.money",
		Seed: "seed",
		Rules: []flags2.Rule2{
			{
				HashBy:  flags2.HashByRandom,
				Percent: flags2.PercentOn,
			},
		},
	}
	b.mtx.RLock()
	defer b.mtx.RUnlock()
	return []flags.Flag{testFlag}, b.t, nil
}

// Test to see proper monitoring of age of the flags dump
func TestCacheFileMetric(t *testing.T) {
	t.Parallel()

	backend := &dummyAgeBackend{t: time.Now().Add(-10 * time.Minute)}
	g, _ := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
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
	g, _ := testGoforit(10*time.Millisecond, backend, 100*time.Microsecond)
	defer g.Close()

	flag, _ := g.flags.Get("go.sun.money")
	g.flags.storeForTesting("go.sun.money", flag)

	iters := 30
	for i := 0; i < iters; i++ {
		g.Enabled(nil, "go.sun.money", nil)
		time.Sleep(3 * time.Millisecond)
	}

	initialMetricCount := len(g.stats.(*mockStatsd).getHistogramValues(lastRefreshMetricName))

	const antiFlakeSlack = 10

	// subtract 2 for iters to avoid flakey tests
	assert.GreaterOrEqual(t, initialMetricCount, iters-antiFlakeSlack)

	// want to stop ticker to simulate Refresh() hanging
	g.ticker.Stop()
	time.Sleep(3 * time.Millisecond)

	for i := 0; i < iters; i++ {
		g.Enabled(nil, "go.sun.money", nil)
		// sleep to ensure the g.stalenessTicker pumps
		time.Sleep(3 * time.Millisecond)
	}

	values := g.stats.(*mockStatsd).getHistogramValues(lastRefreshMetricName)
	assert.Greater(t, len(values), initialMetricCount)
	assert.GreaterOrEqual(t, len(values), iters-antiFlakeSlack)
	// We expect something like: [0, 0.01, 0, 0.01, ..., 0, 0.01, 0.02, 0.03]
	for i := 0; i < initialMetricCount; i++ {
		v := values[i]
		// Should be small. Really 10ms, but add a bit of wiggle room
		assert.True(t, v < 0.03)
	}

	last := math.Inf(-1)
	large := 0
	for i := initialMetricCount; i < len(values); i++ {
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
	g, buf := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
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
	g, buf := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
	defer g.Close()

	time.Sleep(50 * time.Millisecond)

	// Never set staleness, so no warnings
	assert.Zero(t, buf.String())
}

func TestStaleRefresh(t *testing.T) {
	t.Parallel()

	backend := &dummyBackend{}
	g, buf := testGoforit(10*time.Millisecond, backend, time.Nanosecond)
	defer func() { _ = g.Close() }()
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

type flagStatus struct {
	flag   string
	active bool
}

func TestEvaluationCallback(t *testing.T) {
	t.Parallel()

	evaluated := map[flagStatus]int{}
	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))
	g := New(stalenessCheckInterval,
		backend,
		EvaluationCallback(func(flag string, active bool) {
			evaluated[flagStatus{flag, active}] += 1
		}),
		WithOwnedStats(true),
	)
	defer g.Close()

	g.Enabled(nil, "go.sun.money", nil)
	g.Enabled(nil, "go.moon.mercury", nil)
	g.Enabled(nil, "go.moon.mercury", nil)

	assert.Equal(t, 2, len(evaluated))
	assert.Equal(t, 1, evaluated[flagStatus{"go.sun.money", false}])
	assert.Equal(t, 2, evaluated[flagStatus{"go.moon.mercury", true}])
}

func TestDeletionCallback(t *testing.T) {
	t.Parallel()

	deleted := map[flagStatus]int{}
	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_acceptance.json"))
	g := New(stalenessCheckInterval,
		backend,
		DeletedCallback(func(flag string, active bool) {
			deleted[flagStatus{flag, active}] += 1
		}),
		WithOwnedStats(true),
	)
	defer g.Close()

	g.Enabled(nil, "on_flag", nil)
	g.Enabled(nil, "deleted_on_flag", nil)
	g.Enabled(nil, "deleted_on_flag", nil)
	g.Enabled(nil, "explicitly_not_deleted_flag", nil)

	assert.Equal(t, 1, len(deleted))
	assert.Equal(t, 2, deleted[flagStatus{"deleted_on_flag", true}])
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
