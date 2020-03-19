package goforit

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
)

type Operation2 string
type Attributes2 map[string]string

const (
	OpIn      = "in"
	OpNotIn   = "not_in"
	OpIsNil   = "is_nil"
	OptNotNil = "is_not_nil"

	PercentOn  = 1.0
	PercentOff = 0.0

	HashByRandom = "_random"
)

type Predicate2 struct {
	Attribute string
	Operation Operation2
	Values    map[string]bool
}
type Rule2 struct {
	HashBy     string `json:"hash_by"`
	Percent    float64
	Predicates []Predicate2
}
type Flag2 struct {
	Name  string
	Seed  string
	Rules []Rule2
}
type FlagFile2 struct {
	Flags []Flag2
}

type predicate2Json struct {
	Attribute string
	Operation Operation2
	Values    []string
}

func (p *Predicate2) UnmarshalJSON(data []byte) error {
	var raw predicate2Json
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	*p = Predicate2{Attribute: raw.Attribute, Operation: raw.Operation, Values: map[string]bool{}}
	for _, v := range raw.Values {
		p.Values[v] = true
	}
	return nil
}

func (p Predicate2) matches(attributes Attributes2) (bool, error) {
	val, present := attributes[p.Attribute]
	switch p.Operation {
	case OpIn:
		return p.Values[val], nil
	case OpNotIn:
		return !p.Values[val], nil
	case OpIsNil:
		return !present, nil
	case OptNotNil:
		return present, nil
	default:
		return false, fmt.Errorf("unknown predicate %q", p.Operation)
	}
}

func (r Rule2) matches(attributes Attributes2) (bool, error) {
	_, hashPresent := attributes[r.HashBy]
	if !hashPresent && r.HashBy != HashByRandom && r.Percent > PercentOff && r.Percent < PercentOn {
		// We have no way to calculate a percentage, so the specced behavior is to skip this rule
		return false, nil
	}

	for _, pred := range r.Predicates {
		match, err := pred.matches(attributes)
		if err != nil {
			return false, err
		}
		// ALL predicates must match
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func (r Rule2) hashValue(seed, val string) float64 {
	h := sha1.New()
	h.Write([]byte(seed))
	h.Write([]byte("."))
	h.Write([]byte(val))
	sum := h.Sum(nil)
	ival := binary.BigEndian.Uint16(sum[:])
	return float64(ival) / float64(1<<16)
}

func (r Rule2) evaluate(seed string, attributes Attributes2) (bool, error) {
	if r.Percent >= PercentOn {
		return true, nil
	}
	if r.Percent <= PercentOff {
		return false, nil
	}

	if r.HashBy == HashByRandom {
		return rand.Float64() < r.Percent, nil
	}

	val := attributes[r.HashBy]
	return r.hashValue(seed, val) < r.Percent, nil
}

func (f Flag2) Evaluate(attributes Attributes2) (bool, error) {
	for _, rule := range f.Rules {
		match, err := rule.matches(attributes)
		if err != nil {
			return false, err
		}
		if !match {
			continue
		}

		return rule.evaluate(f.Seed, attributes)
	}

	// If no rules match, the flag is off
	return false, nil
}
