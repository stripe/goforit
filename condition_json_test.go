package goforit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConditionJsonSimple(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_condition_simple.json")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	flags, lastMod, err := ConditionJsonFileFormat{}.Read(file)
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

func TestParseConditionJsonFull(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_condition_example.json")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	flags, lastMod, err := ConditionJsonFileFormat{}.Read(file)
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

func TestParseConditionJsonErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		ErrType error
		Json    string
	}{
		{
			ErrConditionTypeUnknown{}, `{
				"version": 1,
				"flags": [
					{
						"name": "test",
						"active": true,
						"conditions": [
							{
								"type": "FAKE"
							}
						]
					}
				]
			}`,
		},
		{
			ErrConditionActionUnknown{}, `{
				"version": 1,
				"flags": [
					{
						"name": "test",
						"active": true,
						"conditions": [
							{
								"type": "in_list",
								"on_match": "FAKE"
							}
						]
					}
				]
			}`,
		},
		{
			ErrConditionJsonVersion{}, `{
				"version": 99,
				"flags": []
			}`,
		},
	}

	for _, tc := range testCases {
		name := fmt.Sprintf("%T", tc.ErrType)
		t.Run(name, func(t *testing.T) {
			buf := bytes.NewBufferString(tc.Json)
			flags, lastMod, err := ConditionJsonFileFormat{}.Read(buf)
			require.Empty(t, flags)
			require.Zero(t, lastMod)
			require.Error(t, err)
			require.IsType(t, tc.ErrType, err)
		})
	}
}
