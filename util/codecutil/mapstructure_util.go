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

var mapUnmarshaler = reflect.TypeOf((*MapUnmarshaler)(nil)).Elem()
var textUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

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
// When the variadic squash parameter is used, its first value is set to the
// Squash field of the decoder configuration.
func MapDecode(src interface{}, dst interface{}, squash ...bool) error {
	sqsh := false
	if len(squash) > 0 {
		sqsh = squash[0]
	}
	cfg := &mapstructure.DecoderConfig{
		TagName:    "json",
		Result:     dst,
		Squash:     sqsh,
		DecodeHook: decodeHook,
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	if err != nil {
		return err
	}
	return decoder.Decode(src)
}

var byteSliceType = reflect.TypeOf([]byte(nil))

func decodeHook(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
	switch dt := data.(type) {
	case map[string]interface{}:
		t, ptr := resolve(t)
		if ptr.Implements(mapUnmarshaler) {
			instance := reflect.New(t)

			ret := instance.Interface()
			err := ret.(MapUnmarshaler).UnmarshalMap(dt)
			if err != nil {
				return nil, err
			}
			return ret, nil

		}
	case string:
		t, ptr := resolve(t)
		if ptr.Implements(textUnmarshaler) {
			instance := reflect.New(t)

			ret := instance.Interface()
			err := ret.(encoding.TextUnmarshaler).UnmarshalText([]byte(dt))
			if err != nil {
				return nil, err
			}
			return ret, nil

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
	ptr := reflect.PointerTo(t)
	return t, ptr
}
