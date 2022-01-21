package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// MarshalCompactString marshals the given value as compact JSON (no indenting,
// no newlines) and returns it as a string.
// The function panics if any errors occur.
func MarshalCompactString(v interface{}) string {
	return string(MarshalCompact(v))
}

// MarshalCompact marshals the given value as compact JSON (no indenting,
// no newlines) and returns it as a byte slice.
// The function panics if any errors occur.
func MarshalCompact(v interface{}) []byte {
	res, err := json.Marshal(v)
	if err != nil {
		err = errors.E("marshal json", errors.K.Invalid, err, "object_dump", spew.Sdump(v))
		panic("Failed to marshal json: " + err.Error())
	}
	return res
}

// MarshalString marshals the given value as indented JSON and returns it
// as a string.
// The function panics if any errors occur.
func MarshalString(v interface{}) string {
	return string(Marshal(v))
}

// Marshal marshals the given value as indented JSON and returns it as a
// byte slice.
// The function panics if any errors occur.
func Marshal(v interface{}) []byte {
	res, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		err = errors.E("marshal json", errors.K.Invalid, err, "object_dump", spew.Sdump(v))
		panic("Failed to marshal json: " + err.Error())
	}
	return res
}

// UnmarshalString unmarshals the given JSON string into v.
// The function panics if any errors occur.
func UnmarshalString(jsonText string, v interface{}) {
	Unmarshal([]byte(jsonText), v)
}

// Unmarshal unmarshals the given JSON byte slice into v.
// The function panics if any errors occur.
func Unmarshal(jsonText []byte, v interface{}) {
	err := json.Unmarshal(jsonText, v)
	if err != nil {
		err = errors.E("unmarshal json", errors.K.Invalid, err, "json", jsonText, "receiver", spew.Sdump(v))
		panic("Failed to unmarshal json: " + err.Error())
	}
}

// UnmarshalStringToMap unmarshals the given JSON string into a new map.
// The function panics if any errors occur.
func UnmarshalStringToMap(jsonText string) map[string]interface{} {
	return UnmarshalToMap([]byte(jsonText))
}

// UnmarshalToMap unmarshals the given JSON byte slice into a new map.
// The function panics if any errors occur.
func UnmarshalToMap(jsonText []byte) map[string]interface{} {
	var m map[string]interface{}
	if len(jsonText) == 0 {
		return m
	}
	err := json.Unmarshal(jsonText, &m)
	if err != nil {
		err = errors.E("unmarshal json", errors.K.Invalid, err, "json", string(jsonText), "receiver", spew.Sdump(m))
		panic("Failed to unmarshal json: " + err.Error())
	}
	return m
}

// UnmarshalStringToAny unmarshals the given JSON string into an empty interface.
// The function panics if any errors occur.
func UnmarshalStringToAny(jsonText string) interface{} {
	return UnmarshalToAny([]byte(jsonText))
}

// UnmarshalToAny unmarshals the given JSON byte slice into an empty interface.
// The function panics if any errors occur.
func UnmarshalToAny(jsonText []byte) interface{} {
	var any interface{}
	if len(jsonText) == 0 {
		return any
	}
	err := json.Unmarshal(jsonText, &any)
	if err != nil {
		err = errors.E("unmarshal json", errors.K.Invalid, err, "json", string(jsonText), "receiver", spew.Sdump(any))
		panic("Failed to unmarshal json: " + err.Error())
	}
	return any
}

// Clone clones the given data structure by marshaling it to JSON and
// unmarshaling it back from JSON.
func Clone(v interface{}) (interface{}, error) {
	var clone interface{}
	jsonText, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(jsonText, &clone)
	}
	return clone, err
}

// MustClone clones the given data structure by marshaling it to JSON and
// unmarshaling it back from JSON.
func MustClone(v interface{}) interface{} {
	var clone interface{}
	Unmarshal(Marshal(v), &clone)
	return clone
}

// IsJson checks whether the given byte slice is a valid JSON document. If
// partial is true, the byte slice may contain only a partial JSON document.
func IsJson(buf []byte, partial bool) bool {
	if len(buf) == 0 {
		// empty is considered valid JSON
		return true
	}
	var js json.RawMessage
	err := json.Unmarshal(buf, &js)
	if err == nil {
		return true
	}
	if !partial {
		return false
	}
	if jse, ok := err.(*json.SyntaxError); ok {
		if jse.Offset == int64(len(buf)) {
			return true
		}
	}
	return false
}

// Pretty parses the given json string and re-marshals it in "pretty" format
// with line breaks and 2-space indentation.
func Pretty(js string) (string, error) {
	var out bytes.Buffer
	err := json.Indent(&out, []byte(js), "", "  ")
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// MustPretty parses the given json string and re-marshals it in "pretty" format
// with line breaks and indentation. Panics if the json cannot be parsed.
func MustPretty(js string) string {
	out, err := Pretty(js)
	if err != nil {
		panic(errors.E("failed to pretty print", err, "json", js))
	}
	return out
}

// Stringer returns a wrapper around val whose String() function will simply
// return val's JSON representation. If val is a 'func() interface{}', it will
// call that function and marshal its return value.
//
// Stringer also implements MarshalJSON and simply delegates to the wrapped
// value or the result of calling the function.
func Stringer(val interface{}) fmt.Stringer {
	return &stringer{val}
}

// stringer is a small decorator that returns the nested value's JSON
// representation in the String() function.
type stringer struct {
	val interface{}
}

func (s *stringer) String() string {
	val := s.val
	if fn, ok := val.(func() interface{}); ok {
		val = fn()
	}
	bts, err := json.Marshal(val)
	if err != nil {
		return fmt.Sprintf("%#v", val)
	}
	return string(bts)
}

func (s *stringer) MarshalJSON() ([]byte, error) {
	val := s.val
	if fn, ok := val.(func() interface{}); ok {
		val = fn()
	}
	return json.Marshal(val)
}

// MarshallingError converts the given marshalling error to a string in JSON
// format limited to ~100 characters. This is useful for handling errors in a
// struct's 'String() string' method that uses JSON marshalling to create the
// string description.
func MarshallingError(msg string, err error) string {
	log.Warn("Failed to marshall "+msg, "error", err)
	ex := err.Error()
	if len(ex) > 80 {
		ex = ex[:80] + ".."
	}
	return fmt.Sprintf(`{"marshalling_error": "%s"}`, ex)
}

// GenericMarshaler is an interface for types that implement a marshalling
// method which converts the type to a generic go data structure consisting of
// map[string]interface{}, []interface{} and primitive types.
type GenericMarshaler interface {
	MarshalGeneric() interface{}
}
