package jsonutil

import (
	"encoding/json"
	"github.com/qluvio/content-fabric/log"

	"github.com/qluvio/content-fabric/errors"

	"github.com/davecgh/go-spew/spew"
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

	log.Debug("=================================================================")
	log.Debug("")
	log.Debug(string(jsonText))
	log.Debug("")
	log.Debug("=================================================================")

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
