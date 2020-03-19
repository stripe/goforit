package goforit

import "encoding/json"

type Operation2 string

const (
	OpIn      Operation2 = "in"
	OpNotIn   Operation2 = "not_in"
	OpIsNil   Operation2 = "is_nil"
	OptNotNil Operation2 = "is_not_nil"
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

func (f Flag2) Evaluate(attributes map[string]string) bool {
	// TODO
	return true
}
