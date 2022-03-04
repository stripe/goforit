package goforit

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// Is a flag clamped to on or off, or can it vary?
type FlagClamp int

const (
	FlagAlwaysOff FlagClamp = iota
	FlagAlwaysOn
	FlagMayVary
)

type Flag interface {
	FlagName() string
	Enabled(rnd randFloater, properties map[string]string) (bool, error)
	Equal(other Flag) bool

	// Yield if a flag is always on/off, for optimization
	Clamp() FlagClamp
}

type Flag1 struct {
	Name   string
	Active bool
	Rules  []RuleInfo
}

func (f Flag1) FlagName() string {
	return f.Name
}

func (f Flag1) Equal(other Flag) bool {
	o, ok := other.(Flag1)
	if !ok {
		return false
	}

	if f.Name != o.Name || f.Active != o.Active || len(f.Rules) != len(o.Rules) {
		return false
	}
	for i := 0; i < len(f.Rules); i++ {
		if f.Rules[i] != o.Rules[i] {
			return false
		}
	}
	return true
}

type RuleAction string

const (
	RuleOn       RuleAction = "on"
	RuleOff      RuleAction = "off"
	RuleContinue RuleAction = "continue"
)

var validRuleActions = map[RuleAction]bool{
	RuleOn:       true,
	RuleOff:      true,
	RuleContinue: true,
}

type RuleInfo struct {
	Rule    Rule
	OnMatch RuleAction
	OnMiss  RuleAction
}

type Rule interface {
	Handle(rnd randFloater, flag string, props map[string]string) (bool, error)
}

type MatchListRule struct {
	Property string
	Values   []string
}

type RateRule struct {
	Rate       float64
	Properties []string
}

func (f Flag1) Clamp() FlagClamp {
	if !f.Active {
		return FlagAlwaysOff
	}
	if len(f.Rules) == 0 {
		return FlagAlwaysOn
	}
	if len(f.Rules) == 1 {
		rule := f.Rules[0]
		if rate, ok := rule.Rule.(*RateRule); ok {
			action := RuleContinue
			if rate.Rate <= 0.0 {
				action = rule.OnMiss
			} else if rate.Rate >= 1.0 {
				action = rule.OnMatch
			}
			if action == RuleOn {
				return FlagAlwaysOn
			} else if action == RuleOff {
				return FlagAlwaysOff
			}
		}
	}
	return FlagMayVary
}

func (flag Flag1) Enabled(rnd randFloater, properties map[string]string) (bool, error) {
	// if flag is inactive, always return false
	if !flag.Active {
		return false, nil
	}
	// if there are no rules, but flag is active, always return true
	if len(flag.Rules) == 0 {
		return true, nil
	}

	for _, r := range flag.Rules {
		res, err := r.Rule.Handle(rnd, flag.Name, properties)
		if err != nil {
			return false, fmt.Errorf("error evaluating rule:\n %v", err)
		}
		var matchBehavior RuleAction
		if res {
			matchBehavior = r.OnMatch
		} else {
			matchBehavior = r.OnMiss
		}
		switch matchBehavior {
		case RuleOn:
			return true, nil
		case RuleOff:
			return false, nil
		case RuleContinue:
			continue
		default:
			return false, fmt.Errorf("unknown match behavior: " + string(matchBehavior))
		}
	}
	return false, nil
}

func getProperty(props map[string]string, prop string) (string, error) {
	if v, ok := props[prop]; ok {
		return v, nil
	} else {
		return "", errors.New("No property " + prop + " in properties map or default tags.")
	}
}

func (r *RateRule) Handle(rnd randFloater, flag string, props map[string]string) (bool, error) {
	if r.Properties != nil {
		// get the sha1 of the properties values concat
		h := sha1.New()
		// sort the properties for consistent behavior
		sort.Strings(r.Properties)
		var buffer bytes.Buffer
		buffer.WriteString(flag)
		for _, val := range r.Properties {
			buffer.WriteString("\000")
			prop, err := getProperty(props, val)
			if err != nil {
				return false, err
			}
			buffer.WriteString(prop)
		}
		h.Write([]byte(buffer.String()))
		bs := h.Sum(nil)
		// get the most significant 32 digits
		x := binary.BigEndian.Uint32(bs)
		// check to see if the 32 most significant bits of the hex
		// is less than (rate * 2^32)
		return float64(x) < (r.Rate * float64(1<<32)), nil
	} else {
		f := rnd.Float64()
		return f < r.Rate, nil
	}
}

func (r *MatchListRule) Handle(rnd randFloater, flag string, props map[string]string) (bool, error) {
	prop, err := getProperty(props, r.Property)
	if err != nil {
		return false, err
	}
	for _, val := range r.Values {
		if val == prop {
			return true, nil
		}
	}
	return false, nil
}
