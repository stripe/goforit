package goforit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

const statsdAddress = "127.0.0.1:8200"

const lastAssertInterval = 60 * time.Second

// An interface reflecting the parts of statsd that we need, so we can mock it
type statsdClient interface {
	Histogram(string, float64, []string, float64) error
	Gauge(string, float64, []string, float64) error
	Count(string, int64, []string, float64) error
	SimpleServiceCheck(string, statsd.ServiceCheckStatus) error
}

var stats statsdClient

var stalenessThreshold time.Duration = 5 * time.Minute
var stalenessMtx = sync.RWMutex{}

var flags = map[string]Flag{}
var flagsMtx = sync.RWMutex{}
var lastFlagRefreshTime time.Time
var lastAssert time.Time

func init() {
	stats, _ = statsd.New(statsdAddress)
}

const DefaultInterval = 30 * time.Second

type Backend interface {
	// Refresh returns a new set of flags.
	// It also returns the age of these flags, or an empty time if no age is known.
	Refresh() (map[string]Flag, time.Time, error)
}

type csvFileBackend struct {
	filename string
}

type jsonFileBackend struct {
	filename string
}

func readFile(file string, backend string, parse func(io.Reader) (map[string]Flag, time.Time, error)) (map[string]Flag, time.Time, error) {
	var checkStatus statsd.ServiceCheckStatus
	defer func() {
		stats.SimpleServiceCheck("goforit.refreshFlags."+backend+"FileBackend.present", checkStatus)
	}()
	f, err := os.Open(file)
	if err != nil {
		checkStatus = statsd.Warn
		log.Print("[goforit] unable to open backend file:\n", err)
		return nil, time.Time{}, err
	}
	defer f.Close()
	return parse(f)
}

func (b jsonFileBackend) Refresh() (map[string]Flag, time.Time, error) {
	return readFile(b.filename, "json", parseFlagsJSON)
}

func (b csvFileBackend) Refresh() (map[string]Flag, time.Time, error) {
	return readFile(b.filename, "csv", parseFlagsCSV)
}

type Flag struct {
	Name string
	Rate float64
}

type JSONFormat struct {
	Flags       []Flag  `json:"flags"`
	UpdatedTime float64 `json:"updated"`
}

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found
func Enabled(ctx context.Context, name string) (enabled bool) {
	defer func() {
		var gauge float64
		if enabled {
			gauge = 1
		}
		stats.Gauge("goforit.flags.enabled", gauge, []string{fmt.Sprintf("flag:%s", name)}, .1)
	}()

	defer func() {
		flagsMtx.RLock()
		defer flagsMtx.RUnlock()
		staleness := time.Since(lastFlagRefreshTime)
		//histogram of cache process age
		stats.Histogram("goforit.flags.last_refresh_s", staleness.Seconds(), nil, .01)
		if staleness > stalenessThreshold && time.Since(lastAssert) > lastAssertInterval {
			lastAssert = time.Now()
			log.Printf("[goforit] The Refresh() cycle has not ran in %s, past our threshold (%s)", staleness, stalenessThreshold)
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

	flagsMtx.RLock()
	defer flagsMtx.RUnlock()
	if flags == nil {
		enabled = false
		return
	}
	flag := flags[name]

	// equality should be strict
	// because Float64() can return 0
	if f := rand.Float64(); f < flag.Rate {
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

func parseFlagsCSV(r io.Reader) (map[string]Flag, time.Time, error) {
	// every row is guaranteed to have 2 fields
	const FieldsPerRecord = 2

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = FieldsPerRecord
	cr.TrimLeadingSpace = true

	rows, err := cr.ReadAll()
	if err != nil {
		log.Print("[goforit] error parsing CSV file:\n", err)
		return nil, time.Time{}, err
	}

	flags := map[string]Flag{}
	for _, row := range rows {
		name := row[0]

		rate, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			// TODO also track somehow
			rate = 0
		}

		flags[name] = Flag{Name: name, Rate: rate}
	}
	return flags, time.Time{}, nil
}

func parseFlagsJSON(r io.Reader) (map[string]Flag, time.Time, error) {
	dec := json.NewDecoder(r)
	var v JSONFormat
	err := dec.Decode(&v)
	if err != nil {
		log.Print("[goforit] error parsing JSON file:\n", err)
		return nil, time.Time{}, err
	}
	return flagsToMap(v.Flags), time.Unix(int64(v.UpdatedTime), 0), nil
}

// BackendFromFile is a helper function that creates a valid
// FlagBackend from a CSV file containing the feature flag values.
// If the same flag is defined multiple times in the same file,
// the last result will be used.
func BackendFromFile(filename string) Backend {
	return csvFileBackend{filename}
}

// BackendFromJSONFile creates a backend powered by JSON file
// instead of CSV
func BackendFromJSONFile(filename string) Backend {
	return jsonFileBackend{filename}
}

// RefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage.
func RefreshFlags(backend Backend) error {

	refreshedFlags, age, err := backend.Refresh()
	if err != nil {
		return err
	}

	fmap := map[string]Flag{}
	for _, flag := range refreshedFlags {
		fmap[flag.Name] = flag
	}
	if !age.IsZero() {
		stalenessMtx.RLock()
		defer stalenessMtx.RUnlock()
		staleness := time.Since(age)
		stale := staleness > stalenessThreshold
		//histogram of staleness
		stats.Histogram("goforit.flags.cache_file_age_s", staleness.Seconds(), nil, .1)
		if stale {
			log.Printf("[goforit] The backend is stale (%s) past our threshold (%s)", staleness, stalenessThreshold)
		}
	}
	// update the package-level flags
	// which are protected by the mutex
	flagsMtx.Lock()
	flags = fmap
	lastFlagRefreshTime = time.Now()
	flagsMtx.Unlock()

	return nil
}

func SetStalenessThreshold(threshold time.Duration) {
	stalenessMtx.Lock()
	defer stalenessMtx.Unlock()
	stalenessThreshold = threshold
}

// Init initializes the flag backend, using the provided refresh function
// to update the internal cache of flags periodically, at the specified interval.
// When the Ticker returned by Init is closed, updates will stop.
func Init(interval time.Duration, backend Backend) *time.Ticker {

	ticker := time.NewTicker(interval)
	err := RefreshFlags(backend)
	if err != nil {
		stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
	}

	go func() {
		for _ = range ticker.C {
			err := RefreshFlags(backend)
			if err != nil {
				stats.Count("goforit.refreshFlags.errors", 1, nil, 1)
			}
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
