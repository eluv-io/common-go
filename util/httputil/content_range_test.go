package httputil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func assertRange(t *testing.T, eOff, eEndOff, eLen int64, ePartial bool, actual *ContentRange, caseid string, header string) {
	if assert.NotNil(t, actual) {
		assert.Equal(t, eOff, actual.GetAdaptedOff(), "%s %s", "adaptedOffset", caseid)
		assert.Equal(t, eEndOff, actual.GetAdaptedEndOff(), "%s %s", "adaptedEndOffset", caseid)
		assert.Equal(t, eLen, actual.GetAdaptedLen(), "%s %s", "adaptedLen", caseid)
		assert.Equal(t, ePartial, actual.IsPartial(), "%s %s", "partial", caseid)
		assert.Equal(t, header, actual.AsHeader(), "%s %s", "header", caseid)
	}
}

func assertRangeError(t *testing.T, actual *ContentRange, err error, mustContain, header string) {
	caseid := mustContain
	assert.Error(t, err, caseid)
	assert.Contains(t, err.Error(), mustContain, caseid)
	assert.Equal(t, header, actual.AsHeader(), caseid)
	assert.Less(t, actual.GetAdaptedOff(), int64(0), caseid)
	assert.Less(t, actual.GetAdaptedEndOff(), int64(0), caseid)
	assert.Less(t, actual.GetAdaptedLen(), int64(0), caseid)
}

func TestAdaptRange(t *testing.T) {
	r, err := AdaptRange(0, -1, 0)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 0, false, r, "a", "bytes 0-0/0")

	r, err = AdaptRange(0, 0, 0)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 0, false, r, "0", "bytes 0-0/0")

	r, err = AdaptRange(0, 1, 0)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 0, false, r, "1", "bytes 0-0/0")

	r, err = AdaptRange(0, 1, 1)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 1, false, r, "2", "bytes 0-0/1")

	r, err = AdaptRange(0, 1, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 0, 1, true, r, "3", "bytes 0-0/10")

	r, err = AdaptRange(-1, 1, 10)
	assert.Nil(t, err)
	assertRange(t, 9, 9, 1, true, r, "4", "bytes 9-9/10")

	r, err = AdaptRange(-1, 10, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 9, 10, false, r, "5", "bytes 0-9/10")

	r, err = AdaptRange(0, 100, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 9, 10, false, r, "6", "bytes 0-9/10")

	r, err = AdaptRange(0, -1, 10)
	assert.Nil(t, err)
	assertRange(t, 0, 9, 10, false, r, "7", "bytes 0-9/10")

	r, err = AdaptRange(1, -1, 10)
	assert.Nil(t, err)
	assertRange(t, 1, 9, 9, true, r, "8", "bytes 1-9/10")

	r, err = AdaptRange(1, 100, 10)
	assert.Nil(t, err)
	assertRange(t, 1, 9, 9, true, r, "9", "bytes 1-9/10")

	r, err = AdaptRange(100, -1, 100)
	assert.NoError(t, err)
	assertRange(t, 100, 100, 0, true, r, "10", "bytes 100-100/100")

	r, err = AdaptRange(100, 10, 100)
	assert.NoError(t, err)
	assertRange(t, 100, 100, 0, true, r, "11", "bytes 100-100/100")

	r, err = AdaptRange(0, -1, -1)
	assert.NoError(t, err)
	assertRange(t, 0, -1, -1, false, r, "12", "bytes 0-")

	r, err = AdaptRange(10, -1, -1)
	assert.NoError(t, err)
	assertRange(t, 10, -1, -1, true, r, "13", "bytes 10-")

	r, err = AdaptRange(10, 100, -1)
	assert.NoError(t, err)
	assertRange(t, 10, 109, 100, true, r, "14", "bytes 10-109")

	// error cases
	r, err = AdaptRange(-1, -1, 10)
	assertRangeError(t, r, err, "negative offset and length", "bytes */10")

	r, err = AdaptRange(-1, 0, -1)
	assertRangeError(t, r, err, "negative offset and total_length", "bytes *")

	r, err = AdaptRange(-1, 2, 1)
	assertRangeError(t, r, err, "length larger than total_length", "bytes */1")

	r, err = AdaptRange(2, -1, 1)
	assertRangeError(t, r, err, "offset larger than total_length", "bytes */1")

	r, err = AdaptRange(2, 1, 1)
	assertRangeError(t, r, err, "offset larger than total_length", "bytes */1")
}
