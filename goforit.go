package goforit

import (
	"context"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"

	"github.com/stripe/goforit/clamp"
	"github.com/stripe/goforit/flags2"
	"github.com/stripe/goforit/internal/safepool"
)

// DefaultStatsdAddr is the address we will emit metrics to if not overridden.
const DefaultStatsdAddr = "127.0.0.1:8200"

const (
	lastAssertInterval     = 5 * time.Minute
	stalenessCheckInterval = 10 * time.Second
)

const (
	lastRefreshMetricName          = "goforit.flags.last_refresh_s"
	reportCountsScannedMetricName  = "goforit.report-counts.scanned"
	reportCountsReportedMetricName = "goforit.report-counts.reported"
	reportCountsDurationMetricName = "goforit.report-counts.duration"
)

// MetricsClient is the set of methods required to emit metrics to statsd, for
// customizing behavior or mocking.
type MetricsClient interface {
	Histogram(string, float64, []string, float64) error
	TimeInMilliseconds(name string, milli float64, tags []string, rate float64) error
	Gauge(string, float64, []string, float64) error
	Count(string, int64, []string, float64) error
	io.Closer
}

// Goforit is the main interface for the library to check if flags enabled, refresh flags
// customizing behavior or mocking.
type Goforit interface {
	Enabled(ctx context.Context, name string, props map[string]string) (enabled bool)
	RefreshFlags(backend Backend)
	TryRefreshFlags(backend Backend) error
	SetStalenessThreshold(threshold time.Duration)
	AddDefaultTags(tags map[string]string)
	ReportCounts(callback func(name string, total, enabled uint64, isDeleted bool))
	Close() error
}

type (
	printFunc          func(msg string, args ...interface{})
	evaluationCallback func(flag string, active bool)
)

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
	flags       *fastFlags
	defaultTags *fastTags
	evalCB      evaluationCallback
	deletedCB   evaluationCallback
	// math.Rand is not concurrency safe, so keep a pool of them for goroutine-independent use
	rnd                  *pooledRandFloater
	shouldCheckStaleness atomic.Bool
	ctxOverrideEnabled   bool // immutable

	stalenessThreshold atomic.Pointer[time.Duration]

	// Unix time in nanos.
	lastFlagRefreshTime atomic.Int64

	stats            atomic.Pointer[MetricsClient]
	shouldCloseStats bool // immutable

	isClosed atomic.Bool

	printf printFunc

	mu sync.Mutex

	done func()

	// lastAssert is the last time we alerted that flags may be out of date
	lastAssert time.Time
	// refreshTicker is used to tell the backend it should re-load state from disk
	refreshTicker *time.Ticker
	// stalenessTicker is used to tell Enabled it should check for staleness.
	stalenessTicker *time.Ticker
}

const DefaultInterval = 30 * time.Second

func newWithoutInit(stalenessTickerInterval time.Duration) (*goforit, context.Context) {
	ctx, done := context.WithCancel(context.Background())

	g := &goforit{
		flags:              newFastFlags(),
		defaultTags:        newFastTags(),
		rnd:                newPooledRandomFloater(),
		ctxOverrideEnabled: true,
		stalenessTicker:    time.NewTicker(stalenessTickerInterval),
		printf:             log.New(os.Stderr, "[goforit] ", log.LstdFlags).Printf, //
		done:               done,
	}

	// set an atomic value async rather than check channel inline (which takes a mutex)
	go func(stalenessTicker *time.Ticker) {
		doneCh := ctx.Done()
		for {
			select {
			case <-doneCh:
				return
			case <-stalenessTicker.C:
				g.shouldCheckStaleness.Store(true)
			}
		}
	}(g.stalenessTicker)

	return g, ctx
}

func (g *goforit) getStats() MetricsClient {
	if stats := g.stats.Load(); stats != nil {
		return *stats
	}
	return noopMetricsClient{}
}

func (g *goforit) setStats(c MetricsClient) {
	g.stats.Store(&c)
}

// New creates a new goforit
func New(interval time.Duration, backend Backend, opts ...Option) Goforit {
	g, ctx := newWithoutInit(stalenessCheckInterval)
	g.init(interval, backend, ctx, opts...)
	// some users may depend on legacy behavior of creating a
	// non-dependency-injected statsd client.
	if g.stats.Load() == nil {
		client, _ := statsd.New(DefaultStatsdAddr)
		g.setStats(client)
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

// WithOwnedStats instructs the returned Goforit instance to call
// Close() on its stats client when Goforit is closed.
func WithOwnedStats(isOwned bool) Option {
	return optionFunc(func(g *goforit) {
		g.shouldCloseStats = isOwned
	})
}

func WithContextOverrideDisabled(disabled bool) Option {
	return optionFunc(func(g *goforit) {
		g.ctxOverrideEnabled = !disabled
	})
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
func Statsd(stats MetricsClient) Option {
	return optionFunc(func(g *goforit) {
		g.stats.Store(&stats)
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
	flag          *flags2.Flag2
	disabledCount atomic.Uint64
	enabledCount  atomic.Uint64
	clamp         clamp.Clamp
}

func (g *goforit) getStalenessThreshold() time.Duration {
	if t := g.stalenessThreshold.Load(); t != nil {
		return *t
	}

	return 0
}

func (g *goforit) logStaleCheck() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
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
	_ = g.getStats().Histogram(metric, staleness.Seconds(), nil, metricRate)

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
		if flag != nil && flag.flag.IsDeleted() {
			defer func() { g.deletedCB(name, enabled) }()
		}
	}

	// Check for an override.
	if g.ctxOverrideEnabled && ctx != nil {
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
		flag.disabledCount.Add(1)
	case clamp.AlwaysOn:
		enabled = true
		flag.enabledCount.Add(1)
	default:
		var err error
		enabled, err = flag.flag.Enabled(g.rnd, properties, g.defaultTags.Load())
		if err != nil {
			g.printf(err.Error())
		}
		// move setting these counts into the switch arms so that they can
		// be predicted better for the alwaysOn/alwaysOff cases.
		if enabled {
			flag.enabledCount.Add(1)
		} else {
			flag.disabledCount.Add(1)
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
		_ = g.getStats().Count("goforit.refreshFlags.errors", 1, nil, 1)
		g.printf("Error refreshing flags: %s", err)
		return err
	}

	if !g.isClosed.Load() {
		g.lastFlagRefreshTime.Store(time.Now().UnixNano())
	}

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
func (g *goforit) init(interval time.Duration, backend Backend, ctx context.Context, opts ...Option) {
	for _, opt := range opts {
		opt.apply(g)
	}

	g.RefreshFlags(backend)
	if interval != 0 {
		ticker := time.NewTicker(interval)
		g.refreshTicker = ticker

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					g.RefreshFlags(backend)
				}
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
	if alreadyClosed := g.isClosed.Swap(true); alreadyClosed {
		return nil
	}

	if g.refreshTicker != nil {
		g.refreshTicker.Stop()
		g.refreshTicker = nil
	}

	if g.stalenessTicker != nil {
		g.stalenessTicker.Stop()
		g.stalenessTicker = nil
	}

	if g.done != nil {
		g.done()
		g.done = nil
	}

	if g.shouldCloseStats {
		_ = g.getStats().Close()
	}
	g.stats.Store(nil)

	// clear this so that tests work better
	g.lastFlagRefreshTime.Store(0)
	g.flags.Close()

	return nil
}

func (g *goforit) ReportCounts(callback func(name string, total, enabled uint64, isDeleted bool)) {
	g.mu.Lock()
	defer g.mu.Unlock()

	start := time.Now()
	scanned := int64(0)
	reported := int64(0)

	for name, fh := range g.flags.load() {
		scanned++
		if fh.disabledCount.Load() == 0 && fh.enabledCount.Load() == 0 {
			continue
		}

		disabled := fh.disabledCount.Swap(0)
		enabled := fh.enabledCount.Swap(0)
		callback(name, disabled+enabled, enabled, fh.flag.IsDeleted())
		reported++
	}

	duration := time.Now().Sub(start)
	stats := g.getStats()
	_ = stats.Gauge(reportCountsScannedMetricName, float64(scanned), nil, 1.0)
	_ = stats.Gauge(reportCountsReportedMetricName, float64(reported), nil, 1.0)
	_ = stats.TimeInMilliseconds(reportCountsDurationMetricName, duration.Seconds()*1000, nil, 1.0)
}

// for the interface compatability static check
var _ Goforit = &goforit{}
