package structured

import (
	"reflect"
)

// Clone deep-copies maps and slices (recursively) and returns everything else (structs, pointers, scalars, strings) by
// reference / value, unchanged.
func Clone[T any](v T) T {
	return castClone[T](cloneAny(v, reflect.Value{}))
}

// CloneMap is like Clone, but achieves better performance for maps by avoiding the overhead of reflect.ValueOf() and
// type assertions.
func CloneMap[Map ~map[K]V, K comparable, V any](m Map) Map {
	if m == nil {
		return nil
	}
	cp := make(Map, len(m))
	switch any(*new(V)).(type) {
	case byte, string, int, int8, int16, int32, int64, uint, uint16, uint32, uint64, float32, float64, bool, uintptr, complex64, complex128:
		for k, v := range m {
			cp[k] = v
		}
	default:
		for k, v := range m {
			cp[k] = castClone[V](cloneAny(v, reflect.Value{}))
		}
	}
	return cp
}

// CloneSlice is like Clone, but achieves better performance for slices by avoiding the overhead of reflect.ValueOf()
// and type assertions.
func CloneSlice[S ~[]E, E any](source S) S {
	if source == nil {
		return nil
	}

	dup := make(S, len(source))
	switch any(*new(E)).(type) {
	case byte, string, int, int8, int16, int32, int64, uint, uint16, uint32, uint64, float32, float64, bool, uintptr, complex64, complex128:
		// E is a basic type, hence we can just copy the slice.
		// This also works for named byte slice types, e.g. `type Bytes []byte`
		copy(dup, source)
		return dup
	}
	// if _, ok := any(*new(E)).(byte); ok {
	// 	// E is a byte, hence S is a byte slice, so we can just copy the slice. This also works for named byte slice
	// 	// types, e.g. `type Bytes []byte`
	// 	copy(dup, source)
	// 	return dup
	// }

	for i, val := range source {
		dup[i] = castClone[E](cloneAny(val, reflect.Value{}))
	}
	return dup
}

// castClone converts the result of cloneAny back to the target type T. A direct type assertion x.(T) panics when x is
// a nil interface and T is itself an interface type (e.g. type Meta any). Checking for nil first and returning the zero
// value of T avoids that: for interface T the zero value is nil, which is the correct result.
func castClone[T any](v any) T {
	if v == nil {
		var zero T
		return zero
	}
	return v.(T)
}

// cloneAny tries the fast type switch first. rv is an optional, already-known reflect.Value for v, letting cloneElem
// skip a redundant reflect.ValueOf in the default branch. When invalid (reflect.Value{}), it's rebuilt on demand.
//
// INVARIANT: if rv.IsValid(), it must be reflect.ValueOf(v) — the *concrete* value, never an interface wrapper.
// cloneElem guarantees this via .Elem().
func cloneAny(v any, rv reflect.Value) any {
	switch m := v.(type) {
	case nil:
		return nil
	case map[string]any:
		if m == nil {
			return m
		}
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = cloneAny(val, reflect.Value{})
		}
		return out
	case []any:
		if m == nil {
			return m
		}
		out := make([]any, len(m))
		for i, val := range m {
			out[i] = cloneAny(val, reflect.Value{})
		}
		return out
	case map[string]string:
		if m == nil {
			return m
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	case []byte:
		if m == nil {
			return m
		}
		out := make([]byte, len(m))
		copy(out, m)
		return out
	case []string:
		if m == nil {
			return m
		}
		out := make([]string, len(m))
		copy(out, m)
		return out
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64,
		complex64, complex128:
		return v
	default:
		if !rv.IsValid() {
			// rv is only valid if it was created from reflect.ValueOf(xyz) in cloneReflect. Otherwise, we pass in a
			// zero-value reflect.Value, in which case we need to create one from v.
			rv = reflect.ValueOf(v)
		}
		return cloneReflect(rv).Interface()
	}
}

func cloneReflect(rv reflect.Value) reflect.Value {
	switch rv.Kind() {
	case reflect.Map:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneElem(iter.Value()))
		}
		return out
	case reflect.Slice:
		if rv.IsNil() {
			return rv
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		switch rv.Type().Elem().Kind() {
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:

			// the slice element type is a basic type, hence we can just copy the slice.
			reflect.Copy(out, rv)
			return out
		}
		// if rv.Type().Elem().Kind() == reflect.Uint8 {
		// 	// same byte slice trick as in CopySlice...
		// 	reflect.Copy(out, rv)
		// 	return out
		// }
		for i := 0; i < rv.Len(); i++ {
			out.Index(i).Set(cloneElem(rv.Index(i)))
		}
		return out
	default:
		return rv // structs, pointers, scalars: by reference / value
	}
}

// cloneElem routes elements back through cloneAny so common element types (e.g. []string, map[string]any) hit the fast
// path even when their enclosing container missed it. The matching concrete reflect.Value is passed along to avoid a
// redundant reflect.ValueOf() call in cloneAny's default branch.
func cloneElem(rv reflect.Value) reflect.Value {
	if rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return rv
		}
		rv = rv.Elem() // unwrap to the concrete Value, not the interface wrapper
	}
	switch rv.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return rv
	}
	return reflect.ValueOf(cloneAny(rv.Interface(), rv))
}
