package goforit

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"time"
)

type Backend interface {
	// Refresh returns a new set of flags.
	// It also returns the age of these flags, or an empty time if no age is known.
	Refresh() ([]Flag, time.Time, error)
}

type csvFileBackend struct {
	filename string
}

type jsonFileBackend struct {
	filename string
}

type flagJson struct {
	Name   string
	Active bool
	Rate   float64
	Rules  []RuleInfo
}

type ruleInfoJson struct {
	Type    string     `json:"type"`
	OnMatch RuleAction `json:"on_match"`
	OnMiss  RuleAction `json:"on_miss"`
}

type JSONFormat struct {
	Flags       []Flag  `json:"flags"`
	UpdatedTime float64 `json:"updated"`
}

// While the goforit client allows for complex feature flag functionality, it is possible to have
//simple flags that specify only Name and Rate (at least for the time being).Instead of using
// versions to formalize this, we will write some simple logic in a custom Unmarshaler to handle
// both cases
func (ri *Flag) UnmarshalJSON(buf []byte) error {
	var raw flagJson
	err := json.Unmarshal(buf, &raw)
	if err != nil {
		return err
	}
	if len(raw.Rules) == 0 {
		// if no rules are specified, we create a RateRule if a non-zero rate was specified, and ensure
		// the flag is active. if no rate was specified, active should be default to false
		if raw.Rate > 0 {
			raw.Active = true
			raw.Rules = []RuleInfo{
				{&RateRule{Rate: raw.Rate}, RuleOn, RuleOff},
			}
		}
	}

	ri.Name = raw.Name
	ri.Active = raw.Active
	ri.Rules = raw.Rules

	return nil
}

func (ri *RuleInfo) UnmarshalJSON(buf []byte) error {
	var raw ruleInfoJson
	err := json.Unmarshal(buf, &raw)
	if err != nil {
		return err
	}

	// Validate actions
	if !validRuleActions[raw.OnMatch] {
		return errors.New("Bad action") // TODO: make a custom error type
	}
	if !validRuleActions[raw.OnMiss] {
		return errors.New("Bad action") // TODO: make a custom error type
	}
	ri.OnMatch = raw.OnMatch
	ri.OnMiss = raw.OnMiss

	// Handle the type
	switch raw.Type {
	case "match_list": // TODO: constant
		ri.Rule = &MatchListRule{}
	case "sample": // TODO: constant
		ri.Rule = &RateRule{}
	default:
		return errors.New("Bad type") // TODO: custom error type
	}

	return json.Unmarshal(buf, ri.Rule)
}

func readFile(file string, backend string, parse func(io.Reader) ([]Flag, time.Time, error)) ([]Flag, time.Time, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	return parse(f)
}

func (b jsonFileBackend) Refresh() ([]Flag, time.Time, error) {
	return readFile(b.filename, "json", parseFlagsJSON)
}

func (b csvFileBackend) Refresh() ([]Flag, time.Time, error) {
	return readFile(b.filename, "csv", parseFlagsCSV)
}

func parseFlagsCSV(r io.Reader) ([]Flag, time.Time, error) {
	// every row is guaranteed to have 2 fields
	const FieldsPerRecord = 2

	cr := csv.NewReader(r)
	cr.FieldsPerRecord = FieldsPerRecord
	cr.TrimLeadingSpace = true

	rows, err := cr.ReadAll()
	if err != nil {
		return nil, time.Time{}, err
	}

	flags := make([]Flag, 0, len(rows))
	for _, row := range rows {
		name := row[0]

		rate, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			// TODO also track somehow
			rate = 0
		}

		f := Flag{
			Name:   name,
			Active: true,
		}
		if rate != 1 {
			f.Rules = []RuleInfo{
				{&RateRule{Rate: rate}, RuleOn, RuleOff},
			}
		}
		flags = append(flags, f)
	}
	return flags, time.Time{}, nil
}

func parseFlagsJSON(r io.Reader) ([]Flag, time.Time, error) {
	dec := json.NewDecoder(r)
	var v JSONFormat
	err := dec.Decode(&v)
	if err != nil {
		return nil, time.Time{}, err
	}
	return v.Flags, time.Unix(int64(v.UpdatedTime), 0), nil
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
