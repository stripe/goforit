package goforit

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"os"

	"github.com/DataDog/datadog-go/statsd"
)

const statsdAddress = "127.0.0.1:8200"

const lastAssertInterval = 60 * time.Second
const defaultStalenessThreshold = 10 * time.Minute

// An interface reflecting the parts of statsd that we need, so we can mock it
type statsdClient interface {
	Histogram(string, float64, []string, float64) error
	Gauge(string, float64, []string, float64) error
	Count(string, int64, []string, float64) error
	SimpleServiceCheck(string, statsd.ServiceCheckStatus) error
}

type goforit struct {
	stalenessMtx       sync.RWMutex
	stalenessThreshold time.Duration

	flagsMtx sync.RWMutex
	flags    map[string]Flag

	stats statsdClient

	lastFlagRefreshTime time.Time
	// Last time we alerted that flags may be out of date
	lastAssert time.Time

	// rand is not concurrency safe, in general
	rndMtx sync.Mutex
	rnd    *rand.Rand

	logger *log.Logger
}

const DefaultInterval = 30 * time.Second

type Flag struct {
	Name string
	Rate float64
}

// New creates a new goforit
func New() *goforit {
	stats, _ := statsd.New(statsdAddress)
	return &goforit{
		stats:              stats,
		flags:              map[string]Flag{},
		stalenessThreshold: defaultStalenessThreshold,
		rnd:                rand.New(rand.NewSource(time.Now().UnixNano())),
		logger:             log.New(os.Stderr, "[goforit] ", log.LstdFlags),
	}
}

func (g *goforit) rand() float64 {
	g.rndMtx.Lock()
	defer g.rndMtx.Unlock()
	return g.rnd.Float64()
}

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found
func (g *goforit) Enabled(ctx context.Context, name string) (enabled bool) {
	defer func() {
		var gauge float64
		if enabled {
			gauge = 1
		}
		g.stats.Gauge("goforit.flags.enabled", gauge, []string{fmt.Sprintf("flag:%s", name)}, .1)
	}()

	defer func() {
		g.flagsMtx.RLock()
		defer g.flagsMtx.RUnlock()
		staleness := time.Since(g.lastFlagRefreshTime)
		//histogram of cache process age
		g.stats.Histogram("goforit.flags.last_refresh_s", staleness.Seconds(), nil, .01)
		if staleness > g.stalenessThreshold && time.Since(g.lastAssert) > lastAssertInterval {
			g.lastAssert = time.Now()
			g.logger.Printf("Refresh() cycle has not ran in %s, past our threshold (%s)", staleness, g.stalenessThreshold)
		}
	}()
	// Check for an override.
	if ctx != nil {
		if ov, ok := ctx.Value(overrideContextKey).(overrides); ok {
			if enabled, ok = ov[name]; ok {
				return
			}
		}
	}

	g.flagsMtx.RLock()
	defer g.flagsMtx.RUnlock()
	if g.flags == nil {
		enabled = false
		return
	}
	flag := g.flags[name]

	// equality should be strict
	// because Float64() can return 0
	if f := g.rand(); f < flag.Rate {
		enabled = true
		return
	}
	enabled = false
	return
}

// RefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage.
func (g *goforit) RefreshFlags(backend Backend) {
	// Ask the backend for the flags
	var checkStatus statsd.ServiceCheckStatus
	defer func() {
		g.stats.SimpleServiceCheck("goforit.refreshFlags.present", checkStatus)
	}()
	refreshedFlags, age, err := backend.Refresh()
	if err != nil {
		checkStatus = statsd.Warn
		g.stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
		g.logger.Printf("Error refreshing flags: %s", err)
		return
	}

	fmap := map[string]Flag{}
	for _, flag := range refreshedFlags {
		fmap[flag.Name] = flag
	}
	if !age.IsZero() {
		g.stalenessMtx.RLock()
		defer g.stalenessMtx.RUnlock()
		staleness := time.Since(age)
		stale := staleness > g.stalenessThreshold
		//histogram of staleness
		g.stats.Histogram("goforit.flags.cache_file_age_s", staleness.Seconds(), nil, .1)
		if stale {
			g.logger.Printf("Backend is stale (%s) past our threshold (%s)", staleness, g.stalenessThreshold)
		}
	}
	// update the package-level flags
	// which are protected by the mutex
	g.flagsMtx.Lock()
	g.flags = fmap
	g.lastFlagRefreshTime = time.Now()
	g.flagsMtx.Unlock()

	return
}

func (g *goforit) SetStalenessThreshold(threshold time.Duration) {
	g.stalenessMtx.Lock()
	defer g.stalenessMtx.Unlock()
	g.stalenessThreshold = threshold
}

// Init initializes the flag backend, using the provided refresh function
// to update the internal cache of flags periodically, at the specified interval.
// When the Ticker returned by Init is closed, updates will stop.
func (g *goforit) Init(interval time.Duration, backend Backend) *time.Ticker {

	ticker := time.NewTicker(interval)
	RefreshFlags(backend)

	go func() {
		for _ = range ticker.C {
			RefreshFlags(backend)
		}
	}()
	return ticker
}

// A unique context key for overrides
type overrideContextKeyType struct{}

var overrideContextKey = overrideContextKeyType{}

type overrides map[string]bool

// Override allows overriding the value of a goforit flag within a context.
// This is mainly useful for tests.
func Override(ctx context.Context, name string, value bool) context.Context {
	ov := overrides{}
	if old, ok := ctx.Value(overrideContextKey).(overrides); ok {
		for k, v := range old {
			ov[k] = v
		}
	}
	ov[name] = value
	return context.WithValue(ctx, overrideContextKey, ov)
}
