package goforit

import (
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// arbitrary but fixed for reproducible testing
const seed = 5194304667978865136

const ε = .02

func Reset() {
	Rand = rand.New(rand.NewSource(seed))
	flags = map[string]Flag{}
	flagsMtx = sync.RWMutex{}
}

func TestParseFlagsCSV(t *testing.T) {
	filename := filepath.Join("fixtures", "flags_example.csv")

	type testcase struct {
		Name     string
		Filename string
		Expected []Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("fixtures", "flags_example.csv"),
			Expected: []Flag{
				{
					"go.sun.money",
					0,
				},
				{
					"go.moon.mercury",
					1,
				},
				{
					"go.stars.money",
					0.5,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			f, err := os.Open(filename)
			assert.NoError(t, err)
			defer f.Close()

			flags, err := parseFlagsCSV(f)

			assertFlagsEqual(t, flagsToMap(tc.Expected), flags)
		})
	}
}

func TestEnabled(t *testing.T) {
	const iterations = 100000

	Reset()
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	ticker := Init(DefaultInterval, backend)
	defer ticker.Stop()

	assert.False(t, Enabled("go.sun.money"))
	assert.True(t, Enabled("go.moon.mercury"))

	count := 0
	for i := 0; i < iterations; i++ {
		if Enabled("go.stars.money") {
			count++
		}
	}
	actualRate := float64(count) / float64(iterations)

	assert.InEpsilon(t, 0.5, actualRate, ε)
}

// dummyBackend lets us test the RefreshFlags
// by returning the flags only the second time the Refresh
// method is called
type dummyBackend struct {
	// tally how many times Refresh() has been called
	refreshedCount int
}

func (b *dummyBackend) Refresh() (map[string]Flag, error) {
	defer func() {
		b.refreshedCount++
	}()

	if b.refreshedCount == 0 {
		return map[string]Flag{}, nil
	}

	f, err := os.Open(filepath.Join("fixtures", "flags_example.csv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseFlagsCSV(f)
}

func TestRefresh(t *testing.T) {
	Reset()
	backend := &dummyBackend{}

	assert.False(t, Enabled("go.sun.money"))
	assert.False(t, Enabled("go.moon.mercury"))

	ticker := Init(10*time.Millisecond, backend)
	defer ticker.Stop()

	// ensure refresh runs twice to avoid race conditions
	// in which the Refresh method returns but the assertions get called
	// before the flags are actually updated
	for backend.refreshedCount < 2 {
		<-time.After(10 * time.Millisecond)
	}

	assert.False(t, Enabled("go.sun.money"))
	assert.True(t, Enabled("go.moon.mercury"))
}

func TestMultipleDefinitions(t *testing.T) {
	const repeatedFlag = "go.sun.money"
	const lastValue = 0.7
	Reset()

	backend := BackendFromFile(filepath.Join("fixtures", "flags_multiple_definitions.csv"))
	RefreshFlags(backend)

	flag := flags[repeatedFlag]
	assert.Equal(t, flag, Flag{repeatedFlag, lastValue})

}

// BenchmarkEnabled runs a benchmark for a feature flag
// that is enabled for 50% of operations.
func BenchmarkEnabled(b *testing.B) {
	Reset()
	backend := BackendFromFile(filepath.Join("fixtures", "flags_example.csv"))
	ticker := Init(DefaultInterval, backend)
	defer ticker.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Enabled("go.stars.money")
	}
}

// assertFlagsEqual is a helper function for asserting
// that two maps of flags are equal
func assertFlagsEqual(t *testing.T, expected, actual map[string]Flag) {
	assert.Equal(t, len(expected), len(actual))

	for k, v := range expected {
		assert.Equal(t, v, actual[k])
	}
}
