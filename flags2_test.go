package goforit

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type FlagTestCase2 struct {
	Flag     string
	Expected bool
	Attrs    map[string]string
	Message  string
}
type FlagAcceptance2 struct {
	FlagFile2
	TestCases []FlagTestCase2 `json:"test_cases"`
}

func TestFlags2Parse(t *testing.T) {
	t.Parallel()

	jsonText := `
{
  "version": 1,
  "flags": [
    {
      "name": "off_flag",
      "_id": "ff_1",
      "seed": "seed_1",
      "rules": []
    },
    {
      "name": "flag5",
      "_id": "ff_5",
      "seed": "seed_1",
      "rules": [
        {"hash_by": "token", "percent": 1.0, "predicates": [
          {"attribute": "token", "operation": "in", "values": ["id_1", "id_2"]},
          {"attribute": "country", "operation": "not_in", "values": ["KP"]}
        ]},
        {"hash_by": "token", "percent": 0.5, "predicates": []}	
      ]
    }
  ]
}
`
	expected := FlagFile2{
		Flags: []Flag2{
			{Name: "off_flag", Seed: "seed_1", Rules: []Rule2{}},
			{
				Name: "flag5",
				Seed: "seed_1",
				Rules: []Rule2{
					{
						HashBy:  "token",
						Percent: 1.0,
						Predicates: []Predicate2{
							{Attribute: "token", Operation: OpIn, Values: []string{"id_1", "id_2"}},
							{Attribute: "country", Operation: OpNotIn, Values: []string{"KP"}},
						},
					},
					{HashBy: "token", Percent: 0.5, Predicates: []Predicate2{}},
				},
			},
		},
	}

	var file FlagFile2
	err := json.Unmarshal([]byte(jsonText), &file)
	assert.NoError(t, err)
	assert.Equal(t, expected, file)
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
		t.Run(tc.Message, func(t *testing.T) {
			actual := flags[tc.Flag].Evaluate(tc.Attrs)
			assert.Equal(t, tc.Expected, actual)
		})
	}
}
