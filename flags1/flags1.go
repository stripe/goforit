package flags1

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/stripe/goforit/clamp"
	"github.com/stripe/goforit/flags"
)

type Flag1 struct {
	Name   string
	Active bool
	Rules  []RuleInfo
}

func (f Flag1) FlagName() string {
	return f.Name
}

func (f Flag1) Equal(other flags.Flag) bool {
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

type flagJson struct {
	Name   string
	Active bool
	Rate   float64
	Rules  []RuleInfo
}

// While the goforit client allows for complex feature flag functionality, it is possible to have
// simple flags that specify only Name and Rate (at least for the time being).Instead of using
// versions to formalize this, we will write some simple logic in a custom Unmarshaler to handle
// both cases
func (ri *Flag1) UnmarshalJSON(buf []byte) error {
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
				{&RateRule{Rate: raw.Rate}, flags.RuleOn, flags.RuleOff},
			}
		}
	}

	ri.Name = raw.Name
	ri.Active = raw.Active
	ri.Rules = raw.Rules

	return nil
}

type ruleInfoJson struct {
	Type    string           `json:"type"`
	OnMatch flags.RuleAction `json:"on_match"`
	OnMiss  flags.RuleAction `json:"on_miss"`
}

func (ri *RuleInfo) UnmarshalJSON(buf []byte) error {
	var raw ruleInfoJson
	err := json.Unmarshal(buf, &raw)
	if err != nil {
		return err
	}

	// Validate actions
	if !validRuleActions[raw.OnMatch] {
		return errors.New("bad action") // TODO: make a custom error type
	}
	if !validRuleActions[raw.OnMiss] {
		return errors.New("bad action") // TODO: make a custom error type
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

var validRuleActions = map[flags.RuleAction]bool{
	flags.RuleOn:       true,
	flags.RuleOff:      true,
	flags.RuleContinue: true,
}

type RuleInfo struct {
	Rule    Rule
	OnMatch flags.RuleAction
	OnMiss  flags.RuleAction
}

type Rule interface {
	Handle(rnd flags.RandFloater, flag string, props map[string]string) (bool, error)
}

type MatchListRule struct {
	Property string
	Values   []string
}

type RateRule struct {
	Rate       float64
	Properties []string
}

func (f Flag1) Clamp() clamp.Clamp {
	if !f.Active {
		return clamp.AlwaysOff
	}
	if len(f.Rules) == 0 {
		return clamp.AlwaysOn
	}
	if len(f.Rules) == 1 {
		rule := f.Rules[0]
		if rate, ok := rule.Rule.(*RateRule); ok {
			action := flags.RuleContinue
			if rate.Rate <= 0.0 {
				action = rule.OnMiss
			} else if rate.Rate >= 1.0 {
				action = rule.OnMatch
			}
			if action == flags.RuleOn {
				return clamp.AlwaysOn
			} else if action == flags.RuleOff {
				return clamp.AlwaysOff
			}
		}
	}
	return clamp.MayVary
}

func (flag Flag1) Enabled(rnd flags.RandFloater, properties map[string]string) (bool, error) {
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
		var matchBehavior flags.RuleAction
		if res {
			matchBehavior = r.OnMatch
		} else {
			matchBehavior = r.OnMiss
		}
		switch matchBehavior {
		case flags.RuleOn:
			return true, nil
		case flags.RuleOff:
			return false, nil
		case flags.RuleContinue:
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

func (r *RateRule) Handle(rnd flags.RandFloater, flag string, props map[string]string) (bool, error) {
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

func (r *MatchListRule) Handle(rnd flags.RandFloater, flag string, props map[string]string) (bool, error) {
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
