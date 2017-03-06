package goforit

import (
	"encoding/csv"
	"io"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

const statsdAddress = "127.0.0.1:8200"

var stats *statsd.Client

func init() {
	stats, _ = statsd.New(statsdAddress)
}

const DefaultInterval = 30 * time.Second

var Rand = rand.New(rand.NewSource(time.Now().Unix()))

type Backend interface {
	Refresh() (map[string]Flag, error)
}

type fileBackend struct {
	filename string
}

func (b fileBackend) Refresh() (map[string]Flag, error) {
	f, err := os.Open(b.filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseFlagsCSV(f)
}

type Flag struct {
	Name string
	Rate float64
}

var flags = map[string]Flag{}
var flagsMtx = sync.RWMutex{}

// Enabled returns a boolean indicating
// whether or not the flag should be considered
// enabled. It returns false if no flag with the specified
// name is found
func Enabled(name string) bool {

	flagsMtx.RLock()
	defer flagsMtx.RUnlock()

	if flags == nil {
		return false
	}
	flag := flags[name]

	// equality should be strict
	// because Float64() can return 0
	if f := Rand.Float64(); f < flag.Rate {
		return true
	}
	return false
}

func flagsToMap(flags []Flag) map[string]Flag {
	flagsMap := map[string]Flag{}
	for _, flag := range flags {
		flagsMap[flag.Name] = Flag{Name: flag.Name, Rate: flag.Rate}
	}
	return flagsMap
}

func parseFlagsCSV(r io.Reader) (map[string]Flag, error) {
	// every row is guaranteed to have 2 fields
	const FieldsPerRecord = 2

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = FieldsPerRecord
	cr.TrimLeadingSpace = true

	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
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
	return flags, nil
}

// BackendFromFile is a helper function that creates a valid
// FlagBackend from a CSV file containing the feature flag values.
// If the same flag is defined multiple times in the same file,
// the last result will be used.
func BackendFromFile(filename string) Backend {
	return fileBackend{filename}
}

// RefreshFlags will use the provided thunk function to
// fetch all feature flags and update the internal cache.
// The thunk provided can use a variety of mechanisms for
// querying the flag values, such as a local file or
// Consul key/value storage.
func RefreshFlags(backend Backend) error {

	refreshedFlags, err := backend.Refresh()
	if err != nil {
		return err
	}

	fmap := map[string]Flag{}
	for _, flag := range refreshedFlags {
		fmap[flag.Name] = flag
	}

	// update the package-level flags
	// which are protected by the mutex
	flagsMtx.Lock()
	flags = fmap
	flagsMtx.Unlock()

	return nil
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
