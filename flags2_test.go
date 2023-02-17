package goforit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stripe/goforit/clamp"
	"github.com/stripe/goforit/flags"
	"github.com/stripe/goforit/flags2"
)

type FlagTestCase2 struct {
	Flag     string
	Expected bool
	Attrs    map[string]*string
	Message  string
}
type FlagAcceptance2 struct {
	flags2.JSONFormat2
	TestCases []FlagTestCase2 `json:"test_cases"`
}

func TestFlags2Backend(t *testing.T) {
	t.Parallel()

	expectedFlags := []flags.Flag{
		flags2.Flag2{Name: "off_flag", Seed: "seed_1", Rules: []flags2.Rule2{}},
		flags2.Flag2{
			Name:  "go.moon.mercury",
			Seed:  "seed_1",
			Rules: []flags2.Rule2{{HashBy: "_random", Percent: 1.0, Predicates: []flags2.Predicate2{}}},
		},
		flags2.Flag2{
			Name:  "go.stars.money",
			Seed:  "seed_1",
			Rules: []flags2.Rule2{{HashBy: "_random", Percent: 0.5, Predicates: []flags2.Predicate2{}}},
		},
		flags2.Flag2{
			Name:  "go.sun.money",
			Seed:  "seed_1",
			Rules: []flags2.Rule2{{HashBy: "_random", Percent: 0.0, Predicates: []flags2.Predicate2{}}},
		},
		flags2.Flag2{
			Name: "flag5",
			Seed: "seed_1",
			Rules: []flags2.Rule2{
				{
					HashBy:  "token",
					Percent: 1.0,
					Predicates: []flags2.Predicate2{
						{Attribute: "token", Operation: flags2.OpIn, Values: map[string]bool{"id_1": true, "id_2": true}},
						{Attribute: "country", Operation: flags2.OpNotIn, Values: map[string]bool{"KP": true}},
					},
				},
				{HashBy: "token", Percent: 0.5, Predicates: []flags2.Predicate2{}},
			},
		},
	}

	backend := BackendFromJSONFile2(filepath.Join("testdata", "flags2_example.json"))
	flags, updated, err := backend.Refresh()

	assert.NoError(t, err)
	assert.Equal(t, expectedFlags, flags)
	assert.Equal(t, int64(1584642857), updated.Unix())
}

func flags2AcceptanceTests(t *testing.T, f func(t *testing.T, flagname string, flag flags2.Flag2, properties map[string]string, expected bool, msg string)) {
	path := filepath.Join("testdata", "flags2_acceptance.json")
	buf, err := ioutil.ReadFile(path)
	require.NoError(t, err)

	var acceptanceData FlagAcceptance2
	err = json.Unmarshal(buf, &acceptanceData)
	require.NoError(t, err)

	flags := map[string]flags2.Flag2{}
	for _, f := range acceptanceData.Flags {
		flags[f.Name] = f
	}

	for _, tc := range acceptanceData.TestCases {
		name := fmt.Sprintf("%s:%s", tc.Flag, tc.Message)
		dup := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// We don't distinguish between missing/nil values
			properties := map[string]string{}
			for k, v := range dup.Attrs {
				if v != nil {
					properties[k] = *v
				}
			}

			msg := fmt.Sprintf("%s %v", dup.Flag, dup.Attrs)
			f(t, dup.Flag, flags[dup.Flag], properties, dup.Expected, msg)
		})
	}
}

func TestFlags2Acceptance(t *testing.T) {
	t.Parallel()

	flags2AcceptanceTests(t, func(t *testing.T, flagname string, flag flags2.Flag2, properties map[string]string, expected bool, msg string) {
		enabled, err := flag.Enabled(nil, properties, nil)
		assert.NoError(t, err)
		assert.Equal(t, expected, enabled, msg)
	})
}

func TestFlags2AcceptanceDefaultTags(t *testing.T) {
	t.Parallel()

	flags2AcceptanceTests(t, func(t *testing.T, flagname string, flag flags2.Flag2, properties map[string]string, expected bool, msg string) {
		enabled, err := flag.Enabled(nil, nil, properties)
		assert.NoError(t, err)
		assert.Equal(t, expected, enabled, msg)
	})
}

func TestFlags2AcceptanceClamp(t *testing.T) {
	t.Parallel()

	flags2AcceptanceTests(t, func(t *testing.T, flagname string, flag flags2.Flag2, properties map[string]string, expected bool, msg string) {
		switch flag.Clamp() {
		case clamp.AlwaysOn:
			assert.True(t, expected, msg)
		case clamp.AlwaysOff:
			assert.False(t, expected, msg)
		}
	})
}

func TestFlags2AcceptanceEndToEnd(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "flags2_acceptance.json")
	backend := BackendFromJSONFile2(path)
	g, _ := testGoforit(10*time.Millisecond, backend, stalenessCheckInterval)
	defer g.Close()

	flags2AcceptanceTests(t, func(t *testing.T, flagname string, flag flags2.Flag2, properties map[string]string, expected bool, msg string) {
		enabled := g.Enabled(context.Background(), flagname, properties)
		assert.Equal(t, expected, enabled, msg)
	})
}

func TestFlags2Reserialize(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "flags2_acceptance.json")
	backend := BackendFromJSONFile2(path)

	flags, _, err := backend.Refresh()
	require.NoError(t, err)

	flagsOut := make([]flags2.Flag2, len(flags))
	for i := 0; i < len(flags); i++ {
		flagsOut[i] = flags[i].(flags2.Flag2)
	}

	root := flags2.JSONFormat2{
		Flags: flagsOut,
	}

	marshaled, err := json.MarshalIndent(&root, "", "  ")
	require.NoError(t, err)

	file, err := os.CreateTemp("", "goforit-flags2-reserializetest")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	_, err = file.Write(marshaled)
	require.NoError(t, err)

	backend2 := BackendFromJSONFile2(file.Name())
	flags2, _, err := backend2.Refresh()
	require.NoError(t, err)

	require.Equal(t, flags, flags2)
}
