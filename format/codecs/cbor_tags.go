package codecs

import (
	"reflect"

	"eluvio/errors"
	"eluvio/format/hash"
	"eluvio/format/id"
	"eluvio/format/link"
	"eluvio/format/structured"
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
	switch t := derefence(v).Interface().(type) {
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
	switch t := derefence(v).Interface().(type) {
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
	m := make(map[string]interface{})
	if !l.Target.IsNil() {
		m["Target"] = l.Target
	}
	if len(l.Selector) > 0 {
		m["Selector"] = l.Selector
	}
	if len(l.Path) > 0 {
		m["Path"] = l.Path
	}
	if l.Off > 0 {
		m["Off"] = l.Off
	}
	if l.Len > 0 {
		m["Len"] = l.Len
	}
	if len(l.Props) > 0 {
		m["Props"] = l.Props
	}
	return m
}

func (x *LinkConverter) UpdateExt(dest interface{}, v interface{}) {
	dst := dest.(*link.Link)
	switch t := derefence(v).Interface().(type) {
	case map[string]interface{}:
		var e interface{}
		var ok bool
		if e, ok = t["Target"]; ok {
			h := e.(hash.Hash)
			dst.Target = &h
		}
		if e, ok = t["Selector"]; ok {
			dst.Selector = link.Selector(e.(string))
		}
		if e, ok = t["Path"]; ok {
			dst.Path = toPath(e.([]interface{}))
		}
		if e, ok = t["Off"]; ok {
			dst.Off = toInt64(e)
		}
		if e, ok = t["Len"]; ok {
			dst.Len = toInt64(e)
		} else {
			dst.Len = -1
		}
		if e, ok = t["Props"]; ok {
			dst.Props = e.(map[string]interface{})
		}
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
	switch t := derefence(v).Interface().(type) {
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

// ===== Helper functions ======================================================

// derefence converts the given value to a reflect.Value, de-referencing any
// pointer indirections.
func derefence(v interface{}) reflect.Value {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	return rv
}

// Converts the given value to an int64 if it is any of the possible Go integer
// types. Returns 0 otherwise.
func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	// signed
	case int:
		return int64(t)
	case int64:
		return t
	case int32:
		return int64(t)
	case int16:
		return int64(t)
	case int8:
		return int64(t)
	// unsigned
	case uint:
		return int64(t)
	case uint64:
		return int64(t)
	case uint32:
		return int64(t)
	case uint16:
		return int64(t)
	case uint8:
		return int64(t)
	}
	return 0
}

func toPath(p []interface{}) structured.Path {
	s := make([]string, len(p))
	for idx, val := range p {
		s[idx] = val.(string)
	}
	return s
}
