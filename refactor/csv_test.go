package refactor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV(t *testing.T) {
	t.Parallel()

	path := filepath.Join("fixtures", "flags_example.csv")
	expected := []SampleFlag{
		{
			"go.sun.money",
			0,
		},
		{
			"go.moon.mercury",
			1,
		},
		{
			"go.stars.money",
			0.5,
		},
	}

	file, err := os.Open(path)
	require.NoError(t, err)
	flags, lastMod, err := csvFileFormat{}.Read(file)
	assert.NoError(t, err)
	assert.True(t, lastMod.IsZero())
	for i, f := range flags {
		sf, ok := f.(SampleFlag)
		require.True(t, ok)
		assert.Equal(t, expected[i].Name(), f.Name())
		assert.Equal(t, expected[i].Rate, sf.Rate)
	}
}
