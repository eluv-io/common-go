package numberutil_test

import (
	"testing"

	"eluvio/util/numberutil"

	"github.com/stretchr/testify/require"
)

func assertAsInt(t *testing.T, expected int, v interface{}) {
	actual := numberutil.AsInt(v)
	require.Equal(t, expected, actual)
}

func assertAsInt64(t *testing.T, expected int64, v interface{}) {
	actual := numberutil.AsInt64(v)
	require.Equal(t, expected, actual)
}

func TestAsInt(t *testing.T) {

	assertAsInt(t, 0, "mlp")
	assertAsInt(t, 8, 8)
	assertAsInt(t, 123, int64(123))
	assertAsInt(t, 456, "456")
	assertAsInt(t, 12, 12.3)
	assertAsInt(t, 13, 12.5)
	assertAsInt(t, 12, float32(12.3))
	assertAsInt(t, 13, float32(12.5))
}

func TestAsInt64(t *testing.T) {

	assertAsInt64(t, 0, "mlp")
	assertAsInt64(t, 8, 8)
	assertAsInt64(t, 123, int64(123))
	assertAsInt64(t, 456, "456")
	assertAsInt64(t, 12, 12.3)
	assertAsInt64(t, 13, 12.5)
	assertAsInt64(t, 12, float32(12.3))
	assertAsInt64(t, 13, float32(12.5))
}
