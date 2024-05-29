package codecutil

import (
	"encoding"
	"encoding/base64"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

type MapUnmarshaler interface {
	UnmarshalMap(m map[string]interface{}) error
}

var (
	mapUnmarshaler  = reflect.TypeOf((*MapUnmarshaler)(nil)).Elem()
	textUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	byteSliceType   = reflect.TypeOf([]byte(nil))
)

// MapDecode decodes a parsed, generic source structure that was e.g.
// produced by unmarshaling JSON
//
//	var any interface{}
//	_ := json.Unmarshal(jsonText, &any)
//
// into the destination object dst (usually a pointer to a struct value). Any
// `json:...` tags defined on the destination structure's member fields will be
// used for unmarshaling (just like when unmarshaling JSON text).
//
// The implementation uses github.com/mitchellh/mapstructure to do the decoding,
// with the following special decoding hooks:
//   - decodes with the 'UnmarshalMap(m map[string]interface{}) error'
//     function if implemented by the destination object/field
//   - decodes with the 'UnmarshalText(text []byte) error' function if the
//     destination implements encoding.TextUnmarshaler
func MapDecode(src interface{}, dst interface{}) error {
	cfg := &mapstructure.DecoderConfig{
		TagName:    "json",
		Result:     dst,
		DecodeHook: decodeHook,
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	if err != nil {
		return err
	}
	return decoder.Decode(src)
}

func decodeHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	switch dt := data.(type) {
	case map[string]interface{}:
		t, ptr := resolve(t)
		if ptr.Implements(mapUnmarshaler) {
			instance := reflect.New(t)
			fnc := instance.MethodByName("UnmarshalMap")
			if !fnc.IsValid() {
				return data, nil
			}
			err := fnc.Call([]reflect.Value{reflect.ValueOf(dt)})
			if len(err) > 0 && !err[0].IsNil() {
				return nil, err[0].Interface().(error)
			}
			return instance.Interface(), nil
		}
	case string:
		t, ptr := resolve(t)
		if ptr.Implements(textUnmarshaler) {
			instance := reflect.New(t)
			fnc := instance.MethodByName("UnmarshalText")
			if !fnc.IsValid() {
				return data, nil
			}
			err := fnc.Call([]reflect.Value{reflect.ValueOf([]byte(dt))})
			if len(err) > 0 && !err[0].IsNil() {
				return nil, err[0].Interface().(error)
			}
			return instance.Interface(), nil
		} else if t == byteSliceType {
			// byte arrays are marshaled to base64 encoded string in JSON by default...
			return base64.StdEncoding.DecodeString(dt)
		}
	}

	return data, nil
}

func resolve(t reflect.Type) (reflect.Type, reflect.Type) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	ptr := reflect.PtrTo(t)
	return t, ptr
}
