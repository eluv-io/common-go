package numberutil_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/eluv-io/common-go/util/numberutil"

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

func TestLessInt(t *testing.T) {
	tests := []struct {
		ascending bool
		i1        int
		i2        int
		tie       func() bool
		wantLess  bool
	}{
		{true, 0, 1, nil, true},
		{true, 0, 0, nil, true},
		{true, 1, 0, nil, false},
		{false, 0, 1, nil, false},
		{false, 0, 0, nil, false},
		{false, 1, 0, nil, true},
		{true, 0, 0, func() bool { return true }, true},
		{true, 0, 0, func() bool { return false }, false},
		{false, 0, 0, func() bool { return true }, true},
		{false, 0, 0, func() bool { return false }, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("ascending=%t,i1=%d,i2=%d,tie=%t", tt.ascending, tt.i1, tt.i2, tt.tie != nil), func(t *testing.T) {
			require.Equal(t, tt.wantLess, numberutil.LessInt(tt.ascending, tt.i1, tt.i2, tt.tie))
		})
	}
}

func assertAsFloat64(t *testing.T, expected float64, v interface{}) {
	actual := numberutil.AsFloat64(v)
	require.Equal(t, expected, actual)
}

func TestAsFloat64(t *testing.T) {
	assertAsFloat64(t, 0, "mlp")
	assertAsFloat64(t, 8, 8)
	assertAsFloat64(t, 123, int64(123))
	assertAsFloat64(t, 456, "456")
	assertAsFloat64(t, 456.789, "456.789")
	assertAsFloat64(t, 12.3, 12.3)
	assertAsFloat64(t, 12.5, 12.5)
	assertAsFloat64(t, 12.0, float32(12.0))
	assertAsFloat64(t, 12.5, float32(12.5))
}

func TestMin(t *testing.T) {
	require.Equal(t, 0, numberutil.Min(0, 0))
	require.Equal(t, 1, numberutil.Min(1, 2))
	require.Equal(t, uint(1), numberutil.Min(uint(1), uint(2)))
	require.Equal(t, 1.5, numberutil.Min(1.5, 2.4))
	require.True(t, math.IsNaN(numberutil.Min(math.NaN(), 2.4)))
	require.True(t, math.IsNaN(numberutil.Min(2.4, math.NaN())))
}

func TestMax(t *testing.T) {
	require.Equal(t, 0, numberutil.Max(0, 0))
	require.Equal(t, 2, numberutil.Max(1, 2))
	require.Equal(t, uint(2), numberutil.Max(uint(1), uint(2)))
	require.Equal(t, 2.4, numberutil.Max(1.5, 2.4))
	require.True(t, math.IsNaN(numberutil.Max(math.NaN(), 2.4)))
	require.True(t, math.IsNaN(numberutil.Max(2.4, math.NaN())))
}
