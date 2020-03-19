package goforit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
