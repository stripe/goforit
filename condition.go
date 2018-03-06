package goforit

import "math/rand"

// A Condition determines whether a condition is true for a given flag
type Condition interface {
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
}

func (c *ConditionInList) Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error) {
	panic("implement me") // TODO
}

// ConditionSample is a condition that matches at a given rate
type ConditionSample struct {
	// Rate is the rate at which to match, from zero to one
	Rate float64 `json:"rate"`
	// Tags are the tags to use for matching. The same values of these tags will always result in the same
	// result.
	Tags []string `json:"tags"`
}

func (c *ConditionSample) Match(rnd *rand.Rand, flag string, tags map[string]string) (bool, error) {
	panic("implement me") // TODO
}

func init() {
	ConditionRegister("in_list", &ConditionInList{})
	ConditionRegister("sample", &ConditionSample{})
}
