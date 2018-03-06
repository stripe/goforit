package condition

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/goforit"
)

func initConditionFlags(fs []goforit.Flag) {
	for _, f := range fs {
		f.(*Flag).Init()
	}
}

func TestParseConditionJsonSimple(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "fixtures", "flags_condition_simple.json")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	flags, lastMod, err := JsonFileFormat{}.Read(file)
	assert.NoError(t, err)
	assert.Equal(t, int64(1519247256), lastMod.Unix())

	expected := []goforit.Flag{
		&Flag{
			FlagName: "go.test",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionNext,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice"},
					},
				},
				{
					OnMatch: ActionFlagDisabled,
					OnMiss:  ActionFlagEnabled,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"bob"},
					},
				},
			},
		},
	}
	initConditionFlags(expected)
	assert.Equal(t, expected, flags)
}

func TestParseConditionJsonFull(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "fixtures", "flags_condition_example.json")
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	flags, lastMod, err := JsonFileFormat{}.Read(file)
	assert.NoError(t, err)
	assert.Equal(t, int64(1519247256), lastMod.Unix())

	expected := []goforit.Flag{
		&Flag{
			FlagName: "go.off",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionNext,
					Condition: &Sample{
						Tags: []string{},
						Rate: 0,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.on",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{},
						Rate: 1,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.sample",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{},
						Rate: 0.1,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.whitelist",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
			},
		},
		&Flag{
			FlagName: "go.double_whitelist",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionNext,
					OnMiss:  ActionFlagDisabled,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice"},
					},
				},
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &InList{
						Tag:    "currency",
						Values: []string{"usd", "cad"},
					},
				},
			},
		},
		&Flag{
			FlagName: "go.blacklist",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagDisabled,
					OnMiss:  ActionFlagEnabled,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
			},
		},
		&Flag{
			FlagName: "go.random_by",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{"server"},
						Rate: 0.1,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.random_by_multiple",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{"server", "dataset"},
						Rate: 0.1,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.random_by_with_whitelist",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionNext,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{"request_id"},
						Rate: 0.1,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.sample_from_whitelist",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionNext,
					OnMiss:  ActionFlagDisabled,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{"request_id"},
						Rate: 0.1,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.random_by_with_whitelist_and_blacklist",
			Active:   true,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionNext,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"alice", "bob"},
					},
				},
				{
					OnMatch: ActionFlagDisabled,
					OnMiss:  ActionNext,
					Condition: &InList{
						Tag:    "user",
						Values: []string{"carol"},
					},
				},
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{"request_id"},
						Rate: 0.2,
					},
				},
			},
		},
		&Flag{
			FlagName: "go.inactive",
			Active:   false,
			Conditions: []Info{
				{
					OnMatch: ActionFlagEnabled,
					OnMiss:  ActionFlagDisabled,
					Condition: &Sample{
						Tags: []string{},
						Rate: 0.1,
					},
				},
			},
		},
	}
	initConditionFlags(expected)
	assert.Equal(t, expected, flags)
}

func TestParseConditionJsonErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		ErrType error
		Json    string
	}{
		{
			ErrTypeUnknown{}, `{
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
			ErrActionUnknown{}, `{
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
			ErrVersion{}, `{
				"version": 99,
				"flags": []
			}`,
		},
	}

	for _, tc := range testCases {
		name := fmt.Sprintf("%T", tc.ErrType)
		t.Run(name, func(t *testing.T) {
			buf := bytes.NewBufferString(tc.Json)
			flags, lastMod, err := JsonFileFormat{}.Read(buf)
			require.Empty(t, flags)
			require.Zero(t, lastMod)
			require.Error(t, err)
			require.IsType(t, tc.ErrType, err)
		})
	}
}
