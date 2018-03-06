package goforit

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"reflect"
)

// ConditionJSONVersion is the supported version of the JSON file format
const ConditionJSONVersion = 1

// ErrConditionTypeUnknown indicates an unknown condition type seen when decoding
type ErrConditionTypeUnknown struct {
	Type string
}

func (e ErrConditionTypeUnknown) Error() string {
	return fmt.Sprintf("Unknown condition type %s", e.Type)
}

// ErrConditionActionUnknown indicates an unknown action seen when decoding
type ErrConditionActionUnknown struct {
	Action string
}

func (e ErrConditionActionUnknown) Error() string {
	return fmt.Sprintf("Unknown condition action %s", e.Action)
}

// ErrConditionJSONVersion indicates a bad version of a condition JSON file
type ErrConditionJSONVersion struct {
	Version int
}

func (e ErrConditionJSONVersion) Error() string {
	return fmt.Sprintf("Unknown condition JSON file version %d", e.Version)
}

var conditionTypes map[string]Condition = map[string]Condition{}

// ConditionRegister registers a new type of condition.
// The `template` parameter should be a pointer that implements Condition.
func ConditionRegister(typeName string, template Condition) {
	if reflect.TypeOf(template).Kind() != reflect.Ptr {
		panic("Attempt to register non-pointer condition sample")
	}
	conditionTypes[typeName] = template
}

type conditionInfoRaw struct {
	Type    string          `json:"type"`
	OnMatch ConditionAction `json:"on_match"`
	OnMiss  ConditionAction `json:"on_miss"`
}

func (c *conditionInfoRaw) validateActions() error {
	// By default, keep trying conditions until one matches
	if c.OnMatch == "" {
		c.OnMatch = ConditionEnabled
	}
	if c.OnMiss == "" {
		c.OnMiss = ConditionNext
	}

	if _, ok := conditionActions[c.OnMatch]; !ok {
		return ErrConditionActionUnknown{string(c.OnMatch)}
	}
	if _, ok := conditionActions[c.OnMiss]; !ok {
		return ErrConditionActionUnknown{string(c.OnMiss)}
	}
	return nil
}

func (c *ConditionInfo) UnmarshalJSON(buf []byte) error {
	// Unmarshal the raw condition, to find the type and actions
	// This is not particularly efficient
	var raw conditionInfoRaw
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
		return ErrConditionTypeUnknown{raw.Type}
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

	// Yield a ConditionInfo
	c.OnMatch = raw.OnMatch
	c.OnMiss = raw.OnMiss
	c.Condition = cond
	return nil
}

// ConditionJSONFileFormat is a file format for reading JSON files with condition flags
type ConditionJSONFileFormat struct{}

type conditionJSONFile struct {
	Version int             `json:"version"`
	Updated float64         `json:"updated"`
	Flags   []ConditionFlag `json:"flags"`
}

func (c *conditionJSONFile) validate() error {
	if c.Version != ConditionJSONVersion {
		return ErrConditionJSONVersion{c.Version}
	}
	// TODO: Detect mis-spellings, eg: tag -> tags
	// Extra validation: Do actions make sense? Are any string empty?
	return nil
}

func (ConditionJSONFileFormat) Read(r io.Reader) ([]Flag, time.Time, error) {
	var conditionFile conditionJSONFile
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
	flags := []Flag{}
	for i := range conditionFile.Flags {
		flags = append(flags, &conditionFile.Flags[i])
	}

	// Decode the time
	sec, frac := math.Modf(conditionFile.Updated)
	t := time.Unix(int64(sec), int64(1e9*frac))

	return flags, t, nil
}
