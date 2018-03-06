package goforit

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"sort"
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
	Name   string
	Active bool
	Rules  []Rule
}

type Rule interface {
	Handle(ctx context.Context, props map[string]string) bool
	onMatch() string
	onMiss() string
}

type MatchListRule struct {
	Property string
	Values   []string
	OnMatch  string `json:"on_match"`
	OnMiss   string `json:"on_miss"`
}

type RateRule struct {
	Rate       float64
	Properties []string
	OnMatch    string `json:"on_match"`
	OnMiss     string `json:"on_miss"`
}

type FlagJSONFormat struct {
	Name   string
	Active bool
	Rules  []json.RawMessage
}

type JSONFormat struct {
	Flags       []FlagJSONFormat `json:"flags"`
	UpdatedTime float64          `json:"updated"`
}

var flags = map[string]Flag{}
var flagsMtx = sync.RWMutex{}

var lastFlagRefreshTime time.Time
var lastAssert time.Time

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found
func Enabled(ctx context.Context, name string, properties map[string]string) (enabled bool, err error) {
	err = nil
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
	enabled = false
	if flags == nil {
		err = errors.New("empty flags map")
		return
	}
	flag := flags[name]
	if !flag.Active {
		return
	}
	for _, r := range flag.Rules {

		res := r.Handle(ctx, properties)
		var matchBehavior string
		if res {
			matchBehavior = r.onMatch()
		} else {
			matchBehavior = r.onMiss()
		}
		switch matchBehavior {
		case "on":
			enabled = true
			return
		case "off":
			enabled = false
			return
		case "match_list":
			continue
		default:
			err = errors.New("unknown match behavior: " + matchBehavior)
			return
		}
	}
	enabled = true
	return
}

func getProperty(ctx context.Context, props map[string]string, prop string) string {
	if v, ok := props[prop]; ok {
		return v
	} else if v, ok := ctx.Value(overrideContextKey).(string); ok {
		return v
	} else {
		return ""
	}
}

func (r RateRule) Handle(ctx context.Context, props map[string]string) bool {
	if r.Properties != nil {
		// get the sha1 of the properties values concat
		h := sha1.New()
		// sort the properties for consistent behavior (should we sort on Refresh()?)
		sort.Strings(r.Properties)
		var buffer bytes.Buffer
		for _, val := range r.Properties {
			buffer.WriteString(getProperty(ctx, props, val))
		}
		h.Write([]byte(buffer.String()))
		bs := h.Sum(nil)
		encodedStr := hex.EncodeToString(bs)
		// get the most significant 16 digits
		x, _ := strconv.ParseUint(encodedStr[0:4], 16, 16)
		// check to see if the 16 most significant bits of the hex
		// is less than (rate * 2^16)
		return float64(x) < (r.Rate * float64(1<<16))
	} else {
		f := rand.Float64()
		return f < r.Rate
	}
}

func (r MatchListRule) Handle(ctx context.Context, props map[string]string) bool {
	prop := getProperty(ctx, props, r.Property)
	for _, val := range r.Values {
		if val == prop {
			return true
		}
	}
	return false
}

// getter for the match behavior
func (b RateRule) onMatch() string {
	return b.OnMatch
}

// getter for the miss behavior
func (b RateRule) onMiss() string {
	return b.OnMiss
}

// getter for the match behavior
func (b MatchListRule) onMatch() string {
	return b.OnMatch
}

// getter for the miss behavior
func (b MatchListRule) onMiss() string {
	return b.OnMiss
}

func flagsToMap(flags []Flag) map[string]Flag {
	flagsMap := map[string]Flag{}
	for _, flag := range flags {
		flagsMap[flag.Name] = flag
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
		var active = false
		var rules = []Rule{}
		if rate > 0 {
			active = true
			if rate < 1 {
				rules = append(rules, RateRule{Rate: rate, OnMatch: "on", OnMiss: "off"})
			}
		}
		flags[name] = Flag{Name: name, Active: active, Rules: rules}
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
	// we first unmarshall into the intermediate FlagJSONFormat
	// then do another pass to parse the different rule types per flag
	flags := make([]Flag, len(v.Flags))
	for i, flag := range v.Flags {
		rules := make([]Rule, len(flag.Rules))
		for i2, r := range flag.Rules {

			// first extract the rule type
			var obj map[string]interface{}
			err = json.Unmarshal(r, &obj)
			if err != nil {
				return nil, time.Time{}, err
			}

			ruleType := ""
			if t, ok := obj["type"].(string); ok {
				ruleType = t
			} else {
				return nil, time.Time{}, errors.New("Malformed json. Missing rule type.")
			}

			// unmarshal again into the correct rule struct now that we know the type
			var actual Rule
			switch ruleType {
			case "sample":
				actual = &RateRule{}
			case "match_list":
				actual = &MatchListRule{}
			default:
				return nil, time.Time{}, errors.New("Malformed json. Unknown rule type: " + ruleType)
			}

			err = json.Unmarshal(r, actual)
			if err != nil {
				return nil, time.Time{}, err
			}

			rules[i2] = actual
			flags[i] = Flag{Name: flag.Name, Active: flag.Active, Rules: rules}
		}
	}
	return flagsToMap(flags), time.Unix(int64(v.UpdatedTime), 0), nil
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
