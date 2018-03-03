package goforit

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffBackend(t *testing.T) {
	backend := OffBackend{}
	flag, lastMod, err := backend.Flag("fake")
	assert.NoError(t, err)
	assert.True(t, lastMod.IsZero())

	rnd := rand.New(rand.NewSource(0))
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, enabled)
}
