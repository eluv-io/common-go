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

// Wrap wraps the given data structure as a structured Value object, offering query, manipulation and conversion
// functions for the data.
//
// Err is an optional error value that occured as a result of retrieving or creating the data value. It allows to
// make error handling optional through the IsError() and Error() functions. All query and manipulation functions act
// on nil if an error is set, and conversion functions return the zero value or the optional default value specified
// in the conversion call.
func Wrap(data interface{}, err ...error) *Value {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	return NewValue(data, e)
}

// Unwrap unwraps any directly nested Value objects and returns the raw data.
func Unwrap(v interface{}) interface{} {
	for {
		if val, ok := v.(*Value); ok {
			v = val.Data
		} else {
			return v
		}
	}
}

// NewValue creates a new Value wrapper from the given value and error. Same as Wrap(val, err).
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

// Value is a wrapper around structured data or the result of a structured data operation with convenience functions
// for querying and manipulating the structured data and accessing it as a specific data type.
//
// All typed conversion functions return the type's "zero value" if the structured value was created with an error or
// if the value is not of the requested type. Alternatively, a default value can be specified that is returned instead
// of the zero value.
//
//	val := structured.NewValue(structured.Resolve(path, data))
//	val.String()  // the string at path or "" if error or not a string
//	val.Int(99)   // the int at path or 99 if error or not an int
//	val.IsError() // true if the Resolve call returned an error
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

// Delete deletes the element at the given path and returns true if the element existed and was therefore deleted,
// false otherwise.
func (v *Value) Delete(path ...string) (deleted bool) {
	v.Data, deleted = Delete(v.Data, path)
	return deleted
}

// Get returns the value at the given path, specified as string slice, e.g.
//
//	val.Get("path", "to", "value")
func (v *Value) Get(path ...string) *Value {
	return NewValue(Resolve(path, v.Data))
}

// GetP returns the value at the given path, specified as a single string, e.g.
//
//	val.GetP("/path/to/value")
//
// Alias of At()
func (v *Value) GetP(path string) *Value {
	return v.At(path)
}

// At returns the value at the given path, specified as a single string, e.g.
//
//	val.At("/path/to/value")
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

// Path is a convenience method to create a path from an arbitrary number of strings.
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

// ValueErr returns the value as empty interface, along with any stored error. If the value wraps an error, the error
// is returned together with the optional default def if specified, or nil.
func (v *Value) ValueErr(def ...interface{}) (interface{}, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return nil, v.err
	}
	return v.Data, nil
}

// Value returns the value as empty interface. If the value wraps an error,
// returns the optional default value def if specified, or nil.
func (v *Value) Value(def ...interface{}) interface{} {
	res, _ := v.ValueErr(def...)
	return res
}

// IntErr returns the value as an int along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil) and no default is provided, errors.K.NotExist is returned.
func (v *Value) IntErr(def ...int) (int, error) {
	var i64Def []int64
	if len(def) > 0 {
		i64Def = []int64{int64(def[0])}
	}
	res, err := v.Int64Err(i64Def...)
	return int(res), err
}

// Int returns the value as an int. If the value wraps an error or is absent, returns
// the optional default value def if specified, or 0.
func (v *Value) Int(def ...int) int {
	res, _ := v.IntErr(def...)
	return res
}

// Int64Err returns the value as an int64 along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist is
// returned.
func (v *Value) Int64Err(def ...int64) (int64, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return 0, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return 0, errors.NoTrace("Int64", errors.K.NotExist)
	}
	res, err := numberutil.AsInt64Err(v.Data)
	if err != nil {
		if len(def) > 0 {
			return def[0], err
		}
		return 0, err
	}
	return res, nil
}

// Int64 returns the value as an int64. If the value wraps an error or is absent, returns
// the optional default value def if specified, or 0.
func (v *Value) Int64(def ...int64) int64 {
	res, _ := v.Int64Err(def...)
	return res
}

// UIntErr returns the value as a uint along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil) and no default is provided, errors.K.NotExist is returned.
func (v *Value) UIntErr(def ...uint) (uint, error) {
	var u64Def []uint64
	if len(def) > 0 {
		u64Def = []uint64{uint64(def[0])}
	}
	res, err := v.UInt64Err(u64Def...)
	return uint(res), err
}

// UInt returns the value as an uint. If the value wraps an error or is absent, returns
// the optional default value def if specified, or 0.
func (v *Value) UInt(def ...uint) uint {
	res, _ := v.UIntErr(def...)
	return res
}

// UInt64Err returns the value as a uint64 along with any conversion error. If the value wraps an error, the error
// is returned. If the value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist
// is returned.
func (v *Value) UInt64Err(def ...uint64) (uint64, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return 0, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return 0, errors.NoTrace("UInt64", errors.K.NotExist)
	}
	res, err := numberutil.AsUInt64Err(v.Data)
	if err != nil {
		if len(def) > 0 {
			return def[0], err
		}
		return 0, err
	}
	return res, nil
}

// UInt64 returns the value as an uint64. If the value wraps an error or is absent, returns
// the optional default value def if specified, or 0.
func (v *Value) UInt64(def ...uint64) uint64 {
	res, _ := v.UInt64Err(def...)
	return res
}

// Float64Err returns the value as a float64 along with any conversion error. If the value wraps an error, the error
// is returned. If the value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist
// is returned.
func (v *Value) Float64Err(def ...float64) (float64, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return 0, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return 0, errors.NoTrace("Float64", errors.K.NotExist)
	}
	res, err := numberutil.AsFloat64Err(v.Data)
	if err != nil {
		if len(def) > 0 {
			return def[0], err
		}
		return 0, err
	}
	return res, nil
}

// Float64 returns the value as a float64. If the value wraps an error or is absent, returns
// the optional default value def if specified, or 0.
func (v *Value) Float64(def ...float64) float64 {
	res, _ := v.Float64Err(def...)
	return res
}

// StringErr returns the value as a string along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist is
// returned. If the value exists but is not a string, a conversion error is returned.
func (v *Value) StringErr(def ...string) (string, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return "", v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return "", errors.NoTrace("String", errors.K.NotExist)
	}
	t, ok := v.Data.(string)
	if !ok {
		if len(def) > 0 {
			return def[0], errors.NoTrace("String", errors.K.Invalid, "value", v.Data)
		}
		return "", errors.NoTrace("String", errors.K.Invalid, "value", v.Data)
	}
	return t, nil
}

// String returns the value as a string. If the value wraps an error or is absent, returns
// the optional default value def if specified, or "".
func (v *Value) String(def ...string) string {
	res, _ := v.StringErr(def...)
	return res
}

// ToStringErr converts the value to a string and returns it along with any error. If the value wraps an error, the
// error is returned. If the value is absent (nil), the default def is returned if provided, otherwise
// errors.K.NotExist is returned. The conversion itself always succeeds for non-nil values.
func (v *Value) ToStringErr(def ...string) (string, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return "", v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return "", errors.NoTrace("ToString", errors.K.NotExist)
	}
	return stringutil.ToString(v.Data), nil
}

// ToString converts the value to a string. Returns optional default or "" if
// the value wraps an error or is absent.
func (v *Value) ToString(def ...string) string {
	res, _ := v.ToStringErr(def...)
	return res
}

// StringSliceErr returns the value as a string slice along with any conversion error. If the value wraps an error,
// the error is returned with def as the result. If the value is absent (nil) and a default is provided, def is
// returned with no error. If absent with no default, errors.K.NotExist is returned. If the value is not a []string,
// a conversion error is returned.
func (v *Value) StringSliceErr(def ...string) ([]string, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def, v.err
		}
		return make([]string, 0), v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def, nil
		}
		return make([]string, 0), errors.NoTrace("StringSlice", errors.K.NotExist)
	}
	if t, ok := v.Data.([]string); ok {
		return t, nil
	}
	if len(def) > 0 {
		return def, errors.NoTrace("StringSlice", errors.K.Invalid, "value", v.Data)
	}
	return make([]string, 0), errors.NoTrace("StringSlice", errors.K.Invalid, "value", v.Data)
}

// StringSlice returns the value as a string slice. If the value wraps an error or is absent, returns the optional
// default slice def if specified, or an empty slice.
func (v *Value) StringSlice(def ...string) []string {
	res, _ := v.StringSliceErr(def...)
	return res
}

// MapErr returns the value as a map along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil) and a default is provided, def is returned with no error. If absent with no
// default, errors.K.NotExist is returned. If the value is not a map[string]interface{}, a conversion error is
// returned.
func (v *Value) MapErr(def ...map[string]interface{}) (map[string]interface{}, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return make(map[string]interface{}), v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return make(map[string]interface{}), errors.NoTrace("Map", errors.K.NotExist)
	}
	if t, ok := v.Data.(map[string]interface{}); ok {
		return t, nil
	}
	if len(def) > 0 {
		return def[0], errors.NoTrace("Map", errors.K.Invalid, "value", v.Data)
	}
	return make(map[string]interface{}), errors.NoTrace("Map", errors.K.Invalid, "value", v.Data)
}

// Map returns the value as a map. If the value wraps an error or is absent, returns
// the optional default value def if specified, or an empty map.
func (v *Value) Map(def ...map[string]interface{}) map[string]interface{} {
	res, _ := v.MapErr(def...)
	return res
}

// SliceErr returns the value as a []interface{} along with any conversion error. If the value wraps an error, the
// error is returned with def as the result. If the value is absent (nil) and a default is provided, def is returned
// with no error. If absent with no default, errors.K.NotExist is returned. If the value is not a []interface{}, a
// conversion error is returned.
func (v *Value) SliceErr(def ...interface{}) ([]interface{}, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def, v.err
		}
		return make([]interface{}, 0), v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def, nil
		}
		return make([]interface{}, 0), errors.NoTrace("Slice", errors.K.NotExist)
	}
	if t, ok := v.Data.([]interface{}); ok {
		return t, nil
	}
	if len(def) > 0 {
		return def, errors.NoTrace("Slice", errors.K.Invalid, "value", v.Data)
	}
	return make([]interface{}, 0), errors.NoTrace("Slice", errors.K.Invalid, "value", v.Data)
}

// Slice returns the value as an []interface{}. If the value wraps an error or is absent,
// returns the optional default slice def if specified, or an empty slice.
func (v *Value) Slice(def ...interface{}) []interface{} {
	res, _ := v.SliceErr(def...)
	return res
}

// BoolErr returns the value as a bool along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist is
// returned. If the value is not a bool, a conversion error is returned.
func (v *Value) BoolErr(def ...bool) (bool, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return false, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return false, errors.NoTrace("Bool", errors.K.NotExist)
	}
	if t, ok := v.Data.(bool); ok {
		return t, nil
	}
	if len(def) > 0 {
		return def[0], errors.NoTrace("Bool", errors.K.Invalid, "value", v.Data)
	}
	return false, errors.NoTrace("Bool", errors.K.Invalid, "value", v.Data)
}

// Bool returns the value as a bool. If the value wraps an error or is absent, returns
// the optional default value def if specified, or false.
func (v *Value) Bool(def ...bool) bool {
	res, _ := v.BoolErr(def...)
	return res
}

// ToBoolErr converts the value to a bool and returns it along with any error. A bool value is returned as-is.
// String values "true"/"false" (case-insensitive) are parsed. All other values result in a conversion error. If the
// value wraps an error, the error is returned. If the value is absent (nil), the default def is returned if
// provided, otherwise errors.K.NotExist is returned.
func (v *Value) ToBoolErr(def ...bool) (bool, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return false, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return false, errors.NoTrace("ToBool", errors.K.NotExist)
	}
	if t, ok := v.Data.(bool); ok {
		return t, nil
	}
	switch strings.ToLower(stringutil.ToString(v.Data)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if len(def) > 0 {
		return def[0], errors.NoTrace("ToBool", errors.K.Invalid, "value", v.Data)
	}
	return false, errors.NoTrace("ToBool", errors.K.Invalid, "value", v.Data)
}

// ToBool converts the value to a boolean. If the value wraps an error or is absent, returns
// the optional default value def if specified, or false.
func (v *Value) ToBool(def ...bool) bool {
	res, _ := v.ToBoolErr(def...)
	return res
}

// UTCErr returns the value as a utc.UTC along with any conversion error. If the value wraps an error, the error is
// returned. If the value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist is
// returned. String values are parsed as UTC timestamps.
func (v *Value) UTCErr(def ...utc.UTC) (utc.UTC, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return utc.Zero, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return utc.Zero, errors.NoTrace("UTC", errors.K.NotExist)
	}
	switch t := v.Unwrap().(type) {
	case utc.UTC:
		return t, nil
	case time.Time:
		return utc.New(t), nil
	case string:
		res, err := utc.FromString(t)
		if err == nil {
			return res, nil
		}
	}
	if len(def) > 0 {
		return def[0], errors.NoTrace("UTC", errors.K.Invalid, "value", v.Data)
	}
	return utc.Zero, errors.NoTrace("UTC", errors.K.Invalid, "value", v.Data)
}

// UTC returns the value as a UTC instance. If the value is a string, it
// attempts to parse it as a UTC time. If the value wraps an error or is absent, returns the
// optional default value def if specified, or utc.Zero.
func (v *Value) UTC(def ...utc.UTC) utc.UTC {
	res, _ := v.UTCErr(def...)
	return res
}

// Decode decodes this Value into the given target object, which is assumed to be a pointer to a struct with optional
// `json`-annotated public members, and returns a potential unmarshaling error. If this Value wraps an error, the
// error is returned without attempting the decoding. See codecutil.MapDecode() for more information on the decoding
// process.
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
		err := r.AssertCode(code)
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

// DurationErr converts this value to a duration spec and returns it along with any error. If the value is numeric,
// it is interpreted as a multiple of the provided unit. If the value wraps an error, the error is returned. If the
// value is absent (nil), the default def is returned if provided, otherwise errors.K.NotExist is returned.
func (v *Value) DurationErr(unit duration.Spec, def ...duration.Spec) (duration.Spec, error) {
	if v.err != nil {
		if len(def) > 0 {
			return def[0], v.err
		}
		return 0, v.err
	}
	if v.Data == nil {
		if len(def) > 0 {
			return def[0], nil
		}
		return 0, errors.NoTrace("Duration", errors.K.NotExist)
	}
	data := v.Unwrap()
	switch t := data.(type) {
	case duration.Spec:
		return t, nil
	case time.Duration:
		return duration.Spec(t), nil
	case string:
		d, err := duration.FromString(t) // also parses time.Duration correctly
		if err == nil {
			return d, nil
		}
		// otherwise try to parse as numeric value below
	}
	f, err := numberutil.AsFloat64Err(data)
	if err == nil {
		return duration.Spec(f * float64(unit)), nil
	}
	if len(def) > 0 {
		return def[0], err
	}
	return 0, err
}

// Duration converts this value to a duration spec. If the value is numeric, it is interpreted as a multiple of the
// provided unit. Returns the default value or 0 if the value is absent or the conversion fails.
func (v *Value) Duration(unit duration.Spec, def ...duration.Spec) duration.Spec {
	res, _ := v.DurationErr(unit, def...)
	return res
}
