package goforit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFlagsCSV(t *testing.T) {
	t.Parallel()

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
					true,
					[]RuleInfo{{&RateRule{Rate: 0}, RuleOn, RuleOff}},
					nil,
				},
				{
					"go.moon.mercury",
					true,
					nil,
					nil,
				},
				{
					"go.stars.money",
					true,
					[]RuleInfo{{&RateRule{Rate: 0.5}, RuleOn, RuleOff}},
					nil,
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

	filename := filepath.Join("fixtures", "flags_example.json")

	type testcase struct {
		Name     string
		Filename string
		Expected []Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("fixtures", "flags_example.json"),
			Expected: []Flag{
				{
					"go.sun.moon",
					true,
					[]RuleInfo{
						{&MatchListRule{"host_name", []string{"apibox_123", "apibox_456"}}, RuleOff, RuleContinue},
						{&MatchListRule{"host_name", []string{"apibox_789"}}, RuleOn, RuleContinue},
						{&RateRule{0.01, []string{"cluster", "db"}}, RuleOn, RuleOff},
					},
					nil,
				},
				{
					"go.sun.mercury",
					true,
					[]RuleInfo{
						{&RateRule{Rate: 0.5}, RuleOn, RuleOff},
					},
					nil,
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

	backend := BackendFromFile(filepath.Join("fixtures", "flags_multiple_definitions.csv"))
	g, _ := testGoforit(0, backend, enabledTickerInterval)
	g.RefreshFlags(backend)

	flag, ok := g.flags[repeatedFlag]
	assert.True(t, ok)
	flag.enabledTicker = nil // we don't compare about comparing this
	assert.Equal(t, flag, Flag{repeatedFlag, true, []RuleInfo{{&RateRule{Rate: lastValue}, RuleOn, RuleOff}}, nil})

}
