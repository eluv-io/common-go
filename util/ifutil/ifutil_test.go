package ifutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type dummy struct{}

func TestIsNil(t *testing.T) {
	var zeroChan chan bool
	var zeroMap map[string]bool
	var zeroSlice []string
	var zeroStruct dummy
	var zeroStructPtr *dummy

	req := require.New(t)

	req.True(IsNil(nil))

	req.True(IsNil(zeroChan))
	req.True(zeroChan == nil)

	req.True(IsNil(zeroMap))
	req.True(zeroMap == nil)

	req.True(IsNil(zeroSlice))
	req.True(zeroSlice == nil)

	req.False(IsNil(zeroStruct))

	req.True(IsNil(zeroStructPtr))
	req.True(zeroStructPtr == nil)

	var iface interface{}
	req.True(IsNil(iface))
	req.True(iface == nil)

	iface = zeroChan
	req.True(IsNil(iface))
	req.False(iface == nil)

	iface = 5
	req.False(IsNil(iface))
	req.False(iface == nil)

	req.False(IsNil(make(chan bool)))
	req.False(IsNil(make(map[string]bool)))
	req.False(IsNil(make([]string, 0)))
}
