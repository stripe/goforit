package flags2

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/stripe/goforit/clamp"
	"github.com/stripe/goforit/flags"
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

// Predicate2 is a newer, more sophisticated type of flag!
//
// Each Flag2 contains a list of rules, and each rule contains a list of predicates.
// When querying a flag, the first rule whose predicates match is applied.
type Predicate2 struct {
	Attribute string          `json:"attribute"`
	Operation Operation2      `json:"operation"`
	Values    map[string]bool `json:"values"`
}
type Rule2 struct {
	HashBy     string       `json:"hash_by"`
	Percent    float64      `json:"percent"`
	Predicates []Predicate2 `json:"predicates"`
}
type Flag2 struct {
	Name    string  `json:"name"`
	Seed    string  `json:"seed"`
	Rules   []Rule2 `json:"rules"`
	Deleted bool    `json:"deleted"`
}

type JSONFormat2 struct {
	Flags   []*Flag2 `json:"flags"`
	Updated float64  `json:"updated"`
}

type predicate2Json struct {
	Attribute string     `json:"attribute"`
	Operation Operation2 `json:"operation"`
	Values    []string   `json:"values"`
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

func (p *Predicate2) MarshalJSON() ([]byte, error) {
	raw := predicate2Json{
		Attribute: p.Attribute,
		Operation: p.Operation,
	}

	if len(p.Values) > 0 {
		raw.Values = make([]string, 0, len(p.Values))
		for k := range p.Values {
			raw.Values = append(raw.Values, k)
		}
		sort.Strings(raw.Values)
	}

	return json.Marshal(&raw)
}

func (f Flag2) FlagName() string {
	return f.Name
}

func (f Flag2) Enabled(rnd flags.Rand, properties, defaultTags map[string]string) (bool, error) {
	for _, rule := range f.Rules {
		match, err := rule.matches(properties, defaultTags)
		if err != nil {
			return false, err
		}
		if !match {
			continue
		}

		return rule.evaluate(rnd, f.Seed, properties, defaultTags)
	}

	// If no rules match, the flag is off
	return false, nil
}

func (f Flag2) Clamp() clamp.Clamp {
	if len(f.Rules) == 0 {
		return clamp.AlwaysOff
	}
	if len(f.Rules) == 1 && len(f.Rules[0].Predicates) == 0 {
		if f.Rules[0].Percent <= PercentOff {
			return clamp.AlwaysOff
		} else if f.Rules[0].Percent >= PercentOn {
			return clamp.AlwaysOn
		}
	}
	return clamp.MayVary
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

func (f Flag2) Equal(o *Flag2) bool {
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

func (f Flag2) IsDeleted() bool {
	return f.Deleted
}

func (p Predicate2) matches(properties, defaultTags map[string]string) (bool, error) {
	val, present := properties[p.Attribute]
	if !present {
		val, present = defaultTags[p.Attribute]
	}
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

func (r Rule2) matches(properties, defaultTags map[string]string) (bool, error) {
	_, hashPresent := properties[r.HashBy]
	if !hashPresent {
		_, hashPresent = defaultTags[r.HashBy]
	}
	if !hashPresent && r.HashBy != HashByRandom && r.Percent > PercentOff && r.Percent < PercentOn {
		// We have no way to calculate a percentage, so the specced behavior is to skip this rule
		return false, nil
	}

	for _, pred := range r.Predicates {
		match, err := pred.matches(properties, defaultTags)
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
	h.Write([]byte{'.'})
	h.Write([]byte(val))
	buf := make([]byte, 0, sha1.Size)
	sum := h.Sum(buf)
	ival := binary.BigEndian.Uint16(sum)
	return float64(ival) / float64(1<<16)
}

func (r Rule2) evaluate(rnd flags.Rand, seed string, properties, defaultTags map[string]string) (bool, error) {
	if r.Percent >= PercentOn {
		return true, nil
	}
	if r.Percent <= PercentOff {
		return false, nil
	}

	if r.HashBy == HashByRandom {
		return rnd.Float64() < r.Percent, nil
	}

	val, found := properties[r.HashBy]
	if !found {
		val = defaultTags[r.HashBy]
	}
	return r.hashValue(seed, val) < r.Percent, nil
}
