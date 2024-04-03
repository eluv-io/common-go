package httputil

import (
	"net/url"
	"testing"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/stretchr/testify/assert"
)

func TestStringQuery(t *testing.T) {
	query := url.Values{}
	query.Add("key", "value")
	query.Add("key2", "value2")

	assert.Equal(t, "value", StringQuery(query, "key", "default"))
	assert.Equal(t, "default", StringQuery(query, "key3", "default"))
}

func TestBoolQuery(t *testing.T) {
	query := url.Values{}
	query.Add("key", "true")
	query.Add("key2", "false")

	assert.True(t, BoolQuery(query, "key", false))
	assert.False(t, BoolQuery(query, "key2", true))
	assert.True(t, BoolQuery(query, "key3", true))
}

func TestArrayQueryWithSplit(t *testing.T) {
	query := url.Values{}
	query.Add("key", "value1,value2")
	assert.Equal(t, []string{"value1", "value2"}, ArrayQueryWithSplit(query, "key", ","))
}

func TestIntQuery(t *testing.T) {
	query := url.Values{}
	query.Add("key", "123")
	query.Add("key2", "abc")

	assert.Equal(t, 123, IntQuery(query, "key", 0))
	assert.Equal(t, 0, IntQuery(query, "key2", 0))
	assert.Equal(t, 0, IntQuery(query, "key3", 0))
}

func TestStringToBool(t *testing.T) {
	assert.True(t, StringToBool("true", false))
	assert.True(t, StringToBool("t", false))
	assert.True(t, StringToBool("yes", false))
	assert.True(t, StringToBool("y", false))
	assert.True(t, StringToBool("1", false))
	assert.True(t, StringToBool("", false))

	assert.False(t, StringToBool("false", false))
	assert.False(t, StringToBool("f", false))
	assert.False(t, StringToBool("no", false))
}

func TestFloatQuery(t *testing.T) {
	query := url.Values{}
	query.Add("key", "123.45")
	query.Add("key2", "abc")

	assert.Equal(t, 123.45, Float64Query(query, "key", 0))
	assert.Equal(t, 0.0, Float64Query(query, "key2", 0))
	assert.Equal(t, 0.0, Float64Query(query, "key3", 0))
}

func TestDurationQuery(t *testing.T) {
	query := url.Values{}
	query.Add("key", "1h")
	query.Add("key2", "abc")

	assert.Equal(t, duration.Spec(3600000000000), DurationQuery(query, "key", duration.Parse("1h", "0"), duration.Parse("0", "0")))
	assert.Equal(t, duration.Spec(0), DurationQuery(query, "key2", duration.Parse("1h", "0"), duration.Parse("0", "0")))
	assert.Equal(t, duration.Spec(0), DurationQuery(query, "key3", duration.Parse("1h", "0"), duration.Parse("0", "0")))
}

func TestUintPtrQuery(t *testing.T) {
	query := url.Values{}
	query.Add("key", "123")
	query.Add("key2", "abc")
	assert.Equal(t, uint(123), *UintPtrQuery(query, "key"))
}
