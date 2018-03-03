package goforit

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that the OffBackend is always off
func TestOffBackend(t *testing.T) {
	// Fetch a flag
	backend := OffBackend{}
	flag, lastMod, err := backend.Flag("fake")
	assert.NoError(t, err)
	assert.True(t, lastMod.IsZero())

	// The flag should report that it's off
	rnd := rand.New(rand.NewSource(0))
	enabled, err := flag.Enabled(rnd, nil)
	assert.NoError(t, err)
	assert.False(t, enabled)
}
