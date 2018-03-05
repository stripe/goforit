package dd

import (
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/goforit"
	"github.com/stripe/goforit/internal"
)

type mockStatsd struct {
	mtx            sync.Mutex
	t              *testing.T
	histograms     map[string][]float64
	histogramRates map[string]float64
	oneCountTags   []string
	counts         map[string]int
	countRates     map[string]float64
	service        string
	serviceChecks  map[statsd.ServiceCheckStatus]int
}

func (m *mockStatsd) Histogram(name string, value float64, tags []string, rate float64) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.histograms == nil {
		m.histograms = map[string][]float64{}
		m.histogramRates = map[string]float64{}
	}

	if m.histogramRates[name] == 0 {
		m.histogramRates[name] = rate
	} else {
		// Use the same rate for the same metric
		assert.Equal(m.t, m.histogramRates[name], rate)
	}

	// We always have nil tags for now
	assert.Nil(m.t, tags)

	m.histograms[name] = append(m.histograms[name], value)
	return nil
}

func (m *mockStatsd) Incr(name string, tags []string, rate float64) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.counts == nil {
		m.counts = map[string]int{}
		m.countRates = map[string]float64{}
	}

	if m.countRates[name] == 0 {
		m.countRates[name] = rate
	} else {
		// Use the same rate for the same metric
		assert.Equal(m.t, m.countRates[name], rate)
	}
	m.oneCountTags = tags
	m.counts[name]++
	return nil
}

func (m *mockStatsd) SimpleServiceCheck(name string, status statsd.ServiceCheckStatus) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.service == "" {
		m.service = name
		m.serviceChecks = map[statsd.ServiceCheckStatus]int{}
	} else {
		assert.Equal(m.t, m.service, name)
	}
	m.serviceChecks[status]++
	return nil
}

// Test DD logging of normal operation
func TestDDIntegration(t *testing.T) {
	t.Parallel()

	stats := &mockStatsd{t: t}

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	internal.AtomicWriteFile(t, file, "a,1\nb,0\n")

	backend := goforit.NewCsvBackend(file.Name(), 40*time.Millisecond)
	fs := goforit.New(backend, Statsd(stats))
	defer fs.Close()

	fs.Enabled("a")
	fs.Enabled("a")
	// Make sure the last one goes last
	time.Sleep(100 * time.Millisecond)
	fs.Enabled("b")
	// Give all our goroutines time to shut down
	time.Sleep(100 * time.Millisecond)

	stats.mtx.Lock()
	defer stats.mtx.Unlock()

	// Check histograms
	assert.Equal(t, 2, len(stats.histograms))

	// These get sent every time we check the source data
	assert.InDelta(t, 5, len(stats.histograms["goforit.age.source"]), 2)
	assert.Equal(t, 0.1, stats.histogramRates["goforit.age.source"])
	for _, v := range stats.histograms["goforit.age.source"] {
		assert.True(t, v < 0.3)
	}

	// These are only sent when we call Enabled
	assert.Equal(t, 3, len(stats.histograms["goforit.age.backend"]))
	assert.Equal(t, 0.01, stats.histogramRates["goforit.age.backend"])
	for _, v := range stats.histograms["goforit.age.backend"] {
		assert.True(t, v < 0.3)
	}

	// Check counts
	assert.Equal(t, 1, len(stats.counts))
	assert.Equal(t, 0.01, stats.countRates["goforit.check"])
	assert.Equal(t, 3, stats.counts["goforit.check"])
	assert.Equal(t, []string{"flag:b", "enabled:false"}, stats.oneCountTags)

	// Check service checks
	assert.Equal(t, goforitService, stats.service)
	assert.Equal(t, 1, len(stats.serviceChecks))
	assert.InDelta(t, 5, stats.serviceChecks[statsd.Ok], 2)
}

// Test DD logging of errors
func TestDDIntegrationErrors(t *testing.T) {
	t.Parallel()

	stats := &mockStatsd{t: t}

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	internal.AtomicWriteFile(t, file, "a,1\nb,0\n")
	backend := goforit.NewCsvBackend(file.Name(), 40*time.Millisecond)
	fs := goforit.New(backend, Statsd(stats))
	defer fs.Close()

	// A non-critical error: flag is missing
	fs.Enabled("c")
	time.Sleep(100 * time.Millisecond)
	func() {
		stats.mtx.Lock()
		defer stats.mtx.Unlock()
		assert.Zero(t, stats.serviceChecks[statsd.Warn])
		assert.Equal(t, 1, stats.counts["goforit.error"])
	}()

	// A critical error: file is invalid
	internal.AtomicWriteFile(t, file, "a")
	time.Sleep(100 * time.Millisecond)
	func() {
		stats.mtx.Lock()
		defer stats.mtx.Unlock()
		assert.True(t, stats.serviceChecks[statsd.Warn] > 0)
		assert.True(t, stats.counts["goforit.error"] > 1)
	}()
}
