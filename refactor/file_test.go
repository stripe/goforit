package refactor

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileBackendInitial(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	path := filepath.Join("fixtures", "flags_example.csv")
	backend := NewFileBackend(path, CsvFileFormat{}, DefaultRefreshInterval)
	defer backend.Close()

	flag, lastMod, err := backend.Flag("go.sun.money")
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	assert.Equal(t, "go.sun.money", flag.Name())
	sf, ok := flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 0.0, sf.Rate)
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, enabled)

	flag, lastMod, err = backend.Flag("go.moon.mercury")
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	assert.Equal(t, "go.moon.mercury", flag.Name())
	sf, ok = flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 1.0, sf.Rate)
	enabled, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.True(t, enabled)

	flag, lastMod, err = backend.Flag("go.stars.money")
	assert.NoError(t, err)
	assert.False(t, lastMod.IsZero())
	assert.Equal(t, "go.stars.money", flag.Name())
	sf, ok = flag.(SampleFlag)
	require.True(t, ok)
	assert.Equal(t, 0.5, sf.Rate)
	enabled, err = flag.Enabled(rnd, nil)
	assert.NoError(t, err)
}

func TestFileBackendMultipleDefinitions(t *testing.T) {

}

func TestFileBackendRefresh(t *testing.T) {

}

func TestFileBackendFileRefresh(t *testing.T) {

}

func TestFileBackendClose(t *testing.T) {

}

func TestFileBackendMissing(t *testing.T) {
}

func TestFileBackendParseError(t *testing.T) {
}

func TestFileBackendAge(t *testing.T) {
}
