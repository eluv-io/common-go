package jsonutil

import (
	"reflect"
	"strings"

	"github.com/eluv-io/errors-go"
)

// FieldTracker is a map that can be used to track which fields (keys) in a JSON
// object (map) are set. When unmarshalling into a FieldTracker, only the keys
// are retained and all values are discarded.
type FieldTracker map[string]*swallow

func (f FieldTracker) contains(field string) bool {
	_, ok := f[field]
	return ok
}

type swallow struct{}

func (f *swallow) UnmarshalJSON(bytes []byte) error {
	// simply ignore
	return nil
}

// SetDefaults copies the default values to the given target struct. Both target
// and defaults are expected to be structs. The fieldMap indicates which values
// on the target were explicitly set and should therefore not be overwritten.
// The fieldMap has to be a map with string keys (for example a FieldTracker).
func SetDefaults(def, target, fieldMap interface{}) error {
	e := errors.Template("set defaults", errors.K.Invalid)

	tracker, err := ToFieldTracker(fieldMap)
	if err != nil {
		return e(err)
	}

	tval := dereference(reflect.ValueOf(target))
	if tval.Kind() != reflect.Struct {
		return e("reason", "target not a struct")
	}
	ttyp := tval.Type()

	dval := dereference(reflect.ValueOf(def))
	if dval.Kind() != reflect.Struct {
		return e("reason", "default not a struct")
	}
	dtyp := dval.Type()

	tfn := GetJsonFields(ttyp)
	dfn := GetJsonFields(dtyp)

	for name, defIndex := range dfn {
		targetIndex := tfn[name]
		if !tracker.contains(name) && targetIndex != nil {
			field := tval.FieldByIndex(targetIndex)
			if field.CanSet() {
				field.Set(dval.FieldByIndex(defIndex))
			}
		}
	}

	return nil
}

// ToFieldTracker converts the given map to a FieldTracker. Returns an error if
// the fieldMap is not a map with string keys.
func ToFieldTracker(fieldMap interface{}) (FieldTracker, error) {
	if fm, ok := fieldMap.(FieldTracker); ok {
		return fm, nil
	}

	mv := reflect.ValueOf(fieldMap)
	if mv.Kind() != reflect.Map || mv.Type().Key().Kind() != reflect.String {
		return nil, errors.E("tracker", errors.K.Invalid,
			"reason", "not a map[string]*",
			"type", errors.TypeOf(fieldMap))
	}

	res := FieldTracker{}
	iter := mv.MapRange()
	for iter.Next() {
		k := iter.Key().String()
		res[k] = &swallow{}
	}

	return res, nil
}

func dereference(val reflect.Value) reflect.Value {
	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return val
}

// ParseJsonTag parses the given struct tag and determines whether it contains a
// JSON key, the JSON name and the squash flag (defined by the mapstructure
// lib).
func ParseJsonTag(tag reflect.StructTag) (hasJson bool, name string, squash bool) {
	if tag == "" {
		return false, "", false
	}

	jsn, ok := tag.Lookup("json")
	if !ok {
		return false, "", false
	}

	split := strings.Split(jsn, ",")
	if len(split) > 0 {
		name = split[0]
		for _, item := range split[1:] {
			if item == "squash" {
				squash = true
			}
		}
	}
	return true, name, squash
}

// GetJsonFields extracts the field names of the given struct type just like
// the json library does using 'json' struct tags.
//
// The returned map contains the "field index" (see reflect.StructField.Index)
// for each field name.
func GetJsonFields(typ reflect.Type) map[string][]int {
	res := map[string][]int{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			// unexported field
			continue
		}
		hasJson, name, _ := ParseJsonTag(field.Tag)
		if hasJson {
			res[name] = field.Index
		} else {
			res[field.Name] = field.Index
		}
	}
	return res
}
