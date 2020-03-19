package goforit

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

type Operation2 string

const (
	OpIn      = "in"
	OpNotIn   = "not_in"
	OpIsNil   = "is_nil"
	OptNotNil = "is_not_nil"

	PercentOn  = 1.0
	PercentOff = 0.0

	HashByRandom = "_random"
)

// A newer, more sophisticated type of flag!
//
// Each Flag2 contains a list of rules, and each rule contains a list of predicates.
// When querying a flag, the first rule whose predicates match is applied.
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

type JSONFormat2 struct {
	Flags   []Flag2
	Updated float64
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

func (f Flag2) FlagName() string {
	return f.Name
}

func (f Flag2) Enabled(rnd randFunc, properties map[string]string) (bool, error) {
	for _, rule := range f.Rules {
		match, err := rule.matches(properties)
		if err != nil {
			return false, err
		}
		if !match {
			continue
		}

		return rule.evaluate(rnd, f.Seed, properties)
	}

	// If no rules match, the flag is off
	return false, nil
}

func (p Predicate2) equal(o Predicate2) bool {
	if p.Attribute != o.Attribute || p.Operation != o.Operation || len(p.Values) != len(o.Values) {
		return false
	}

	for v := range p.Values {
		if !o.Values[v] {
			return false
		}
	}
	// Since cardinality is the same, the whole set must be the same
	return true
}

func (r Rule2) equal(o Rule2) bool {
	if r.HashBy != o.HashBy || r.Percent != o.Percent || len(r.Predicates) != len(o.Predicates) {
		return false
	}
	for i := range r.Predicates {
		if !r.Predicates[i].equal(o.Predicates[i]) {
			return false
		}
	}
	return true
}

func (f Flag2) Equal(other Flag) bool {
	o, ok := other.(Flag2)
	if !ok {
		return false
	}

	if f.Name != o.Name || f.Seed != o.Seed || len(f.Rules) != len(o.Rules) {
		return false
	}
	for i := range f.Rules {
		if !f.Rules[i].equal(o.Rules[i]) {
			return false
		}
	}
	return true
}

func (p Predicate2) matches(properties map[string]string) (bool, error) {
	val, present := properties[p.Attribute]
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

func (r Rule2) matches(properties map[string]string) (bool, error) {
	_, hashPresent := properties[r.HashBy]
	if !hashPresent && r.HashBy != HashByRandom && r.Percent > PercentOff && r.Percent < PercentOn {
		// We have no way to calculate a percentage, so the specced behavior is to skip this rule
		return false, nil
	}

	for _, pred := range r.Predicates {
		match, err := pred.matches(properties)
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

func (r Rule2) evaluate(rnd randFunc, seed string, properties map[string]string) (bool, error) {
	if r.Percent >= PercentOn {
		return true, nil
	}
	if r.Percent <= PercentOff {
		return false, nil
	}

	if r.HashBy == HashByRandom {
		return rnd() < r.Percent, nil
	}

	val := properties[r.HashBy]
	return r.hashValue(seed, val) < r.Percent, nil
}

func (b jsonFileBackend2) Refresh() ([]Flag, time.Time, error) {
	return readFile(b.filename, "json2", parseFlagsJSON2)
}

func parseFlagsJSON2(r io.Reader) ([]Flag, time.Time, error) {
	dec := json.NewDecoder(r)
	var v JSONFormat2
	err := dec.Decode(&v)
	if err != nil {
		return nil, time.Time{}, err
	}

	flags := make([]Flag, len(v.Flags))
	for i, f := range v.Flags {
		flags[i] = f
	}

	return flags, time.Unix(int64(v.Updated), 0), nil
}

// BackendFromJSONFile2 creates a v2 backend powered by a JSON file
func BackendFromJSONFile2(filename string) Backend {
	return jsonFileBackend2{filename}
}
