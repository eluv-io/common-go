package structured

import (
	"encoding/json"
	"time"

	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/codecutil"
	"github.com/qluvio/content-fabric/util/numberutil"
)

// Wrap wraps the given data structure as a structured Value object, offering
// query, manipulation and conversion functions for the data.
//
// Err is an optional error value that occured as a result of retrieving or
// creating the data value. It allows to make error handling optional through
// the IsError() and Error() functions. All query and manipulation functions
// act on nil if an error is set, and conversion functions return the zero value
// or the optional default value specified in the conversion call.
func Wrap(data interface{}, err ...error) *Value {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	return NewValue(data, e)
}

// NewValue creates a new Value wrapper from the given value and error. Same
// as Wrap(val, err).
func NewValue(val interface{}, err error) *Value {
	return &Value{
		Data: val,
		err:  err,
	}
}

// WrapJson parses the given JSON document and returns the result as Value.
func WrapJson(jsonDoc string) *Value {
	val := &Value{}
	val.err = json.Unmarshal([]byte(jsonDoc), &val.Data)
	return val
}

// Value is a wrapper around structured data or the result of a structured data
// operation with convenience functions for querying and manipulating the
// structured data and accessing it as a specific data type.
//
// All typed conversion functions return the type's "zero value" if the
// structured value was created with an error or if the value is not of the
// requested type. Alternatively, a default value can be specified that is
// returned instead of the zero value.
//
//	val := structured.NewValue(structured.Resolve(path, data))
//	val.String()  // the string at path or "" if error or not a string
//	val.Int(99)   // the int at path or 99 if error or not an int
//	val.IsError() // true if the Resolve call returned an error
//
type Value struct {
	Data interface{}
	err  error
}

func (v *Value) Set(path Path, data interface{}) error {
	data, err := Set(v.Data, path, data)
	if err != nil {
		return err
	}
	v.Data = data
	return nil
}

func (v *Value) Merge(path Path, data interface{}) error {
	data, err := Merge(v.Data, path, data)
	if err != nil {
		return err
	}
	v.Data = data
	return nil
}

// Delete deletes the element at the given path and returns true if the element
// existed and was therefore deleted, false otherwise.
func (v *Value) Delete(path ...string) (deleted bool) {
	v.Data, deleted = Delete(v.Data, path)
	return deleted
}

// Get returns the value at the given path, specified as string slice, e.g.
// 	val.Get("path", "to", "value")
func (v *Value) Get(path ...string) *Value {
	return NewValue(Resolve(path, v.Data))
}

// GetP returns the value at the given path, specified as a single string, e.g.
// 	val.GetP("/path/to/value")
// Alias of At()
func (v *Value) GetP(path string) *Value {
	return v.At(path)
}

// At returns the value at the given path, specified as a single string, e.g.
// 	val.At("/path/to/value")
func (v *Value) At(path string) *Value {
	return NewValue(Resolve(ParsePath(path), v.Data))
}

func (v *Value) Query(query string) *Value {
	filter, err := NewFilter(query)
	if err != nil {
		return NewValue(nil, err)
	}
	return NewValue(filter.Apply(v.Data))
}

func (v *Value) Clear() error {
	return v.Set(nil, nil)
}

// Path is a convenience method to create a path from an arbitrary number of
// strings.
func (v *Value) Path(p ...string) Path {
	return Path(p)
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
		return v.Data
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

// Int64 returns the value as an int. If the value wraps an error, returns
// the optional default value def if specified, or 0.
func (v *Value) Int64(def ...int64) int64 {
	if v.err == nil && v.Data != nil {
		res, err := numberutil.AsInt64Err(v.Data)
		if err == nil {
			return res
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}

// Float64 returns the value as a float64. If the value wraps an error, returns
// the optional default value def if specified, or 0.
func (v *Value) Float64(def ...float64) float64 {
	if v.err == nil && v.Data != nil {
		res, err := numberutil.AsFloat64Err(v.Data)
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
	if v.err == nil && v.Data != nil {
		if t, ok := v.Data.(string); ok {
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
	if v.err == nil && v.Data != nil {
		if t, ok := v.Data.([]string); ok {
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
	if v.err == nil && v.Data != nil {
		if t, ok := v.Data.(map[string]interface{}); ok {
			return t
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return make(map[string]interface{})
}

// Slice returns the value as an []interface{}. If the value wraps an error,
// returns the optional default slice def if specified, or an empty slice.
func (v *Value) Slice(def ...interface{}) []interface{} {
	if v.err == nil && v.Data != nil {
		if t, ok := v.Data.([]interface{}); ok {
			return t
		}
	}
	if len(def) > 0 {
		return def
	}
	return make([]interface{}, 0)
}

// Bool returns the value as a string. If the value wraps an error, returns
// the optional default value def if specified, or false.
func (v *Value) Bool(def ...bool) bool {
	if v.err == nil && v.Data != nil {
		if t, ok := v.Data.(bool); ok {
			return t
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return false
}

// UTC returns the value as a UTC instance. If the value is a string, it
// attempts to parse it as a UTC time. If the value wraps an error, returns the
// optional default value def if specified, or utc.Zero.
func (v *Value) UTC(def ...utc.UTC) utc.UTC {
	if v.err == nil && v.Data != nil {
		switch t := v.Data.(type) {
		case utc.UTC:
			return t
		case time.Time:
			return utc.New(t)
		case string:
			res, err := utc.FromString(t)
			if err == nil {
				return res
			}
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return utc.Zero
}

// Decode decodes this Value into the given target object, which is assumed
// to be a pointer to a struct with optional `json`-annotated public members,
// and returns a potential unmarshaling error.
// If this Value wraps an error, the error is returned without attempting the
// decoding.
// See codecutil.MapDecode() for more information on the decoding process.
func (v *Value) Decode(target interface{}) error {
	if v.IsError() {
		return v.Error()
	}
	return codecutil.MapDecode(v.Data, target)
}
