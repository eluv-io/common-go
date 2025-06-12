package ifutil

import (
	"reflect"

	"github.com/davecgh/go-spew/spew"
	"github.com/pmezard/go-difflib/difflib"
)

var nillableKinds = []reflect.Kind{
	reflect.Chan, reflect.Func,
	reflect.Interface, reflect.Map,
	reflect.Ptr, reflect.Slice}

// IsNil returns true if the given object is nil (== nil) or is a nillable type
// (channel, function, interface, map, pointer or slice) with a nil value.
func IsNil(obj interface{}) bool {
	if obj == nil {
		return true
	}

	value := reflect.ValueOf(obj)
	kind := value.Kind()
	for _, k := range nillableKinds {
		if k == kind {
			return value.IsNil()
		}
	}

	return false
}

// FirstNonNil returns the first argument that is not nil as determined by the IsNil function. Returns the zero value
// for the argument type if all arguments are nil.
func FirstNonNil[T any](objs ...T) T {
	for _, obj := range objs {
		if !IsNil(obj) {
			return obj
		}
	}
	var zero T
	return zero
}

// IsEmpty returns true if the given object is considered "empty":
//   - nil
//   - collections with no element (arrays, slices, maps)
//   - unbuffered channels or buffered channels without any buffered elements
//   - nil pointer or pointer to an empty object
//   - the zero value for all other types
func IsEmpty(obj interface{}) bool {
	if obj == nil {
		return true
	}

	val := reflect.ValueOf(obj)

	switch val.Kind() {
	// collection types are empty when they have no element
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
		return val.Len() == 0
	// pointers are empty if nil or if the value they point to is empty
	case reflect.Ptr:
		if val.IsNil() {
			return true
		}
		// dereference and check again
		return IsEmpty(val.Elem().Interface())
	// for all other types, compare against the zero value
	default:
		return IsZero(obj)
	}
}

// FirstNonEmpty returns the first argument that is not empty as determined by the IsEmpty function. Returns the zero
// value for the argument type if all arguments are empty.
func FirstNonEmpty[T any](objs ...T) T {
	for _, obj := range objs {
		if !IsEmpty(obj) {
			return obj
		}
	}
	var zero T
	return zero
}

// IsZero returns true if the given argument is the zero value of its type, false otherwise.
func IsZero(v interface{}) bool {
	if v == nil {
		return true
	}
	return reflect.ValueOf(v).IsZero()
}

// FirstNonZero returns the first argument that is not the zero value as determined by the IsZero function. Returns the
// zero value for the argument type if all arguments are empty.
func FirstNonZero[T comparable](ts ...T) T {
	for _, t := range ts {
		if !IsZero(t) {
			return t
		}
	}
	var zero T
	return zero
}

// FirstOrDefault returns the first non-zero element from the given slice or the provided default value otherwise.
// Useful for initializing optional function parameters with a default value:
//
//	func Foo(optInclude ...bool) {
//		include = FirstOrDefault(optInclude, false)
//		...
//	}
func FirstOrDefault[T any](opts []T, defaultValue T) T {
	for _, t := range opts {
		if !IsZero(t) {
			return t
		}
	}
	return defaultValue
}

var spewConfig = spew.ConfigState{
	Indent:                  " ",
	DisablePointerAddresses: true,
	DisableCapacities:       true,
	SortKeys:                true,
}

// Diff returns the difference of two objects in "unified diff" format. It first
// converts each object to text using spew.Sdump, then calculates and returns
// the diff.
func Diff(labelA string, a interface{}, labelB string, b interface{}) string {
	old := spewConfig.Sdump(a)
	cur := spewConfig.Sdump(b)
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(old),
		B:        difflib.SplitLines(cur),
		FromFile: labelA,
		FromDate: "",
		ToFile:   labelB,
		ToDate:   "",
		Context:  1,
	})
	if len(diff) < 2 {
		return diff
	}
	return diff[:len(diff)-2]
}

// Must takes an object and an error, and panics if the error is not nil. It can be used for
// inlining function calls when you know that the function won't error.
func Must[T any](value T, err error) T {
	if !IsNil(err) {
		panic(err)
	}
	return value
}

// IgnoreErr takes an object and an error, and ignores the error. It can be used for inlining
// function calls.
func IgnoreErr[T any](value T, _ error) T {
	return value
}
