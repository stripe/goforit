package goforit

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
	Values    []string
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
