package condition

import (
	"crypto/sha1"
	"encoding/binary"
	"io"
	"math"
	"math/rand"
	"sort"

	"github.com/stripe/goforit"
)

// A Condition determines whether a condition is true for a given flag
type Condition interface {
	// Init allows this condition to do any initialization it needs
	Init()

	// Match matches the flag and tags against this condition
	Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error)
}

// Action specifies what to do when a condition is executed
type Action string

const (
	// ActionNext indicates that the next condition in sequence should be executed
	ActionNext Action = "next"
	// ActionFlagEnabled indicates that the flag should be enabled
	ActionFlagEnabled Action = "enabled"
	// ActionFlagDisabled indicates that the flag should be disabled
	ActionFlagDisabled Action = "disabled"
)

var knownActions = map[Action]bool{
	ActionNext:         true,
	ActionFlagEnabled:  true,
	ActionFlagDisabled: true,
}

// Info is a condition, plus information about what to do when it is true
type Info struct {
	// Condition is the condition to execute in this step
	Condition Condition
	// OnMatch is the action to take if the condition matches
	OnMatch Action
	// OnMiss is the action to take if the condition does not match
	OnMiss Action
}

// Flag is a more complex flag, that applies a list of conditions
type Flag struct {
	// FlagName is the name of the given flag
	FlagName string `json:"name"`
	// Active is whether or not the flag is active
	Active bool `json:"active"`
	// Conditions are the conditions to be applied, in order to check if the flag is enabled
	Conditions []Info `json:"conditions"`
}

func (f *Flag) Name() string {
	return f.FlagName
}

// Init gets this flag ready for use
func (f *Flag) Init() {
	for _, cond := range f.Conditions {
		cond.Condition.Init()
	}
}

func (f *Flag) Enabled(rnd *rand.Rand, tags map[string]string) (bool, error) {
	if !f.Active {
		return false, nil
	}

	// Try each condition in sequence
	for _, cond := range f.Conditions {
		action := ActionNext
		match, err := cond.Condition.Match(rnd, f.Name(), tags)
		if err != nil {
			return false, err
		}

		// Take an action
		if match {
			action = cond.OnMatch
		} else {
			action = cond.OnMiss
		}
		if action == ActionFlagEnabled {
			return true, nil
		} else if action == ActionFlagDisabled {
			return false, nil
		}
		// Otherwise, keep going
	}

	// If we get to the end, that's equivalent to false
	return false, nil
}

// InList is a condition that matches a tag against a list of values
type InList struct {
	// Tag is the tag to match
	Tag string `json:"tag"`
	// Values are the values to match against
	Values []string `json:"values"`

	values map[string]bool
}

func (c *InList) Init() {
	c.values = map[string]bool{}
	for _, v := range c.Values {
		c.values[v] = true
	}
}

func (c *InList) Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error) {
	value, ok := tags[c.Tag]
	if !ok {
		return false, goforit.ErrMissingTag{flag, c.Tag}
	}
	return c.values[value], nil
}

// Sample is a condition that matches at a given rate
type Sample struct {
	// Rate is the rate at which to match, from zero to one
	Rate float64 `json:"rate"`
	// Tags are the tags to use for matching. The same values of these tags will always result in the same
	// result.
	Tags []string `json:"tags"`
}

func (c *Sample) Init() {
	// Sort the tags
	sort.Strings(c.Tags)
}

func (c *Sample) Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error) {
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
			return false, goforit.ErrMissingTag{flag, k}
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
	Register("in_list", &InList{})
	Register("sample", &Sample{})
}
