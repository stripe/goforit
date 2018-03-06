package condition

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"time"

	"github.com/stripe/goforit"
)

// JsonVersion is the supported version of the JSON file format
const JsonVersion = 1

// ErrTypeUnknown indicates an unknown condition type seen when decoding
type ErrTypeUnknown struct {
	Type string
}

func (e ErrTypeUnknown) Error() string {
	return fmt.Sprintf("Unknown condition type %s", e.Type)
}

// ErrActionUnknown indicates an unknown action seen when decoding
type ErrActionUnknown struct {
	Action string
}

func (e ErrActionUnknown) Error() string {
	return fmt.Sprintf("Unknown condition action %s", e.Action)
}

// ErrVersion indicates a bad version of a condition JSON file
type ErrVersion struct {
	Version int
}

func (e ErrVersion) Error() string {
	return fmt.Sprintf("Unknown condition JSON file version %d", e.Version)
}

var conditionTypes = map[string]Condition{}

// Register registers a new type of condition.
// The `template` parameter should be a pointer that implements Condition.
func Register(typeName string, template Condition) {
	if reflect.TypeOf(template).Kind() != reflect.Ptr {
		panic("Attempt to register non-pointer condition sample")
	}
	conditionTypes[typeName] = template
}

type infoRaw struct {
	Type    string `json:"type"`
	OnMatch Action `json:"on_match"`
	OnMiss  Action `json:"on_miss"`
}

func (c *infoRaw) validateActions() error {
	// By default, keep trying conditions until one matches
	if c.OnMatch == "" {
		c.OnMatch = ActionFlagEnabled
	}
	if c.OnMiss == "" {
		c.OnMiss = ActionNext
	}

	if _, ok := knownActions[c.OnMatch]; !ok {
		return ErrActionUnknown{string(c.OnMatch)}
	}
	if _, ok := knownActions[c.OnMiss]; !ok {
		return ErrActionUnknown{string(c.OnMiss)}
	}
	return nil
}

func (c *Info) UnmarshalJSON(buf []byte) error {
	// Unmarshal the raw condition, to find the type and actions
	// This is not particularly efficient
	var raw infoRaw
	err := json.Unmarshal(buf, &raw)
	if err != nil {
		return err
	}
	err = raw.validateActions()
	if err != nil {
		return err
	}

	// Check the type
	cond, ok := conditionTypes[raw.Type]
	if !ok {
		return ErrTypeUnknown{raw.Type}
	}

	// Make a clone, so we can reuse this value
	val := reflect.ValueOf(cond)
	elem := val.Type().Elem()
	clone := reflect.New(elem)
	cond = clone.Interface().(Condition)

	// Unmarshal into the clone
	err = json.Unmarshal(buf, cond)
	if err != nil {
		return err
	}

	// Yield a Info
	c.OnMatch = raw.OnMatch
	c.OnMiss = raw.OnMiss
	c.Condition = cond
	return nil
}

// JsonFileFormat is a file format for reading JSON files with condition flags
type JsonFileFormat struct{}

type jsonFile struct {
	Version int     `json:"version"`
	Updated float64 `json:"updated"`
	Flags   []Flag  `json:"flags"`
}

func (c *jsonFile) validate() error {
	if c.Version != JsonVersion {
		return ErrVersion{c.Version}
	}
	// TODO: DisallowUnknownFields to detect mispellings, but it's 1.10 only
	// Extra validation: Do actions make sense? Are any strings empty?
	return nil
}

func (JsonFileFormat) Read(r io.Reader) ([]goforit.Flag, time.Time, error) {
	var conditionFile jsonFile
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&conditionFile)
	if err != nil {
		return nil, time.Time{}, err
	}

	err = conditionFile.validate()
	if err != nil {
		return nil, time.Time{}, err
	}

	// Convert to interface
	flags := []goforit.Flag{}
	for i := range conditionFile.Flags {
		conditionFile.Flags[i].Init()
		flags = append(flags, &conditionFile.Flags[i])
	}

	// Decode the time
	sec, frac := math.Modf(conditionFile.Updated)
	t := time.Unix(int64(sec), int64(1e9*frac))

	return flags, t, nil
}
