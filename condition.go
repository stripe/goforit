package goforit

import (
	"crypto/sha1"
	"encoding/binary"
	"io"
	"math"
	"math/rand"
	"sort"
)

// A Condition determines whether a condition is true for a given flag
type Condition interface {
	// Init allows this condition to do any initialization it needs
	Init()

	// Match matches the flag and tags against this condition
	Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error)
}

// ConditionAction specifies what to do when a condition is executed
type ConditionAction string

const (
	// ConditionNextRule indicates that the next condition in sequence should be executed
	ConditionNext ConditionAction = "next"
	// ConditionEnabled indicates that the flag should be enabled
	ConditionEnabled ConditionAction = "enabled"
	// ConditionDisabled indicates that the flag should be disabled
	ConditionDisabled ConditionAction = "disabled"
)

var conditionActions = map[ConditionAction]bool{
	ConditionNext:     true,
	ConditionEnabled:  true,
	ConditionDisabled: true,
}

// ConditionInfo is a condition, plus information about what to do when it is true
type ConditionInfo struct {
	// Condition is the condition to execute in this step
	Condition Condition
	// OnMatch is the action to take if the condition matches
	OnMatch ConditionAction
	// OnMiss is the action to take if the condition does not match
	OnMiss ConditionAction
}

// ConditionFlag is a more complex flag, that applies a list of conditions
type ConditionFlag struct {
	// FlagName is the name of the given flag
	FlagName string `json:"name"`
	// Active is whether or not the flag is active
	Active bool `json:"active"`
	// Conditions are the conditions to be applied, in order to check if the flag is enabled
	Conditions []ConditionInfo `json:"conditions"`
}

func (f *ConditionFlag) Name() string {
	return f.FlagName
}

func (f *ConditionFlag) Enabled(rnd *rand.Rand, tags map[string]string) (bool, error) {
	panic("implement me") // TODO
}

// ConditionInList is a condition that matches a tag against a list of values
type ConditionInList struct {
	// Tag is the tag to match
	Tag string `json:"tag"`
	// Values are the values to match against
	Values []string `json:"values"`

	values map[string]bool
}

func (c *ConditionInList) Init() {
	c.values = map[string]bool{}
	for _, v := range c.Values {
		c.values[v] = true
	}
}

func (c *ConditionInList) Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error) {
	value, ok := tags[c.Tag]
	if !ok {
		return false, ErrMissingTag{flag, c.Tag}
	}
	return c.values[value], nil
}

// ConditionSample is a condition that matches at a given rate
type ConditionSample struct {
	// Rate is the rate at which to match, from zero to one
	Rate float64 `json:"rate"`
	// Tags are the tags to use for matching. The same values of these tags will always result in the same
	// result.
	Tags []string `json:"tags"`
}

func (c *ConditionSample) Init() {
	// Sort the tags
	sort.Strings(c.Tags)
}

func (c *ConditionSample) Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error) {
	// Just a basic random sampling
	if len(c.Tags) == 0 {
		return rnd.Float64() < c.Rate, nil
	}

	// If we have tags, be deterministic based on their hash
	hash := sha1.New()
	io.WriteString(hash, flag)

	// Add tags to our hash, in order
	zero := []byte{0} // nil-separate everything
	for _, k := range c.Tags {
		hash.Write(zero)
		io.WriteString(hash, k)
		hash.Write(zero)
		v, ok := tags[k]
		if !ok {
			return false, ErrMissingTag{flag, k}
		}
		io.WriteString(hash, v)
	}

	// Turn our sum into a float
	buf := hash.Sum(nil)
	ival := binary.LittleEndian.Uint64(buf)
	fval := float64(ival) / float64(math.MaxUint64)

	return fval < c.Rate, nil
}

func init() {
	ConditionRegister("in_list", &ConditionInList{})
	ConditionRegister("sample", &ConditionSample{})
}
