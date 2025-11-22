package rtp

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSequenceUnwrapper(t *testing.T) {
	tests := []struct {
		sequence []uint16
		expected []int64
	}{
		{
			sequence: []uint16{0, 1, 2, 3, 4},
			expected: []int64{0, 1, 2, 3, 4},
		},
		{
			sequence: []uint16{65534, 65535, 0, 1, 2},
			expected: []int64{65534, 65535, 65536, 65537, 65538},
		},
		{
			sequence: []uint16{32767, 0},
			expected: []int64{32767, 0},
		},
		{
			sequence: []uint16{32768, 0},
			expected: []int64{32768, 0},
		},
		{
			sequence: []uint16{32769, 0},
			expected: []int64{32769, 65536},
		},
		{
			sequence: []uint16{0, 1, 4, 3, 2, 5},
			expected: []int64{0, 1, 4, 3, 2, 5},
		},
		{
			sequence: []uint16{65534, 0, 1, 65535, 4, 3, 2, 5},
			expected: []int64{65534, 65536, 65537, 65535, 65540, 65539, 65538, 65541},
		},
		{
			sequence: []uint16{0, 32767, 32768, 32769, 32770, 1, 2, 32765, 32770, 65535},
			expected: []int64{0, 32767, 32768, 32769, 32770, 65537, 65538, 98301, 98306, 131071},
		},
		{
			sequence: []uint16{50, 55, 48, 21845, 43690, 65535, 0, 65532, 1, 21845},
			expected: []int64{50, 55, 48, 21845, 43690, 65535, 65536, 65532, 65537, 87381},
		},
		{
			sequence: []uint16{50, 55, 48, 21845, 0, 65532, 1, 21845},
			expected: []int64{50, 55, 48, 21845, 0, -4, 1, 21845},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.sequence), func(t *testing.T) {
			u := &SequenceUnwrapper{}
			var resCurrent []int64
			var resPrevious []int64
			for _, i := range test.sequence {
				previous, current := u.Unwrap(i)
				resPrevious = append(resPrevious, previous)
				resCurrent = append(resCurrent, current)
			}
			assert.Equal(t, test.expected, resCurrent)
			assert.Equal(t, append([]int64{resCurrent[0] - 1}, test.expected[:len(test.expected)-1]...), resPrevious)
		})
	}
}

func TestSequenceUnwrapper_Linear(t *testing.T) {
	su := SequenceUnwrapper{}
	for i := int64(0); i < 2*math.MaxUint16; i++ {
		last, current := su.Unwrap(uint16(i))
		require.Equal(t, i, current)
		require.Equal(t, i-1, last)
	}
}
