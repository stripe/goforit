package goforit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/davecgh/go-spew/spew"
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
	assert.NotNil(t, flags)
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	spew.Dump(flags)

	// TODO: check return
}

// Condition types
// Overall flag algorithm
// Marshaling/unmarshaling
// Errors unmarshaling
