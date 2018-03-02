package refactor

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSampleFlag(t *testing.T) {
	t.Parallel()

	rnd := rand.New(rand.NewSource(0))

	on := SampleFlag{FlagName: "on", Rate: 1.0}
	assert.Equal(t, "on", on.Name())
	for i := 0; i < 1000; i++ {
		enabled, err := on.Enabled(rnd, nil)
		assert.NoError(t, err)
		assert.True(t, enabled)
	}

	off := SampleFlag{FlagName: "off", Rate: 0.0}
	assert.Equal(t, "off", off.Name())
	for i := 0; i < 1000; i++ {
		enabled, err := off.Enabled(rnd, nil)
		assert.NoError(t, err)
		assert.False(t, enabled)
	}

	half := SampleFlag{FlagName: "half", Rate: 0.5}
	assert.Equal(t, "half", half.Name())
	cnt := 0
	for i := 0; i < 1000; i++ {
		enabled, err := half.Enabled(rnd, nil)
		assert.NoError(t, err)
		if enabled {
			cnt += 1
		}
	}
	assert.InEpsilon(t, 500, cnt, 0.1)

	eighty := SampleFlag{FlagName: "eighty", Rate: 0.8}
	assert.Equal(t, "eighty", eighty.Name())
	cnt = 0
	for i := 0; i < 1000; i++ {
		enabled, err := eighty.Enabled(rnd, nil)
		assert.NoError(t, err)
		if enabled {
			cnt += 1
		}
	}
	assert.InEpsilon(t, 800, cnt, 0.1)
}
