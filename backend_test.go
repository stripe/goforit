package goforit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stripe/goforit/flags"
	"github.com/stripe/goforit/flags1"
)

func TestParseFlagsCSV(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("testdata", "flags_example.csv")

	type testcase struct {
		Name     string
		Filename string
		Expected []flags.Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("testdata", "flags_example.csv"),
			Expected: []flags.Flag{
				flags1.Flag1{
					"go.sun.money",
					true,
					[]flags1.RuleInfo{{&flags1.RateRule{Rate: 0}, flags.RuleOn, flags.RuleOff}},
				},
				flags1.Flag1{
					"go.moon.mercury",
					true,
					nil,
				},
				flags1.Flag1{
					"go.stars.money",
					true,
					[]flags1.RuleInfo{{&flags1.RateRule{Rate: 0.5}, flags.RuleOn, flags.RuleOff}},
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
		Expected []flags.Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("testdata", "flags_example.json"),
			Expected: []flags.Flag{
				flags1.Flag1{
					"go.sun.moon",
					true,
					[]flags1.RuleInfo{
						{&flags1.MatchListRule{"host_name", []string{"apibox_123", "apibox_456"}}, flags.RuleOff, flags.RuleContinue},
						{&flags1.MatchListRule{"host_name", []string{"apibox_789"}}, flags.RuleOn, flags.RuleContinue},
						{&flags1.RateRule{0.01, []string{"cluster", "db"}}, flags.RuleOn, flags.RuleOff},
					},
				},
				flags1.Flag1{
					"go.sun.mercury",
					true,
					[]flags1.RuleInfo{
						{&flags1.RateRule{Rate: 0.5}, flags.RuleOn, flags.RuleOff},
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
	g, _ := testGoforit(0, backend, stalenessCheckInterval)
	defer func() { _ = g.Close() }()
	g.RefreshFlags(backend)

	flagHolder, ok := g.flags.Get(repeatedFlag)
	assert.True(t, ok)

	expected := flags1.Flag1{
		Name:   repeatedFlag,
		Active: true,
		Rules: []flags1.RuleInfo{
			{
				Rule:    &flags1.RateRule{Rate: lastValue},
				OnMatch: flags.RuleOn,
				OnMiss:  flags.RuleOff,
			},
		},
	}
	assert.Equal(t, expected, flagHolder.flag)
}

func TestTimestampFallback(t *testing.T) {
	backend := jsonFileBackend{
		filename: filepath.Join("testdata", "flags_example.json"),
	}
	_, updated, err := backend.Refresh()
	assert.NoError(t, err)
	assert.Equal(t, int64(1519247256), updated.Unix())

	backendNoTimestamp := jsonFileBackend{
		filename: filepath.Join("testdata", "flags_example_no_timestamp.json"),
	}
	_, updated, err = backendNoTimestamp.Refresh()
	assert.NoError(t, err)

	info, err := os.Stat(filepath.Join("testdata", "flags_example_no_timestamp.json"))
	assert.NoError(t, err)
	assert.Equal(t, info.ModTime(), updated)
}
