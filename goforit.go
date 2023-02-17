package goforit

import (
	"context"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"

	"github.com/stripe/goforit/clamp"
	"github.com/stripe/goforit/flags"
	"github.com/stripe/goforit/internal/safepool"
)

// DefaultStatsdAddr is the address we will emit metrics to if not overridden.
const DefaultStatsdAddr = "127.0.0.1:8200"

const (
	lastAssertInterval     = 5 * time.Minute
	stalenessCheckInterval = 10 * time.Second
)

const lastRefreshMetricName = "goforit.flags.last_refresh_s"

// StatsdClient is the set of methods required to emit metrics to statsd, for
// customizing behavior or mocking.
type StatsdClient interface {
	Histogram(string, float64, []string, float64) error
	Gauge(string, float64, []string, float64) error
	Count(string, int64, []string, float64) error
}

// Goforit is the main interface for the library to check if flags enabled, refresh flags
// customizing behavior or mocking.
type Goforit interface {
	Enabled(ctx context.Context, name string, props map[string]string) (enabled bool)
	RefreshFlags(backend Backend)
	TryRefreshFlags(backend Backend) error
	SetStalenessThreshold(threshold time.Duration)
	AddDefaultTags(tags map[string]string)
	Close() error
}

type (
	printFunc          func(msg string, args ...interface{})
	evaluationCallback func(flag string, active bool)
)

type randFloater interface {
	Float64() float64
}

type pooledRandFloater struct {
	// Rand is not concurrency safe, so keep a pool of them for goroutine-independent use
	rndPool *safepool.RandPool
}

func (prf *pooledRandFloater) Float64() float64 {
	rnd := prf.rndPool.Get()
	defer prf.rndPool.Put(rnd)
	return rnd.Float64()
}

func newPooledRandomFloater() *pooledRandFloater {
	return &pooledRandFloater{
		rndPool: safepool.NewRandPool(),
	}
}

type goforit struct {
	ticker *time.Ticker

	stalenessThreshold atomic.Pointer[time.Duration]

	flags *fastFlags

	// stalenessTicker is used to tell Enabled it should check for staleness.
	stalenessTicker *time.Ticker

	shouldCheckStaleness atomic.Bool

	// Unix time in nanos.
	lastFlagRefreshTime atomic.Int64

	defaultTags *fastTags

	stats StatsdClient

	// Last time we alerted that flags may be out of date
	lastAssertMtx sync.Mutex
	lastAssert    time.Time

	// Rand is not concurrency safe, so keep a pool of them for goroutine-independent use
	rnd randFloater

	printf    printFunc
	evalCB    evaluationCallback
	deletedCB evaluationCallback
}

const DefaultInterval = 30 * time.Second

func newWithoutInit(stalenessTickerInterval time.Duration) *goforit {
	g := &goforit{
		flags:           newFastFlags(),
		defaultTags:     newFastTags(),
		stats:           new(statsd.NoOpClient),
		stalenessTicker: time.NewTicker(stalenessTickerInterval),
		rnd:             newPooledRandomFloater(),
		printf:          log.New(os.Stderr, "[goforit] ", log.LstdFlags).Printf,
	}

	// set an atomic value async rather than check channel inline (which takes a mutex)
	go func() {
		for range g.stalenessTicker.C {
			g.shouldCheckStaleness.Store(true)
		}
	}()

	return g
}

// New creates a new goforit
func New(interval time.Duration, backend Backend, opts ...Option) Goforit {
	g := newWithoutInit(stalenessCheckInterval)
	g.init(interval, backend, opts...)
	// some users may depend on legacy behavior of creating a
	// non-dependency-injected statsd client.
	if _, ok := g.stats.(*statsd.NoOpClient); ok {
		g.stats, _ = statsd.New(DefaultStatsdAddr)
	}
	return g
}

type Option interface {
	apply(g *goforit)
}

type optionFunc func(g *goforit)

func (o optionFunc) apply(g *goforit) {
	o(g)
}

// Logger uses the supplied function to log errors. By default, errors are
// written to os.Stderr.
func Logger(printf func(msg string, args ...interface{})) Option {
	return optionFunc(func(g *goforit) {
		g.printf = printf
	})
}

// Statsd uses the supplied client to emit metrics to. By default, a client is
// created and configured to emit metrics to DefaultStatsdAddr.
func Statsd(stats StatsdClient) Option {
	return optionFunc(func(g *goforit) {
		g.stats = stats
	})
}

// EvaluationCallback registers a callback to execute for each evaluated flag
func EvaluationCallback(cb evaluationCallback) Option {
	return optionFunc(func(g *goforit) {
		g.evalCB = cb
	})
}

// DeletedCallback registers a callback to execute for each flag that is scheduled for deletion
func DeletedCallback(cb evaluationCallback) Option {
	return optionFunc(func(g *goforit) {
		g.deletedCB = cb
	})
}

type flagHolder struct {
	flag  flags.Flag
	clamp clamp.Clamp
}

func (g *goforit) getStalenessThreshold() time.Duration {
	if t := g.stalenessThreshold.Load(); t != nil {
		return *t
	}

	return 0
}

func (g *goforit) logStaleCheck() bool {
	g.lastAssertMtx.Lock()
	defer g.lastAssertMtx.Unlock()
	if time.Since(g.lastAssert) < lastAssertInterval {
		return false
	}
	g.lastAssert = time.Now()
	return true
}

// Check if a time is stale.
func (g *goforit) staleCheck(t time.Time, metric string, metricRate float64, msg string, checkLastAssert bool) {
	if t.IsZero() {
		// Not really useful to treat this as a real time
		return
	}

	// Report the staleness
	staleness := time.Since(t)
	_ = g.stats.Histogram(metric, staleness.Seconds(), nil, metricRate)

	// Log if we're old
	thresh := g.getStalenessThreshold()
	if thresh == 0 {
		return
	}
	if staleness <= thresh {
		return
	}
	// Don't log too often!
	if !checkLastAssert || g.logStaleCheck() {
		g.printf(msg, staleness, thresh)
	}
}

//go:noinline
func (g *goforit) doStaleCheck() {
	last := g.lastFlagRefreshTime.Load()
	// time.Duration is conveniently measured in nanoseconds.
	lastRefreshTime := time.Unix(last/int64(time.Second), last%int64(time.Second))
	g.staleCheck(lastRefreshTime, lastRefreshMetricName, 1,
		"Refresh cycle has not run in %s, past our threshold (%s)", true)
}

// Enabled returns true if the flag should be considered enabled.
// It returns false if no flag with the specified name is found.
func (g *goforit) Enabled(ctx context.Context, name string, properties map[string]string) (enabled bool) {
	enabled = false
	flag, flagExists := g.flags.Get(name)

	// nested loop is to avoid a Swap/write to the bool in the common case,
	// but still ensure only a single Enabled caller does the staleness check.
	if g.shouldCheckStaleness.Load() {
		if stillShouldCheck := g.shouldCheckStaleness.Swap(false); stillShouldCheck {
			g.doStaleCheck()
		}
	}

	if g.evalCB != nil {
		// Wrap in a func, so `enabled` is evaluated at return-time instead of when defer is called
		defer func() { g.evalCB(name, enabled) }()
	}
	if g.deletedCB != nil {
		if df, ok := flag.flag.(flags.DeletableFlag); ok && df.IsDeleted() {
			defer func() { g.deletedCB(name, enabled) }()
		}
	}

	// Check for an override.
	if ctx != nil {
		if ov, ok := ctx.Value(overrideContextKey).(overrides); ok {
			if enabled, ok = ov[name]; ok {
				return
			}
		}
	}

	if !flagExists {
		enabled = false
		return
	}

	switch flag.clamp {
	case clamp.AlwaysOff:
		enabled = false
	case clamp.AlwaysOn:
		enabled = true
	default:
		defaultTags := g.defaultTags.Load()

		var mergedProperties map[string]string
		if len(properties) == 0 {
			// avoid allocating a merged array if we don't have any explicit properties/overrides
			mergedProperties = defaultTags
		} else {
			mergedProperties = make(map[string]string, len(defaultTags)+len(properties))
			for k, v := range defaultTags {
				mergedProperties[k] = v
			}
			for k, v := range properties {
				mergedProperties[k] = v
			}
		}

		var err error
		enabled, err = flag.flag.Enabled(g.rnd, mergedProperties)
		if err != nil {
			g.printf(err.Error())
		}
	}
	return
}

// RefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage. Backend refresh errors are
// ignored.
func (g *goforit) RefreshFlags(backend Backend) {
	_ = g.TryRefreshFlags(backend)
}

// TryRefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage. An error will be returned if
// the backend refresh fails.
func (g *goforit) TryRefreshFlags(backend Backend) error {
	// Ask the backend for the flags
	refreshedFlags, updated, err := backend.Refresh()
	if err != nil {
		_ = g.stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
		g.printf("Error refreshing flags: %s", err)
		return err
	}
	g.lastFlagRefreshTime.Store(time.Now().UnixNano())

	g.flags.Update(refreshedFlags)

	g.staleCheck(updated, "goforit.flags.cache_file_age_s", 0.1,
		"Backend is stale (%s) past our threshold (%s)", false)

	return nil
}

func (g *goforit) SetStalenessThreshold(threshold time.Duration) {
	g.stalenessThreshold.Store(&threshold)
}

func (g *goforit) AddDefaultTags(tags map[string]string) {
	g.defaultTags.Set(tags)
}

// init initializes the flag backend, using the provided refresh function
// to update the internal cache of flags periodically, at the specified interval.
// Applies passed initialization options to the goforit instance.
func (g *goforit) init(interval time.Duration, backend Backend, opts ...Option) {
	for _, opt := range opts {
		opt.apply(g)
	}

	g.RefreshFlags(backend)
	if interval != 0 {
		ticker := time.NewTicker(interval)
		g.ticker = ticker

		go func() {
			for range ticker.C {
				g.RefreshFlags(backend)
			}
		}()
	}
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

// Close releases resources held
// It's still safe to call Enabled()
func (g *goforit) Close() error {
	if g.ticker != nil {
		g.ticker.Stop()
		g.ticker = nil

		g.flags.Close()

		g.stalenessTicker.Stop()
	}
	return nil
}

// for the interface compatability static check
var _ Goforit = &goforit{}
