package ifutil

import (
	"reflect"
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
