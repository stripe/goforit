package goforit

import (
	"github.com/stripe/goforit/flags2"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stripe/goforit/flags"
)

func TestParseFlagsJSON(t *testing.T) {
	t.Parallel()

	filename := filepath.Join("testdata", "flags2_example.json")

	type testcase struct {
		Name     string
		Filename string
		Expected []flags.Flag
	}

	cases := []testcase{
		{
			Name:     "BasicExample",
			Filename: filepath.Join("testdata", "flags2_example.json"),
			Expected: []flags.Flag{
				flags2.Flag2{
					Name:  "off_flag",
					Seed:  "seed_1",
					Rules: []flags2.Rule2{},
				},
				flags2.Flag2{
					Name: "go.moon.mercury",
					Seed: "seed_1",
					Rules: []flags2.Rule2{
						{
							HashBy:     "_random",
							Percent:    1.0,
							Predicates: []flags2.Predicate2{},
						},
					},
				},
				flags2.Flag2{
					Name: "go.stars.money",
					Seed: "seed_1",
					Rules: []flags2.Rule2{
						{
							HashBy:     "_random",
							Percent:    0.5,
							Predicates: []flags2.Predicate2{},
						},
					},
				},
				flags2.Flag2{
					Name: "go.sun.money",
					Seed: "seed_1",
					Rules: []flags2.Rule2{
						{
							HashBy:     "_random",
							Percent:    0.0,
							Predicates: []flags2.Predicate2{},
						},
					},
				},
				flags2.Flag2{
					Name: "flag5",
					Seed: "seed_1",
					Rules: []flags2.Rule2{
						{
							HashBy:  "token",
							Percent: 1.0,
							Predicates: []flags2.Predicate2{
								{
									Attribute: "token",
									Operation: flags2.OpIn,
									Values: map[string]bool{
										"id_1": true,
										"id_2": true,
									},
								},
								{
									Attribute: "country",
									Operation: flags2.OpNotIn,
									Values: map[string]bool{
										"KP": true,
									},
								},
							},
						},
						{
							HashBy:     "token",
							Percent:    0.5,
							Predicates: []flags2.Predicate2{},
						},
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

			flags, _, err := parseFlagsJSON2(f)

			assert.Equal(t, tc.Expected, flags)
		})
	}
}

func TestMultipleDefinitions(t *testing.T) {
	t.Parallel()

	const repeatedFlag = "go.sun.money"
	const lastValue = 0.7

	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_multiple_definitions.json"))
	g, _ := testGoforit(0, backend, stalenessCheckInterval)
	defer func() { _ = g.Close() }()
	g.RefreshFlags(backend)

	flagHolder, ok := g.flags.Get(repeatedFlag)
	assert.True(t, ok)

	expected := flags2.Flag2{
		Name: repeatedFlag,
		Seed: "seed_1",
		Rules: []flags2.Rule2{
			{
				HashBy:     "_random",
				Percent:    lastValue,
				Predicates: []flags2.Predicate2{},
			},
		},
	}
	assert.Equal(t, expected, flagHolder.flag)
}

func TestTimestampFallback(t *testing.T) {
	backend := jsonFileBackend2{
		filename: filepath.Join("testdata", "flags2_example.json"),
	}
	_, updated, err := backend.Refresh()
	assert.NoError(t, err)
	assert.Equal(t, int64(1584642857), updated.Unix())

	backendNoTimestamp := jsonFileBackend2{
		filename: filepath.Join("testdata", "flags2_example_no_timestamp.json"),
	}
	_, updated, err = backendNoTimestamp.Refresh()
	assert.NoError(t, err)

	info, err := os.Stat(filepath.Join("testdata", "flags2_example_no_timestamp.json"))
	assert.NoError(t, err)
	assert.Equal(t, info.ModTime(), updated)
}
