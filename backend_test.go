package goforit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFlagsCSV(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("testdata", "flags_example.csv")

	type testcase struct {
		Name     string
		Filename string
		Expected []Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("testdata", "flags_example.csv"),
			Expected: []Flag{
				Flag1{
					"go.sun.money",
					true,
					[]RuleInfo{{&RateRule{Rate: 0}, RuleOn, RuleOff}},
				},
				Flag1{
					"go.moon.mercury",
					true,
					nil,
				},
				Flag1{
					"go.stars.money",
					true,
					[]RuleInfo{{&RateRule{Rate: 0.5}, RuleOn, RuleOff}},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			f, err := os.Open(filename)
			assert.NoError(t, err)
			defer f.Close()

			flags, _, err := parseFlagsCSV(f)

			assert.Equal(t, tc.Expected, flags)
		})
	}
}

func TestParseFlagsJSON(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("testdata", "flags_example.json")

	type testcase struct {
		Name     string
		Filename string
		Expected []Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("testdata", "flags_example.json"),
			Expected: []Flag{
				Flag1{
					"go.sun.moon",
					true,
					[]RuleInfo{
						{&MatchListRule{"host_name", []string{"apibox_123", "apibox_456"}}, RuleOff, RuleContinue},
						{&MatchListRule{"host_name", []string{"apibox_789"}}, RuleOn, RuleContinue},
						{&RateRule{0.01, []string{"cluster", "db"}}, RuleOn, RuleOff},
					},
				},
				Flag1{
					"go.sun.mercury",
					true,
					[]RuleInfo{
						{&RateRule{Rate: 0.5}, RuleOn, RuleOff},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			f, err := os.Open(filename)
			assert.NoError(t, err)
			defer f.Close()

			flags, _, err := parseFlagsJSON(f)

			assert.Equal(t, tc.Expected, flags)
		})
	}
}

func TestMultipleDefinitions(t *testing.T) {
	t.Parallel()

	const repeatedFlag = "go.sun.money"
	const lastValue = 0.7

	backend := BackendFromFile(filepath.Join("testdata", "flags_multiple_definitions.csv"))
	g, _ := testGoforit(0, backend, enabledTickerInterval)
	g.RefreshFlags(backend)

	flagHolder, ok := g.flags.Get(repeatedFlag)
	assert.True(t, ok)
	assert.Equal(t, flagHolder.flag, Flag1{repeatedFlag, true, []RuleInfo{{&RateRule{Rate: lastValue}, RuleOn, RuleOff}}})

}
