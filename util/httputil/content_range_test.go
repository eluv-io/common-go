package httputil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func assertRange(t *testing.T, eOff, eEndOff, eLen int64, ePartial bool, actual *ContentRange, caseid string) {
	if assert.NotNil(t, actual) {
		assert.Equal(t, eOff, actual.GetAdaptedOff(), "%s %s", "adaptedOffset", caseid)
		assert.Equal(t, eEndOff, actual.GetAdaptedEndOff(), "%s %s", "adaptedEndOffset", caseid)
		assert.Equal(t, eLen, actual.GetAdaptedLen(), "%s %s", "adaptedLen", caseid)
		assert.Equal(t, ePartial, actual.IsPartial(), "%s %s", "partial", caseid)
	}
}

func TestAdaptRange(t *testing.T) {
	r, err := AdaptRange(0, -1, 0)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 0, false, r, "a")

	r, err = AdaptRange(0, 0, 0)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 0, false, r, "0")

	r, err = AdaptRange(0, 1, 0)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 0, false, r, "1")

	r, err = AdaptRange(0, 1, 1)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 1, false, r, "2")

	r, err = AdaptRange(0, 1, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 1, true, r, "3")

	r, err = AdaptRange(-1, 1, 10)
	assert.Nil(t, err)
	assertRange(t, 9, 9, 1, true, r, "4")

	r, err = AdaptRange(-1, 10, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 9, 10, false, r, "5")

	r, err = AdaptRange(0, 100, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 9, 10, false, r, "6")

	r, err = AdaptRange(0, -1, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 9, 10, false, r, "7")

	r, err = AdaptRange(1, -1, 10)
	assert.Nil(t, err)
	assertRange(t, 1, 9, 9, true, r, "8")

	r, err = AdaptRange(1, 100, 10)
	assert.Nil(t, err)
	assertRange(t, 1, 9, 9, true, r, "9")

	r, err = AdaptRange(100, -1, 100)
	assert.NoError(t, err)
	assertRange(t, 100, 100, 0, true, r, "10")

	r, err = AdaptRange(100, 10, 100)
	assert.NoError(t, err)
	assertRange(t, 100, 100, 0, true, r, "11")

	r, err = AdaptRange(0, -1, -1)
	assert.NoError(t, err)
	assertRange(t, 0, -1, -1, false, r, "12")

	r, err = AdaptRange(10, -1, -1)
	assert.NoError(t, err)
	assertRange(t, 10, -1, -1, true, r, "13")

	r, err = AdaptRange(10, 100, -1)
	assert.NoError(t, err)
	assertRange(t, 10, 109, 100, true, r, "14")

	// error cases
	r, err = AdaptRange(-1, -1, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "negative offset and length")

	r, err = AdaptRange(-1, 0, -1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "negative offset and total_length")

	r, err = AdaptRange(-1, 2, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "length larger than total_length")

	r, err = AdaptRange(2, -1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "offset larger than total_length")

	r, err = AdaptRange(2, 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "offset larger than total_length")
}
