package goforit

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

const statsdAddress = "127.0.0.1:8200"

const lastAssertInterval = 60 * time.Second

// DefaultInterval is the default amount of time to wait between refreshes
const DefaultInterval = 30 * time.Second

const defaultStalenessThreshold = 5 * time.Minute

// An interface reflecting the parts of statsd that we need, so we can mock it
type statsdClient interface {
	Histogram(string, float64, []string, float64) error
	Gauge(string, float64, []string, float64) error
	Count(string, int64, []string, float64) error
	SimpleServiceCheck(string, statsd.ServiceCheckStatus) error
}

type Flag struct {
	Name string
	Rate float64
}

// Goforit allows checking feature flag statuses
type Goforit struct {
	backend Backend
	ticker  *time.Ticker

	logger *log.Logger
	stats  statsdClient

	rndMtx sync.Mutex
	rnd    *rand.Rand

	flagsMtx            sync.RWMutex
	flags               map[string]Flag
	lastFlagRefreshTime time.Time
	lastAssert          time.Time

	stalenessMtx       sync.RWMutex
	stalenessThreshold time.Duration
}

// New creates and starts a new Goforit
func New(backend Backend) *Goforit {
	stats, _ := statsd.New(statsdAddress)
	g := &Goforit{
		backend:            backend,
		logger:             log.New(os.Stderr, "", log.LstdFlags),
		rnd:                rand.New(rand.NewSource(time.Now().UnixNano())),
		stats:              stats,
		flags:              map[string]Flag{},
		stalenessThreshold: defaultStalenessThreshold,
	}

	return g
}

// Init starts polling for feature flag updates
func (g *Goforit) Init(interval time.Duration) error {
	err := g.RefreshFlags(g.backend)
	if err != nil {
		g.stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
		return err
	}

	g.ticker = time.NewTicker(interval)
	go func() {
		for _ = range g.ticker.C {
			err := g.RefreshFlags(g.backend)
			if err != nil {
				g.stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
			}
		}
	}()
	return nil
}

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found.
func (g *Goforit) Enabled(ctx context.Context, name string) (enabled bool) {
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
		// histogram of cache process age
		g.stats.Histogram("goforit.flags.last_refresh_s", staleness.Seconds(), nil, .01)
		if staleness > g.stalenessThreshold && time.Since(g.lastAssert) > lastAssertInterval {
			g.lastAssert = time.Now()
			g.logger.Printf("[goforit] The Refresh() cycle has not ran in %s, past our threshold (%s)", staleness,
				g.stalenessThreshold)
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
	// TODO: Send metric if flag is not found

	// equality should be strict
	// because Float64() can return 0
	g.rndMtx.Lock()
	f := g.rnd.Float64()
	g.rndMtx.Unlock()
	if f < flag.Rate {
		enabled = true
		return
	}
	enabled = false
	return
}

func flagsToMap(flags []Flag) map[string]Flag {
	flagsMap := map[string]Flag{}
	for _, flag := range flags {
		flagsMap[flag.Name] = Flag{Name: flag.Name, Rate: flag.Rate}
	}
	return flagsMap
}

// RefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage.
func (g *Goforit) RefreshFlags(backend Backend) error {
	refreshedFlags, age, err := backend.Refresh()
	if err != nil {
		return err
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
			g.logger.Printf("[goforit] The backend is stale (%s) past our threshold (%s)", staleness, g.stalenessThreshold)
		}
	}

	// update the flags
	g.flagsMtx.Lock()
	g.flags = fmap
	g.lastFlagRefreshTime = time.Now()
	g.flagsMtx.Unlock()

	return nil
}

// SetStalenessThreshold sets the maximum age flags should have before we alert
func (g *Goforit) SetStalenessThreshold(threshold time.Duration) {
	g.stalenessMtx.Lock()
	defer g.stalenessMtx.Unlock()
	g.stalenessThreshold = threshold
}

// Close stops refreshing flags
func (g *Goforit) Close() error {
	g.ticker.Stop()
	return nil
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
