package goforit

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// arbitrary but fixed for reproducible testing
const seed = 5194304667978865136

const ε = .02

func Reset() {
	flags = map[string]Flag{}
	flagsMtx = sync.RWMutex{}
	stats, _ = statsd.New(statsdAddress)
}

func TestParseFlagsCSV(t *testing.T) {
	filename := filepath.Join("fixtures", "flags_example.csv")

	type testcase struct {
		Name     string
		Filename string
		Expected []Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("fixtures", "flags_example.csv"),
			Expected: []Flag{
				{
					"go.sun.money",
					false,
					[]Rule{},
				},
				{
					"go.moon.mercury",
					true,
					[]Rule{},
				},
				{
					"go.stars.money",
					true,
					[]Rule{
						RateRule{
							Rate: 0.5,
							OnMatch: "on",
							OnMiss: "off",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			f, err := os.Open(filename)
			assert.NoError(t, err)
			defer f.Close()

			flags, _, err := parseFlagsCSV(f)

			assertFlagsEqual(t, flagsToMap(tc.Expected), flags)
		})
	}
}

func TestParseFlagsJSON(t *testing.T) {
	filename := filepath.Join("fixtures", "flags_example.json")

	type testcase struct {
		Name     string
		Filename string
		Expected []Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("fixtures", "flags_example.json"),
			Expected: []Flag{
				{
					"go.sun.moon",
					true,
					[]Rule{
						MatchListRule{
							"host_name",
							[]string{"apibox_123", "apibox_456"},
							"off",
							"continue",
						},
						MatchListRule{
							"host_name",
							[]string{"apibox_789"},
							"on",
							"continue",
						},
						RateRule{
							0.01,
							[]string{"cluster", "db"},
							"on",
							"off",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			f, err := os.Open(filename)
			assert.NoError(t, err)
			defer f.Close()

			flags, _, err := parseFlagsJSON(f)

			assertFlagsEqual(t, flagsToMap(tc.Expected), flags)
		})
	}
}

// should check there is no error
//func checkEnabled(t *testing.T, ctx context.Context, flag string) bool {
func checkEnabled(ctx context.Context, flag string) bool {
	ret, _ := Enabled(ctx, flag, nil)
	// assert.Nil(t, err)
	return ret
}

func TestEnabled(t *testing.T) {
	const iterations = 100000

	Reset()
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	ticker := Init(DefaultInterval, backend)
	defer ticker.Stop()

	assert.False(t, checkEnabled(context.Background(), "go.sun.money"))
	assert.True(t, checkEnabled(context.Background(), "go.moon.mercury"))

	// nil is equivalent to empty context
	assert.False(t, checkEnabled(nil, "go.sun.money"))
	assert.True(t, checkEnabled(nil, "go.moon.mercury"))

	count := 0
	for i := 0; i < iterations; i++ {
		if checkEnabled(context.Background(), "go.stars.money") {
			count++
		}
	}
	actualRate := float64(count) / float64(iterations)

	assert.InEpsilon(t, 0.5, actualRate, ε)
}

// dummyBackend lets us test the RefreshFlags
// by returning the flags only the second time the Refresh
// method is called
type dummyBackend struct {
	// tally how many times Refresh() has been called
	refreshedCount int
}

func (b *dummyBackend) Refresh() (map[string]Flag, time.Time, error) {
	defer func() {
		b.refreshedCount++
	}()

	if b.refreshedCount == 0 {
		return map[string]Flag{}, time.Time{}, nil
	}

	f, err := os.Open(filepath.Join("fixtures", "flags_example.csv"))
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	return parseFlagsCSV(f)
}

func TestRefresh(t *testing.T) {
	Reset()
	backend := &dummyBackend{}

	assert.False(t, checkEnabled(context.Background(), "go.sun.money"))
	assert.False(t, checkEnabled(context.Background(), "go.moon.mercury"))

	ticker := Init(10*time.Millisecond, backend)
	defer ticker.Stop()

	// ensure refresh runs twice to avoid race conditions
	// in which the Refresh method returns but the assertions get called
	// before the flags are actually updated
	for backend.refreshedCount < 2 {
		<-time.After(10 * time.Millisecond)
	}

	assert.False(t, checkEnabled(context.Background(), "go.sun.money"))
	assert.True(t, checkEnabled(context.Background(), "go.moon.mercury"))
}

func TestMultipleDefinitions(t *testing.T) {
	const repeatedFlag = "go.sun.money"
	const lastValue = 0.7
	Reset()

	backend := BackendFromFile(filepath.Join("fixtures", "flags_multiple_definitions.csv"))
	RefreshFlags(backend)

	flag := flags[repeatedFlag]
	assert.Equal(t, flag, Flag{repeatedFlag, true, []Rule{RateRule{Rate: lastValue, OnMatch: "on", OnMiss: "off",}}})

}

// BenchmarkEnabled runs a benchmark for a feature flag
// that is enabled for 50% of operations.
func BenchmarkEnabled(b *testing.B) {
	Reset()
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	ticker := Init(DefaultInterval, backend)
	defer ticker.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checkEnabled(context.Background(), "go.stars.money")
	}
}

// assertFlagsEqual is a helper function for asserting
// that two maps of flags are equal
func assertFlagsEqual(t *testing.T, expected, actual map[string]Flag) {
	assert.Equal(t, len(expected), len(actual))

	for k, v := range expected {
		assert.Equal(t, v, actual[k])
	}
}

func TestOverride(t *testing.T) {
	Reset()

	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	ticker := Init(DefaultInterval, backend)
	defer ticker.Stop()
	RefreshFlags(backend)

	// Empty context gets values from backend.
	assert.False(t, checkEnabled(context.Background(), "go.sun.money"))
	assert.True(t, checkEnabled(context.Background(), "go.moon.mercury"))
	assert.False(t, checkEnabled(context.Background(), "go.extra"))

	// Nil is equivalent to empty context.
	assert.False(t, checkEnabled(nil, "go.sun.money"))
	assert.True(t, checkEnabled(nil, "go.moon.mercury"))
	assert.False(t, checkEnabled(nil, "go.extra"))

	// Can override to true in context.
	ctx := context.Background()
	ctx = Override(ctx, "go.sun.money", true)
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.True(t, checkEnabled(ctx, "go.moon.mercury"))
	assert.False(t, checkEnabled(ctx, "go.extra"))

	// Can override to false.
	ctx = Override(ctx, "go.moon.mercury", false)
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.False(t, checkEnabled(ctx, "go.moon.mercury"))
	assert.False(t, checkEnabled(ctx, "go.extra"))

	// Can override brand new flag.
	ctx = Override(ctx, "go.extra", true)
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.False(t, checkEnabled(ctx, "go.moon.mercury"))
	assert.True(t, checkEnabled(ctx, "go.extra"))

	// Can override an override.
	ctx = Override(ctx, "go.extra", false)
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.False(t, checkEnabled(ctx, "go.moon.mercury"))
	assert.False(t, checkEnabled(ctx, "go.extra"))

	// Separate contexts don't interfere with each other.
	// This allows parallel tests that use feature flags.
	ctx2 := Override(context.Background(), "go.extra", true)
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.False(t, checkEnabled(ctx, "go.moon.mercury"))
	assert.False(t, checkEnabled(ctx, "go.extra"))
	assert.False(t, checkEnabled(ctx2, "go.sun.money"))
	assert.True(t, checkEnabled(ctx2, "go.moon.mercury"))
	assert.True(t, checkEnabled(ctx2, "go.extra"))

	// Overrides apply to child contexts.
	child := context.WithValue(ctx, "foo", "bar")
	assert.True(t, checkEnabled(child, "go.sun.money"))
	assert.False(t, checkEnabled(child, "go.moon.mercury"))
	assert.False(t, checkEnabled(child, "go.extra"))

	// Changes to child contexts don't affect parents.
	child = Override(child, "go.moon.mercury", true)
	assert.True(t, checkEnabled(child, "go.sun.money"))
	assert.True(t, checkEnabled(child, "go.moon.mercury"))
	assert.False(t, checkEnabled(child, "go.extra"))
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.False(t, checkEnabled(ctx, "go.moon.mercury"))
	assert.False(t, checkEnabled(ctx, "go.extra"))
}

func TestOverrideWithoutInit(t *testing.T) {
	Reset()

	// Everything is false by default.
	assert.False(t, checkEnabled(context.Background(), "go.sun.money"))
	assert.False(t, checkEnabled(context.Background(), "go.moon.mercury"))

	// Can override.
	ctx := Override(context.Background(), "go.sun.money", true)
	assert.True(t, checkEnabled(ctx, "go.sun.money"))
	assert.False(t, checkEnabled(ctx, "go.moon.mercury"))
}

type mockHistogramClient struct {
	*statsd.Client
	targetName      string
	histogramValues []float64
	lock            sync.RWMutex
}

func (m *mockHistogramClient) Histogram(name string, value float64, tags []string, rate float64) error {
	if m.targetName == name {
		m.lock.Lock()
		defer m.lock.Unlock()
		m.histogramValues = append(m.histogramValues, value)
	}
	return nil
}

func writeMockJSONFile(t *testing.T, path string, updatedPeriod time.Duration) {
	flags := []FlagJSONFormat{{"go.sun.money", true, []json.RawMessage{}}}
	updatedTime := time.Now().Add(updatedPeriod)
	flagsJson := &JSONFormat{flags, float64(updatedTime.Unix())}

	jsonData, err := json.Marshal(flagsJson)
	require.NoError(t, err)

	tmp, err := ioutil.TempFile(filepath.Dir(path), "flags-temp-")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(jsonData)
	require.NoError(t, err)
	tmp.Close()

	err = os.Rename(tmp.Name(), path)
	require.NoError(t, err)
}

type dummyAgeBackend struct {
	t   time.Time
	mtx sync.RWMutex
}

func (b *dummyAgeBackend) Refresh() (map[string]Flag, time.Time, error) {
	b.mtx.RLock()
	defer b.mtx.RUnlock()
	return map[string]Flag{}, b.t, nil
}

// Test to see proper monitoring of age of the flags dump
func TestCacheFileMetric(t *testing.T) {
	Reset()
	mockStats := &mockHistogramClient{stats.(*statsd.Client), "goforit.flags.cache_file_age_s", []float64{}, sync.RWMutex{}}
	stats = mockStats

	backend := &dummyAgeBackend{t: time.Now().Add(-10 * time.Minute)}
	ticker := Init(10*time.Millisecond, backend)
	defer ticker.Stop()

	time.Sleep(50 * time.Millisecond)
	func() {
		backend.mtx.Lock()
		defer backend.mtx.Unlock()
		backend.t = time.Now()
	}()
	time.Sleep(50 * time.Millisecond)

	mockStats.lock.RLock()
	defer mockStats.lock.RUnlock()

	// We expect something like: [600, 600.01, ..., 0.0, 0.01, ...]
	last := math.Inf(-1)
	old := 0
	recent := 0
	for _, v := range mockStats.histogramValues {
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
	Reset()
	mockStats := &mockHistogramClient{stats.(*statsd.Client), "goforit.flags.last_refresh_s", []float64{}, sync.RWMutex{}}
	stats = mockStats
	ctx := context.Background()

	backend := &dummyBackend{}
	ticker := Init(10*time.Millisecond, backend)
	defer ticker.Stop()

	for i := 0; i < 10; i++ {
		checkEnabled(ctx, "go.sun.money")
		time.Sleep(3 * time.Millisecond)
	}

	// want to stop ticker to simulate Refresh() hanging
	ticker.Stop()

	for i := 0; i < 10; i++ {
		checkEnabled(ctx, "go.sun.money")
		time.Sleep(3 * time.Millisecond)
	}

	mockStats.lock.RLock()
	defer mockStats.lock.RUnlock()

	// We expect something like: [0, 0.01, 0, 0.01, ..., 0, 0.01, 0.02, 0.03]
	for i := 0; i < 10; i++ {
		v := mockStats.histogramValues[i]
		// Should be ~< 10ms
		assert.InDelta(t, 0.005, v, 0.010)
	}

	last := math.Inf(-1)
	large := 0
	for i := 10; i < 20; i++ {
		v := mockStats.histogramValues[i]
		assert.True(t, v > last)
		last = v
		if v > 0.012 {
			large++
		}
	}
	assert.True(t, large > 2)
}
