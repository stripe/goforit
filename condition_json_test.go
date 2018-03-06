package goforit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConditionJSONSimple(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_condition_simple.json")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	flags, lastMod, err := ConditionJSONFileFormat{}.Read(file)
	assert.NoError(t, err)
	assert.Equal(t, int64(1519247256), lastMod.Unix())

	expected := []Flag{
		&ConditionFlag{
			FlagName: "go.test",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionNext,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice"},
					},
				},
				{
					OnMatch: ConditionDisabled,
					OnMiss:  ConditionEnabled,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"bob"},
					},
				},
			},
		},
	}
	assert.Equal(t, expected, flags)
}

func TestParseConditionJSONFull(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_condition_example.json")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	flags, lastMod, err := ConditionJSONFileFormat{}.Read(file)
	assert.NoError(t, err)
	assert.Equal(t, int64(1519247256), lastMod.Unix())

	expected := []Flag{
		&ConditionFlag{
			FlagName: "go.off",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionNext,
					Condition: &ConditionSample{
						Tags: []string{},
						Rate: 0,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.on",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{},
						Rate: 1,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.sample",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{},
						Rate: 0.1,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.whitelist",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.double_whitelist",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionNext,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice"},
					},
				},
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionInList{
						Tag:    "currency",
						Values: []string{"usd", "cad"},
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.blacklist",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionDisabled,
					OnMiss:  ConditionEnabled,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.random_by",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{"server"},
						Rate: 0.1,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.random_by_multiple",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{"server", "dataset"},
						Rate: 0.1,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.random_by_with_whitelist",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionNext,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{"request_id"},
						Rate: 0.1,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.sample_from_whitelist",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionNext,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{"request_id"},
						Rate: 0.1,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.random_by_with_whitelist_and_blacklist",
			Active:   true,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionNext,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
				{
					OnMatch: ConditionDisabled,
					OnMiss:  ConditionNext,
					Condition: &ConditionInList{
						Tag:    "user",
						Values: []string{"carol"},
					},
				},
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{"request_id"},
						Rate: 0.2,
					},
				},
			},
		},
		&ConditionFlag{
			FlagName: "go.inactive",
			Active:   false,
			Conditions: []ConditionInfo{
				{
					OnMatch: ConditionEnabled,
					OnMiss:  ConditionDisabled,
					Condition: &ConditionSample{
						Tags: []string{},
						Rate: 0.1,
					},
				},
			},
		},
	}
	assert.Equal(t, expected, flags)
}

// TODO:

// Condition types
// Overall flag algorithm
// Unmarshaling
// Errors unmarshaling
