package ifutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type dummy struct {
	a string
}

var (
	zeroChan      chan bool
	zeroMap       map[string]bool
	zeroSlice     []string
	zeroStruct    dummy
	zeroStructPtr *dummy

	emptyChan  = make(chan bool, 0)
	emptyMap   = map[string]bool{}
	emptySlice = make([]string, 0)
	emptyArray [0]string
)

func TestIsNil(t *testing.T) {

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

func TestIsEmpty(t *testing.T) {
	req := require.New(t)

	req.True(IsEmpty(emptyArray))
	req.True(IsEmpty(emptySlice))
	req.True(IsEmpty(emptyMap))
	req.True(IsEmpty(emptyChan))

	req.True(IsEmpty(nil))

	var iface interface{}
	req.True(IsEmpty(iface))

	req.True(IsEmpty(zeroSlice))
	req.True(IsEmpty(zeroMap))
	req.True(IsEmpty(zeroChan))
	req.True(IsEmpty(zeroStruct))
	req.True(IsEmpty(zeroStructPtr))

	req.True(IsEmpty(0))
	req.True(IsEmpty(""))
	req.True(IsEmpty(0.0))
	req.True(IsEmpty(false))
	req.True(IsEmpty(int8(0)))
	req.True(IsEmpty(int16(0)))

	req.False(IsEmpty(make([]string, 1)))
	req.False(IsEmpty(map[string]string{"a": "b"}))
	req.False(IsEmpty(make([]chan bool, 1)))
	req.False(IsEmpty(dummy{"a"}))
	req.False(IsEmpty(&dummy{"a"}))

	req.False(IsEmpty(1))
	req.False(IsEmpty("dfdsf"))
	req.False(IsEmpty(0.1))
	req.False(IsEmpty(true))
	req.False(IsEmpty(int8(3)))
	req.False(IsEmpty(int16(-1)))
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
