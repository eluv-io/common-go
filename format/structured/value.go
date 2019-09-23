package structured

import "github.com/qluvio/content-fabric/util/numberutil"

// NewValue creates a new Value wrapper from the given value and error.
func NewValue(val interface{}, err error) *Value {
	return &Value{
		val: val,
		err: err,
	}
}

// Value is a wrapper around the result of a structured data operation with
// convenience functions for accessing the result as a specific data type.
// Its typed accessor functions return the type's "zero value" if the operation
// resulted in an error, or if the result was not of the requested type.
// Alternatively, a default value can be specified that is returned instead of
// the zero value in case of result error or type mismatch.
//
//	val := structured.NewValue(structured.Resolve(path, data))
//	val.String()  // the string at path or "" if error or not a string
//	val.Int(99)   // the int at path or 99 if error or not an int
//	val.IsError() // true if the Resolve call returned an error
//
type Value struct {
	val interface{}
	err error
}

// IsError returns true if this Value wraps an error, false otherwise.
func (v *Value) IsError() bool {
	return v.err != nil
}

// Error returns the error if this Value wraps an error, nil otherwise.
func (v *Value) Error() error {
	return v.err
}

// Value returns the value as empty interface. If the value wraps an error,
// returns the optional default value def if specified, or nil.
func (v *Value) Value(def ...interface{}) interface{} {
	if v.err == nil {
		return v.val
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

// Int returns the value as an int. If the value wraps an error, returns
// the optional default value def if specified, or 0.
func (v *Value) Int(def ...int) int {
	if len(def) > 0 {
		return int(v.Int64(int64(def[0])))
	}
	return int(v.Int64())
}

// Int returns the value as an int. If the value wraps an error, returns
// the optional default value def if specified, or 0.
func (v *Value) Int64(def ...int64) int64 {
	if v.err == nil && v.val != nil {
		res, err := numberutil.AsInt64Err(v.val)
		if err == nil {
			return res
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

// String returns the value as a string. If the value wraps an error, returns
// the optional default value def if specified, or "".
func (v *Value) String(def ...string) string {
	if v.err == nil && v.val != nil {
		if t, ok := v.val.(string); ok {
			return t
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return ""
}

// StringSlice returns the value as a string slice. If the value wraps an
// error, returns the optional default slice def if specified, or an empty slice.
func (v *Value) StringSlice(def ...string) []string {
	if v.err == nil && v.val != nil {
		if t, ok := v.val.([]string); ok {
			return t
		}
	}
	if len(def) > 0 {
		return def
	}
	return make([]string, 0)
}

// Map returns the value as a map. If the value wraps an error, returns
// the optional default value def if specified, or an empty map.
func (v *Value) Map(def ...map[string]interface{}) map[string]interface{} {
	if v.err == nil && v.val != nil {
		if t, ok := v.val.(map[string]interface{}); ok {
			return t
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return make(map[string]interface{})
}

// Wraps this value in another SD.
func (v *Value) Wrap() *SD {
	return Wrap(v.val)
}
