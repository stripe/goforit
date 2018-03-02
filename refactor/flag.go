package refactor

import "math/rand"

// A Flag knows whether it's enabled or disabled
type Flag interface {
	// Name gets the name of this flag
	Name() string

	// Enabled checks whether this flag is enabled
	Enabled(rnd *rand.Rand, tags map[string]string) (bool, error)
}

// SampleFlag is a simple type of flag, that only does sampling
type SampleFlag struct {
	FlagName string
	Rate     float64
}

// Name yields the name of this flag
func (f SampleFlag) Name() string {
	return f.FlagName
}

// Enabled determines whether this flag is enabled
func (f SampleFlag) Enabled(rnd *rand.Rand, tags map[string]string) (bool, error) {
	return rnd.Float64() < f.Rate, nil
}
