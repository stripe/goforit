package goforit

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/stripe/goforit/flags"
	"github.com/stripe/goforit/flags1"
	"github.com/stripe/goforit/flags2"
)

type Backend interface {
	// Refresh returns a new set of flags.
	// It also returns the age of these flags, or an empty time if no age is known.
	Refresh() ([]flags.Flag, time.Time, error)
}

type csvFileBackend struct {
	filename string
}

type jsonFileBackend struct {
	filename string
}

type jsonFileBackend2 struct {
	filename string
}

type jsonFormat1 struct {
	Flags       []flags1.Flag1 `json:"flags"`
	UpdatedTime float64        `json:"updated"`
}

func readFile(file string, backend string, parse func(io.Reader) ([]flags.Flag, time.Time, error)) ([]flags.Flag, time.Time, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	return parse(f)
}

func (b jsonFileBackend) Refresh() ([]flags.Flag, time.Time, error) {
	flags, updated, err := readFile(b.filename, "json", parseFlagsJSON)
	if updated != time.Unix(0, 0) {
		return flags, updated, err
	}

	fileInfo, err := os.Stat(b.filename)
	if err != nil {
		return nil, time.Time{}, err
	}
	return flags, fileInfo.ModTime(), nil
}

func (b csvFileBackend) Refresh() ([]flags.Flag, time.Time, error) {
	return readFile(b.filename, "csv", parseFlagsCSV)
}

func parseFlagsCSV(r io.Reader) ([]flags.Flag, time.Time, error) {
	// every row is guaranteed to have 2 fields
	const FieldsPerRecord = 2

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = FieldsPerRecord
	cr.TrimLeadingSpace = true

	rows, err := cr.ReadAll()
	if err != nil {
		return nil, time.Time{}, err
	}

	fflags := make([]flags.Flag, 0, len(rows))
	for _, row := range rows {
		name := row[0]

		rate, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			// TODO also track somehow
			rate = 0
		}

		f := flags1.Flag1{
			Name:   name,
			Active: true,
		}
		if rate != 1 {
			f.Rules = []flags1.RuleInfo{
				{&flags1.RateRule{Rate: rate}, flags.RuleOn, flags.RuleOff},
			}
		}
		fflags = append(fflags, f)
	}
	return fflags, time.Time{}, nil
}

func parseFlagsJSON(r io.Reader) ([]flags.Flag, time.Time, error) {
	dec := json.NewDecoder(r)
	var v jsonFormat1
	err := dec.Decode(&v)
	if err != nil {
		return nil, time.Time{}, err
	}

	fflags := make([]flags.Flag, len(v.Flags))
	for i, f := range v.Flags {
		fflags[i] = f
	}

	return fflags, time.Unix(int64(v.UpdatedTime), 0), nil
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

func (b jsonFileBackend2) Refresh() ([]flags.Flag, time.Time, error) {
	return readFile(b.filename, "json2", parseFlagsJSON2)
}

func parseFlagsJSON2(r io.Reader) ([]flags.Flag, time.Time, error) {
	dec := json.NewDecoder(r)
	var v flags2.JSONFormat2
	err := dec.Decode(&v)
	if err != nil {
		return nil, time.Time{}, err
	}

	flags := make([]flags.Flag, len(v.Flags))
	for i, f := range v.Flags {
		flags[i] = f
	}

	return flags, time.Unix(int64(v.Updated), 0), nil
}

// BackendFromJSONFile2 creates a v2 backend powered by a JSON file
func BackendFromJSONFile2(filename string) Backend {
	return jsonFileBackend2{filename}
}
