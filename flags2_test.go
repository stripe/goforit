package goforit

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FlagTestCase2 struct {
	Flag     string
	Expected bool
	Attrs    map[string]*string
	Message  string
}
type FlagAcceptance2 struct {
	JSONFormat2
	TestCases []FlagTestCase2 `json:"test_cases"`
}

func TestFlags2Backend(t *testing.T) {
	t.Parallel()

	expectedFlags := []Flag{
		Flag2{Name: "off_flag", Seed: "seed_1", Rules: []Rule2{}},
		Flag2{
			Name:  "go.moon.mercury",
			Seed:  "seed_1",
			Rules: []Rule2{{HashBy: "_random", Percent: 1.0, Predicates: []Predicate2{}}},
		},
		Flag2{
			Name:  "go.stars.money",
			Seed:  "seed_1",
			Rules: []Rule2{{HashBy: "_random", Percent: 0.5, Predicates: []Predicate2{}}},
		},
		Flag2{
			Name: "flag5",
			Seed: "seed_1",
			Rules: []Rule2{
				{
					HashBy:  "token",
					Percent: 1.0,
					Predicates: []Predicate2{
						{Attribute: "token", Operation: OpIn, Values: map[string]bool{"id_1": true, "id_2": true}},
						{Attribute: "country", Operation: OpNotIn, Values: map[string]bool{"KP": true}},
					},
				},
				{HashBy: "token", Percent: 0.5, Predicates: []Predicate2{}},
			},
		},
	}

	backend := BackendFromJSONFile2(filepath.Join("fixtures", "flags2_example.json"))
	flags, updated, err := backend.Refresh()

	assert.NoError(t, err)
	assert.Equal(t, expectedFlags, flags)
	assert.Equal(t, int64(1584642857), updated.Unix())
}

func TestFlags2Acceptance(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags2_acceptance.json")
	buf, err := ioutil.ReadFile(path)
	require.NoError(t, err)

	var acceptanceData FlagAcceptance2
	err = json.Unmarshal(buf, &acceptanceData)
	require.NoError(t, err)

	var flags = map[string]Flag2{}
	for _, f := range acceptanceData.Flags {
		flags[f.Name] = f
	}

	for _, tc := range acceptanceData.TestCases {
		name := fmt.Sprintf("%s:%s", tc.Flag, tc.Message)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// We don't distinguish between missing/nil values
			attrs := map[string]string{}
			for k, v := range tc.Attrs {
				if v != nil {
					attrs[k] = *v
				}
			}

			actual, err := flags[tc.Flag].Enabled(nil, attrs)
			assert.NoError(t, err)
			assert.Equal(t, tc.Expected, actual, "%q %q", tc.Flag, tc.Attrs)
		})
	}
}
