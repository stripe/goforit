package goforit

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

const statsdAddress = "127.0.0.1:8200"

const lastAssertInterval = 5 * time.Minute

const enabledTickerInterval = 10 * time.Second

// An interface reflecting the parts of statsd that we need, so we can mock it
type statsdClient interface {
	Histogram(string, float64, []string, float64) error
	Gauge(string, float64, []string, float64) error
	Count(string, int64, []string, float64) error
	SimpleServiceCheck(string, statsd.ServiceCheckStatus) error
}

type goforit struct {
	ticker *time.Ticker

	stalenessMtx       sync.RWMutex
	stalenessThreshold time.Duration

	flags sync.Map

	enabledTickerInterval time.Duration
	// If a flag doesn't exist, this shared ticker will be used.
	enabledTicker *time.Ticker

	// Unix time in nanos.
	lastFlagRefreshTime int64

	defaultTags sync.Map

	stats statsdClient

	// Last time we alerted that flags may be out of date
	lastAssertMtx sync.Mutex
	lastAssert    time.Time

	// rand is not concurrency safe, in general
	rndMtx sync.Mutex
	rnd    *rand.Rand

	logger *log.Logger
}

const DefaultInterval = 30 * time.Second

func newWithoutInit(enabledTickerInterval time.Duration) *goforit {
	stats, _ := statsd.New(statsdAddress)
	return &goforit{
		stats: stats,
		enabledTickerInterval: enabledTickerInterval,
		enabledTicker:         time.NewTicker(enabledTickerInterval),
		rnd:                   rand.New(rand.NewSource(time.Now().UnixNano())),
		logger:                log.New(os.Stderr, "[goforit] ", log.LstdFlags),
	}
}

// New creates a new goforit
func New(interval time.Duration, backend Backend) *goforit {
	g := newWithoutInit(enabledTickerInterval)
	g.init(interval, backend)
	return g
}

func (g *goforit) rand() float64 {
	g.rndMtx.Lock()
	defer g.rndMtx.Unlock()
	return g.rnd.Float64()
}

type Flag struct {
	Name          string
	Active        bool
	Rules         []RuleInfo
	enabledTicker *time.Ticker
}

func (f Flag) Equal(o Flag) bool {
	if f.Name != o.Name || f.Active != o.Active || len(f.Rules) != len(o.Rules) {
		return false
	}
	for i := 0; i < len(f.Rules); i++ {
		if f.Rules[i] != o.Rules[i] {
			return false
		}
	}
	return true
}

type RuleAction string

const (
	RuleOn       RuleAction = "on"
	RuleOff      RuleAction = "off"
	RuleContinue RuleAction = "continue"
)

var validRuleActions = map[RuleAction]bool{
	RuleOn:       true,
	RuleOff:      true,
	RuleContinue: true,
}

type RuleInfo struct {
	Rule    Rule
	OnMatch RuleAction
	OnMiss  RuleAction
}

type Rule interface {
	Handle(flag string, props map[string]string) (bool, error)
}

type MatchListRule struct {
	Property string
	Values   []string
}

type RateRule struct {
	Rate       float64
	Properties []string
}

func (g *goforit) getStalenessThreshold() time.Duration {
	g.stalenessMtx.RLock()
	defer g.stalenessMtx.RUnlock()
	return g.stalenessThreshold
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
	g.stats.Histogram(metric, staleness.Seconds(), nil, metricRate)

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
		g.logger.Printf(msg, staleness, thresh)
	}
}

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found
func (g *goforit) Enabled(ctx context.Context, name string, properties map[string]string) (enabled bool) {
	enabled = false
	f, ok := g.flags.Load(name)
	var flag Flag
	var tickerC <-chan time.Time
	if ok {
		flag = f.(Flag)
		tickerC = flag.enabledTicker.C
	} else {
		tickerC = g.enabledTicker.C
	}

	select {
	case <-tickerC:
		defer func() {
			var gauge float64
			if enabled {
				gauge = 1
			}
			g.stats.Gauge("goforit.flags.enabled", gauge, []string{fmt.Sprintf("flag:%s", name)}, 1)
			last := atomic.LoadInt64(&g.lastFlagRefreshTime)
			// time.Duration is conveniently measured in nanoseconds.
			lastRefreshTime := time.Unix(last/int64(time.Second), last%int64(time.Second))
			g.staleCheck(lastRefreshTime, "goforit.flags.last_refresh_s", 1,
				"Refresh cycle has not run in %s, past our threshold (%s)", true)
		}()
	default:
	}

	// Check for an override.
	if ctx != nil {
		if ov, ok := ctx.Value(overrideContextKey).(overrides); ok {
			if enabled, ok = ov[name]; ok {
				return
			}
		}
	}

	// if flag is inactive, always return false
	if !flag.Active {
		return
	}

	// if there are no rules, but flag is active, always return true
	if len(flag.Rules) == 0 {
		enabled = true
		return
	}

	mergedProperties := make(map[string]string)
	g.defaultTags.Range(func(k, v interface{}) bool {
		mergedProperties[k.(string)] = v.(string)
		return true
	})
	for k, v := range properties {
		mergedProperties[k] = v
	}

	for _, r := range flag.Rules {
		res, err := r.Rule.Handle(flag.Name, mergedProperties)
		if err != nil {
			g.logger.Printf("[goforit] error evaluating rule:\n %s", err)
			return
		}
		var matchBehavior RuleAction
		if res {
			matchBehavior = r.OnMatch
		} else {
			matchBehavior = r.OnMiss
		}
		switch matchBehavior {
		case RuleOn:
			enabled = true
			return
		case RuleOff:
			enabled = false
			return
		case RuleContinue:
			continue
		default:
			g.logger.Printf("[goforit] unknown match behavior: " + string(matchBehavior))
			return
		}
	}
	enabled = false
	return
}

func getProperty(props map[string]string, prop string) (string, error) {
	if v, ok := props[prop]; ok {
		return v, nil
	} else {
		return "", errors.New("No property " + prop + " in properties map or default tags.")
	}
}

func (r *RateRule) Handle(flag string, props map[string]string) (bool, error) {
	if r.Properties != nil {
		// get the sha1 of the properties values concat
		h := sha1.New()
		// sort the properties for consistent behavior
		sort.Strings(r.Properties)
		var buffer bytes.Buffer
		buffer.WriteString(flag)
		for _, val := range r.Properties {
			buffer.WriteString("\000")
			prop, err := getProperty(props, val)
			if err != nil {
				return false, err
			}
			buffer.WriteString(prop)
		}
		h.Write([]byte(buffer.String()))
		bs := h.Sum(nil)
		// get the most significant 32 digits
		x := binary.BigEndian.Uint32(bs)
		// check to see if the 32 most significant bits of the hex
		// is less than (rate * 2^32)
		return float64(x) < (r.Rate * float64(1<<32)), nil
	} else {
		f := rand.Float64()
		return f < r.Rate, nil
	}
}

func (r *MatchListRule) Handle(flag string, props map[string]string) (bool, error) {
	prop, err := getProperty(props, r.Property)
	if err != nil {
		return false, err
	}
	for _, val := range r.Values {
		if val == prop {
			return true, nil
		}
	}
	return false, nil
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
	refreshedFlags, updated, err := backend.Refresh()
	if err != nil {
		checkStatus = statsd.Warn
		g.stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
		g.logger.Printf("Error refreshing flags: %s", err)
		return
	}
	atomic.StoreInt64(&g.lastFlagRefreshTime, time.Now().UnixNano())

	deleted := make(map[string]bool)
	g.flags.Range(func(name, flag interface{}) bool {
		deleted[name.(string)] = true
		return true
	})

	for _, flag := range refreshedFlags {
		delete(deleted, flag.Name)
		oldFlag, ok := g.flags.Load(flag.Name)
		if ok {
			// Avoid churning the map if the flag hasn't changed.
			if !oldFlag.(Flag).Equal(flag) {
				flag.enabledTicker = oldFlag.(Flag).enabledTicker
				g.flags.Store(flag.Name, flag)
			}
		} else {
			flag.enabledTicker = time.NewTicker(g.enabledTickerInterval)
			g.flags.Store(flag.Name, flag)
		}
	}

	for name := range deleted {
		f, ok := g.flags.Load(name)
		if ok {
			f.(Flag).enabledTicker.Stop()
			g.flags.Delete(name)
		}
	}

	g.staleCheck(updated, "goforit.flags.cache_file_age_s", 0.1,
		"Backend is stale (%s) past our threshold (%s)", false)

	return
}

func (g *goforit) SetStalenessThreshold(threshold time.Duration) {
	g.stalenessMtx.Lock()
	defer g.stalenessMtx.Unlock()
	g.stalenessThreshold = threshold
}

func (g *goforit) AddDefaultTags(tags map[string]string) {
	for k, v := range tags {
		g.defaultTags.Store(k, v)
	}
}

// init initializes the flag backend, using the provided refresh function
// to update the internal cache of flags periodically, at the specified interval.
func (g *goforit) init(interval time.Duration, backend Backend) {
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

		g.flags.Range(func(k, v interface{}) bool {
			v.(Flag).enabledTicker.Stop()
			return true
		})

		g.enabledTicker.Stop()
	}
	return nil
}
