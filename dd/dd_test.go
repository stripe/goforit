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

func TestDDIntegration(t *testing.T) {
	t.Parallel()

	stats := &mockStatsd{t: t}

	file, err := ioutil.TempFile("", "goforit-")
	require.NoError(t, err)
	defer file.Close()
	defer os.Remove(file.Name())

	internal.AtomicWriteFile(t, file, "a,1\nb,0\n")

	backend := goforit.NewCsvBackend(file.Name(), 10*time.Millisecond)
	fs := goforit.New(backend, addStatsd(stats, nil))
	defer fs.Close()

	fs.Enabled("a")
	fs.Enabled("a")
	fs.Enabled("b")
	time.Sleep(100 * time.Millisecond)

	stats.mtx.Lock()
	defer stats.mtx.Unlock()

	assert.Equal(t, 2, len(stats.histograms))
	assert.Equal(t, 0.1, stats.histogramRates["goforit.age.source"])
	assert.InDelta(t, 10, len(stats.histograms["goforit.age.source"]), 3)
	for _, v := range stats.histograms["goforit.age.source"] {
		assert.InDelta(t, 0.05, v, 0.08)
	}
	assert.Equal(t, 0.01, stats.histogramRates["goforit.age.backend"])
	assert.Equal(t, 3, len(stats.histograms["goforit.age.backend"]))
	for _, v := range stats.histograms["goforit.age.backend"] {
		assert.InDelta(t, 0.05, v, 0.08)
	}

	// TODO: counts, services
}
