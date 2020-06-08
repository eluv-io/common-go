package ifutil

import (
	"fmt"
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

func ExampleDiff() {
	fmt.Println(Diff("string A", "string a", "string B", "string b"))
	fmt.Println(Diff("int A", 1, "int B", 2))
	fmt.Println(Diff("float A", 1.1, "float B", 2.1))
	fmt.Println(Diff("same", "", "same", ""))
	fmt.Println(Diff("same", nil, "same", nil))

	// Output:
	//
	// --- string A
	// +++ string B
	// @@ -1,2 +1,2 @@
	// -(string) (len=8) "string a"
	// +(string) (len=8) "string b"
	//
	// --- int A
	// +++ int B
	// @@ -1,2 +1,2 @@
	// -(int) 1
	// +(int) 2
	//
	// --- float A
	// +++ float B
	// @@ -1,2 +1,2 @@
	// -(float64) 1.1
	// +(float64) 2.1
}
