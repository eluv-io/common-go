package utc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYearZeroOffset(t *testing.T) {
	require.Equal(t, -yearZeroOffsetSec, Min.Unix())
}
