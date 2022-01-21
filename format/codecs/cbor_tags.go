package codecs

import (
	"reflect"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"

	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/link"
)

// HashStringConverter marshals/unmarshals a hash.Hash object to/from a string.
type HashConverter struct{}

func (x *HashConverter) ConvertExt(v interface{}) interface{} {
	b, err := v.(*hash.Hash).MarshalText()
	if err != nil {
		panic(errors.E("HashConverter.ConvertExt", err))
	}
	return b
}

func (x *HashConverter) UpdateExt(dest interface{}, v interface{}) {
	dst := dest.(*hash.Hash)
	switch t := dereference(v).Interface().(type) {
	case []byte:
		err := dst.UnmarshalText(t)
		if err != nil {
			panic(errors.E("HashConverter.UpdateExt", err))
		}
	default:
		panic(errors.E("HashConverter.UpdateExt", errors.K.Invalid,
			"expected_types", []string{"[]byte"},
			"actual_type", reflect.ValueOf(t).String()))
	}
}

// ===== IDConverter =========================================================

// IDConverter marshals/unmarshals a id.ID object to/from a byte array.
type IDConverter struct{}

func (x *IDConverter) ConvertExt(v interface{}) interface{} {
	return []byte(v.(id.ID))
}

func (x *IDConverter) UpdateExt(dest interface{}, v interface{}) {
	dst := dest.(*id.ID)
	switch t := dereference(v).Interface().(type) {
	case []byte:
		*dst = t
	default:
		panic(errors.E("IDConverter.UpdateExt", errors.K.Invalid,
			"expected_types", []string{"[]byte"},
			"actual_type", reflect.ValueOf(t).String()))
	}
}

// ===== LinkConverter ===================================================

// LinkConverter marshals/unmarshals a link.LinkObject to/from a map.
type LinkConverter struct{}

func (x *LinkConverter) ConvertExt(v interface{}) interface{} {
	l := v.(*link.Link)
	return l.MarshalCBOR()
}

func (x *LinkConverter) UpdateExt(dest interface{}, v interface{}) {
	dst := dest.(*link.Link)
	switch t := dereference(v).Interface().(type) {
	case map[string]interface{}:
		dst.UnmarshalCBOR(t)
	default:
		panic(errors.E("LinkConverter.UpdateExt", errors.K.Invalid,
			"expected_types", []string{"map[string]interface{}"},
			"actual_type", reflect.ValueOf(t).String()))
	}
}

// ===== LinkStringConverter =============================================

// LinkStringConverter marshals/unmarshals a link.LinkObject to/from a string.
type LinkStringConverter struct{}

func (x *LinkStringConverter) ConvertExt(v interface{}) interface{} {
	l := v.(*link.Link)
	return l.String()
}

func (x *LinkStringConverter) UpdateExt(dest interface{}, v interface{}) {
	dst := dest.(*link.Link)
	switch t := dereference(v).Interface().(type) {
	case string:
		l, err := link.FromString(t)
		if err != nil {
			panic(errors.E("LinkStringConverter.UpdateExt", err))
		}
		*dst = *l
	default:
		panic(errors.E("LinkStringConverter.UpdateExt", errors.K.Invalid,
			"expected_types", []string{"string"},
			"actual_type", reflect.ValueOf(t).String()))
	}
}

// ===== UTC Converter =========================================================

// UTCConverter marshals/unmarshals a utc.UTC object to/from binary format of
// time.Time.
type UTCConverter struct{}

func (x *UTCConverter) ConvertExt(v interface{}) interface{} {
	b, err := v.(*utc.UTC).MarshalBinary()
	if err != nil {
		panic(errors.E("UTCConverter.ConvertExt", err))
	}
	return b
}

func (x *UTCConverter) UpdateExt(dest interface{}, v interface{}) {
	dst := dest.(*utc.UTC)
	switch t := dereference(v).Interface().(type) {
	case []byte:
		err := dst.UnmarshalBinary(t)
		if err != nil {
			panic(errors.E("UTCConverter.UpdateExt", err))
		}
	default:
		panic(errors.E("UTCConverter.UpdateExt", errors.K.Invalid,
			"expected_types", []string{"[]byte"},
			"actual_type", reflect.ValueOf(t).String()))
	}
}

// ===== Helper functions ======================================================

// dereference converts the given value to a reflect.Value, de-referencing any
// pointer indirections.
func dereference(v interface{}) reflect.Value {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv
}
