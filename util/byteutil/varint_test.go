package byteutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLenUvarInt(t *testing.T) {
	tests := []struct {
		x    uint64
		want int
	}{
		{0, 1},
		{1, 1},
		{127, 1},
		{128, 2},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.x), func(t *testing.T) {
			require.Equal(t, tt.want, LenUvarInt(tt.x))
		})
	}
}
