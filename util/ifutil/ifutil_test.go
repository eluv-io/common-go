package ifutil

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

type chanType = chan bool
type mapType = map[string]bool
type sliceType = []string
type structType = struct{ a string }
type funcType = func(t *testing.T)

var (
	zeroChan      chanType
	zeroMap       mapType
	zeroSlice     sliceType
	zeroStruct    structType
	zeroStructPtr *structType
	zeroFunc      funcType

	emptyChan  = make(chan bool, 0)
	emptyMap   = map[string]bool{}
	emptySlice = make([]string, 0)
	emptyArray [0]string

	nonEmptyChan = func() chanType {
		c := make(chanType, 10)
		c <- true
		return c
	}()
	nonEmptyMap   = mapType{"a": true}
	nonEmptySlice = sliceType{"a", "b"}

	aStruct = structType{"a"}
	aFunc   = TestIsNil
)

func TestIsNil(t *testing.T) {

	asrt := assert.New(t)

	asrt.True(IsNil(nil))

	asrt.True(IsNil(zeroChan))
	asrt.True(zeroChan == nil)

	asrt.True(IsNil(zeroMap))
	asrt.True(zeroMap == nil)

	asrt.True(IsNil(zeroSlice))
	asrt.True(zeroSlice == nil)

	asrt.False(IsNil(zeroStruct))

	asrt.True(IsNil(zeroStructPtr))
	asrt.True(zeroStructPtr == nil)

	asrt.True(IsNil(zeroFunc))
	asrt.True(zeroFunc == nil)

	var iface interface{}
	asrt.True(IsNil(iface))
	asrt.True(iface == nil)

	iface = zeroChan
	asrt.True(IsNil(iface))
	asrt.False(iface == nil)

	iface = 5
	asrt.False(IsNil(iface))
	asrt.False(iface == nil)

	asrt.False(IsNil(make(chan bool)))
	asrt.False(IsNil(make(map[string]bool)))
	asrt.False(IsNil(make([]string, 0)))
}

func TestFirstNonNil(t *testing.T) {
	assert.Equal(t, nil, FirstNonNil[any]())
	assert.Equal(t, nil, FirstNonNil[any](nil))
	assert.Equal(t, nil, FirstNonNil[any](nil, nil))
	assert.Equal(t, aStruct, FirstNonEmpty[any](nil, aStruct))
	assert.Equal(t, &aStruct, FirstNonEmpty[any](nil, &aStruct))

	assert.Equal(t, zeroChan, FirstNonNil[chanType]())
	assert.Equal(t, zeroChan, FirstNonNil[chanType](nil))
	assert.Equal(t, zeroChan, FirstNonNil(zeroChan))
	assert.Equal(t, emptyChan, FirstNonNil(zeroChan, emptyChan, nonEmptyChan))

	assert.Equal(t, zeroMap, FirstNonNil[mapType]())
	assert.Equal(t, zeroMap, FirstNonNil[mapType](nil))
	assert.Equal(t, zeroMap, FirstNonNil(zeroMap))
	assert.Equal(t, nonEmptyMap, FirstNonNil(zeroMap, nonEmptyMap))

	assert.Equal(t, zeroSlice, FirstNonNil[sliceType]())
	assert.Equal(t, zeroSlice, FirstNonNil[sliceType](nil))
	assert.Equal(t, zeroSlice, FirstNonNil(zeroSlice))
	assert.Equal(t, nonEmptySlice, FirstNonNil(zeroSlice, nonEmptySlice))

	assert.Equal(t, zeroStruct, FirstNonNil[structType]())
	assert.Equal(t, zeroStruct, FirstNonNil(zeroStruct))
	assert.Equal(t, zeroStruct, FirstNonNil(zeroStruct, aStruct))

	assert.Equal(t, zeroStructPtr, FirstNonNil[*structType]())
	assert.Equal(t, zeroStructPtr, FirstNonNil[*structType](nil))
	assert.Equal(t, zeroStructPtr, FirstNonNil(zeroStructPtr))
	assert.Equal(t, &aStruct, FirstNonNil(zeroStructPtr, &aStruct))
	assert.Equal(t, &structType{"a"}, FirstNonNil(zeroStructPtr, &aStruct))

	assert.True(t, nil == FirstNonNil[func()]())
	assert.True(t, nil == FirstNonNil(zeroFunc))
	fnEqual(t, aFunc, FirstNonNil(zeroFunc, aFunc))
}

func TestIsEmpty(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(IsEmpty(emptyArray))
	asrt.True(IsEmpty(emptySlice))
	asrt.True(IsEmpty(emptyMap))
	asrt.True(IsEmpty(emptyChan))

	asrt.True(IsEmpty(nil))

	var iface interface{}
	asrt.True(IsEmpty(iface))

	asrt.True(IsEmpty(zeroSlice))
	asrt.True(IsEmpty(zeroMap))
	asrt.True(IsEmpty(zeroChan))
	asrt.True(IsEmpty(zeroStruct))
	asrt.True(IsEmpty(zeroStructPtr))
	asrt.True(IsEmpty(zeroFunc))

	asrt.True(IsEmpty(0))
	asrt.True(IsEmpty(""))
	asrt.True(IsEmpty(0.0))
	asrt.True(IsEmpty(false))
	asrt.True(IsEmpty(int8(0)))
	asrt.True(IsEmpty(int16(0)))

	asrt.False(IsEmpty(make([]string, 1)))
	asrt.False(IsEmpty(map[string]string{"a": "b"}))
	asrt.False(IsEmpty(make([]chan bool, 1)))
	asrt.False(IsEmpty(structType{"a"}))
	asrt.False(IsEmpty(&structType{"a"}))

	asrt.False(IsEmpty(1))
	asrt.False(IsEmpty("dfdsf"))
	asrt.False(IsEmpty(0.1))
	asrt.False(IsEmpty(true))
	asrt.False(IsEmpty(int8(3)))
	asrt.False(IsEmpty(int16(-1)))
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, nil, FirstNonEmpty[any]())
	assert.Equal(t, nil, FirstNonEmpty[any](nil))
	assert.Equal(t, nil, FirstNonEmpty[any](nil, nil))
	assert.Equal(t, aStruct, FirstNonEmpty[any](nil, aStruct))
	assert.Equal(t, &aStruct, FirstNonEmpty[any](nil, &aStruct))

	assert.Equal(t, zeroChan, FirstNonEmpty[chanType]())
	assert.Equal(t, zeroChan, FirstNonEmpty[chanType](nil))
	assert.Equal(t, zeroChan, FirstNonEmpty(zeroChan))
	assert.Equal(t, nonEmptyChan, FirstNonEmpty(zeroChan, emptyChan, nonEmptyChan))

	assert.Equal(t, zeroMap, FirstNonEmpty[mapType]())
	assert.Equal(t, zeroMap, FirstNonEmpty[mapType](nil))
	assert.Equal(t, zeroMap, FirstNonEmpty(zeroMap))
	assert.Equal(t, nonEmptyMap, FirstNonEmpty(zeroMap, emptyMap, nonEmptyMap))

	assert.Equal(t, zeroSlice, FirstNonEmpty[sliceType]())
	assert.Equal(t, zeroSlice, FirstNonEmpty[sliceType](nil))
	assert.Equal(t, zeroSlice, FirstNonEmpty(zeroSlice))
	assert.Equal(t, nonEmptySlice, FirstNonEmpty(zeroSlice, emptySlice, nonEmptySlice))

	assert.Equal(t, zeroStruct, FirstNonEmpty[structType]())
	assert.Equal(t, zeroStruct, FirstNonEmpty(zeroStruct))
	assert.Equal(t, aStruct, FirstNonEmpty(zeroStruct, aStruct))

	assert.Equal(t, zeroStructPtr, FirstNonEmpty[*structType]())
	assert.Equal(t, zeroStructPtr, FirstNonEmpty[*structType](nil))
	assert.Equal(t, zeroStructPtr, FirstNonEmpty(zeroStructPtr))
	assert.Equal(t, &aStruct, FirstNonEmpty(zeroStructPtr, &aStruct))
	assert.Equal(t, &structType{"a"}, FirstNonEmpty(zeroStructPtr, &aStruct))

	assert.True(t, nil == FirstNonEmpty[func()]())
	assert.True(t, nil == FirstNonEmpty(zeroFunc))
	fnEqual(t, aFunc, FirstNonEmpty(zeroFunc, aFunc))
}

func TestIsZero(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(IsZero(emptyArray))

	asrt.True(IsZero(zeroChan))
	asrt.True(IsZero(zeroStruct))
	asrt.True(IsZero(zeroStructPtr))
	// req.True(IsZero(zeroFunc)) ==> func is not comparable!

	asrt.True(IsZero(0))
	asrt.True(IsZero(""))
	asrt.True(IsZero(0.0))
	asrt.True(IsZero(false))
	asrt.True(IsZero(int8(0)))
	asrt.True(IsZero(int16(0)))

	asrt.False(IsZero(emptyChan))

	asrt.False(IsZero(structType{"a"}))
	asrt.False(IsZero(&structType{"a"}))

	asrt.False(IsZero(1))
	asrt.False(IsZero("dfdsf"))
	asrt.False(IsZero(0.1))
	asrt.False(IsZero(true))
	asrt.False(IsZero(int8(3)))
	asrt.False(IsZero(int16(-1)))
}

func TestFirstNonZero(t *testing.T) {
	assert.Equal(t, zeroChan, FirstNonZero[chanType]())
	assert.Equal(t, zeroChan, FirstNonZero[chanType](nil))
	assert.Equal(t, zeroChan, FirstNonZero(zeroChan))
	assert.Equal(t, emptyChan, FirstNonZero(zeroChan, emptyChan, nonEmptyChan))

	assert.Equal(t, zeroStruct, FirstNonZero[structType]())
	assert.Equal(t, zeroStruct, FirstNonZero(zeroStruct))
	assert.Equal(t, aStruct, FirstNonZero(zeroStruct, aStruct))

	assert.Equal(t, zeroStructPtr, FirstNonZero[*structType]())
	assert.Equal(t, zeroStructPtr, FirstNonZero[*structType](nil))
	assert.Equal(t, zeroStructPtr, FirstNonZero(zeroStructPtr))
	assert.Equal(t, &aStruct, FirstNonZero(zeroStructPtr, &aStruct))
	assert.Equal(t, &structType{"a"}, FirstNonZero(zeroStructPtr, &aStruct))

	assert.Equal(t, 0, FirstNonZero[int]())
	assert.Equal(t, 1, FirstNonZero(0, 1, 2))
	assert.Equal(t, 0.0, FirstNonZero[float64]())
	assert.Equal(t, 1.0, FirstNonZero(0.0, 1.0, 2.0))
	assert.Equal(t, "", FirstNonZero[string]())
	assert.Equal(t, "a", FirstNonZero("", "a", "b"))
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

// fnEqual compares two functions for equality. Comparing two functions directly is not allowed in go and assert.Equal
// returns a failure "cannot take func type as argument" when trying to do so.
//
// Hence the code here uses spew.Sdump to generate strings and compares those...
func fnEqual(t *testing.T, a, b interface{}) {
	assert.Equal(t, spew.Sdump(a), spew.Sdump(b))
}
