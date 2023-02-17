package flags

import (
	"github.com/stripe/goforit/clamp"
)

// Flag is the interface for individual feature flags
type Flag interface {
	FlagName() string
	Enabled(rnd Rand, properties, defaultTags map[string]string) (bool, error)
	Equal(other Flag) bool
	// Clamp returns whether a flag is always on/off, for optimization
	Clamp() clamp.Clamp
}

// DeletableFlag can report whether this flag is scheduled for deletion
type DeletableFlag interface {
	// IsDeleted yields true if this flag is scheduled for deletion
	IsDeleted() bool
}

type RuleAction string

const (
	RuleOn       RuleAction = "on"
	RuleOff      RuleAction = "off"
	RuleContinue RuleAction = "continue"
)
