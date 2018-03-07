package goforit

import (
	"errors"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"time"
)

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

type ruleInfoJson struct {
	Type string `json:"type"`
	OnMatch  RuleAction `json:"on_match"`
	OnMiss   RuleAction `json:"on_miss"`
}

type JSONFormat struct {
	Flags       []Flag `json:"flags"`
	UpdatedTime float64          `json:"updated"`
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

func readFile(file string, backend string, parse func(io.Reader) (map[string]Flag, time.Time, error)) (map[string]Flag, time.Time, error) {
	f, err := os.Open(file)
	if err != nil {
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

		flags[name] = Flag{Name: name, Active: true, Rules: []RuleInfo{{&RateRule{Rate: rate}, RuleOn, RuleOff}}}
	}
	return flags, time.Time{}, nil
}

func parseFlagsJSON(r io.Reader) (map[string]Flag, time.Time, error) {
	dec := json.NewDecoder(r)
	var v JSONFormat
	err := dec.Decode(&v)
	if err != nil {
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
