package structured

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/codecutil"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/numberutil"
	"github.com/eluv-io/common-go/util/stringutil"
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

// Unwraps any directly nested Value objects and returns the raw data.
func Unwrap(v interface{}) interface{} {
	for {
		if val, ok := v.(*Value); ok {
			v = val.Data
		} else {
			return v
		}
	}
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

func (v *Value) UnmarshalMap(m map[string]interface{}) error {
	v.Data = m
	return nil
}

func (v *Value) UnmarshalJSON(bytes []byte) error {
	return json.Unmarshal(bytes, &v.Data)
}

func (v *Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Data)
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
	return v == nil || v.err != nil
}

// Error returns the error if this Value wraps an error, nil otherwise.
func (v *Value) Error() error {
	if v == nil {
		return errors.E("", errors.K.Invalid, "reason", "nil value")
	}
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

// UInt returns the value as an uint. If the value wraps an error, returns
// the optional default value def if specified, or 0.
func (v *Value) UInt(def ...uint) uint {
	if len(def) > 0 {
		return uint(v.UInt64(uint64(def[0])))
	}
	return uint(v.UInt64())
}

// UInt64 returns the value as an uint. If the value wraps an error, returns
// the optional default value def if specified, or 0.
func (v *Value) UInt64(def ...uint64) uint64 {
	if v.err == nil && v.Data != nil {
		res, err := numberutil.AsUInt64Err(v.Data)
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

// ToString converts the value to a string. Returns optional default or "" if
// the value wraps an error or is nil.
func (v *Value) ToString(def ...string) string {
	if v.err == nil && v.Data != nil {
		return stringutil.ToString(v.Data)
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

// ToBool converts the value to a boolean. If the value wraps an error, returns
// the optional default value def if specified, or false.
func (v *Value) ToBool(def ...bool) bool {
	if v.err == nil && v.Data != nil {
		if t, ok := v.Data.(bool); ok {
			return t
		}
	}
	switch strings.ToLower(v.ToString()) {
	case "true":
		return true
	case "false":
		return false
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
		switch t := v.Unwrap().(type) {
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

// Unwrap returns the value wrapped by this Value object, recursively. Similar
// to Value.Data, but unwraps nested Value objects.
//
//	Wrap("a string").Data           // "a string"
//	Wrap("a string").Unwrap()       // "a string"
//	Wrap(Wrap("a string")).Data     // *Value
//	Wrap(Wrap("a string")).Unwrap() // "a string"
func (v *Value) Unwrap() interface{} {
	return Unwrap(v)
}

// ID returns this value as an id.ID with the given code and optional default
// value.
func (v *Value) ID(code id.Code, def ...id.ID) (id.ID, error) {
	raw := v.Unwrap()
	if ifutil.IsNil(raw) && len(def) > 0 {
		return def[0], nil
	}
	switch r := raw.(type) {
	case id.ID:
		err := r.AssertCompatible(code)
		if err != nil {
			return nil, err
		}
		return r, nil
	case string:
		ret, err := code.FromString(r)
		if err != nil {
			return nil, err
		}
		return ret, nil
	}
	ret, err := code.FromString(fmt.Sprintf("%v", raw))
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Duration converts this value to a duration spec. If the value is numeric, it is interpreted as a multiple of the
// provided unit. Returns the default value or 0 if the conversion fails.
func (v *Value) Duration(unit duration.Spec, def ...duration.Spec) duration.Spec {
	if v.err == nil && v.Data != nil {
		data := v.Unwrap()
		switch t := data.(type) {
		case duration.Spec:
			return t
		case time.Duration:
			return duration.Spec(t)
		case string:
			d, err := duration.FromString(t) // also parses time.Duration correctly
			if err == nil {
				return d
			}
			// otherwise try to parse as numeric value below
		}
		f, err := numberutil.AsFloat64Err(data)
		if err == nil {
			return duration.Spec(f * float64(unit))
		}
	}
	if len(def) > 0 {
		return def[0]
	}
	return 0
}
