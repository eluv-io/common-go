package structured

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/maputil"
	"github.com/eluv-io/common-go/util/sliceutil"
	"github.com/eluv-io/common-go/util/stringutil"
)

var stringType = reflect.TypeOf("")

// sub is a structure that holds the result of a path resolution action. It
// allows to
//   - get the value at the resolved path
//   - set it to a new value
//   - and also get the potentially new root element of the target data structure
//     on which the path was resolved with the 'create' option
type sub interface {
	// Returns the value referenced by the path that was resolved
	Get() interface{}
	// Sets the value referenced by the path that was resolved
	Set(val interface{}, evenIfNil bool)
	// Returns the (potentially new) root element of the structure
	Root() interface{}
}

type subRoot struct {
	val interface{}
}

func (s *subRoot) Get() interface{}                    { return s.val }
func (s *subRoot) Set(val interface{}, evenIfNil bool) { _ = evenIfNil; s.val = val }
func (s *subRoot) Root() interface{} {
	return s.val
}

type subMap struct {
	root interface{}
	key  string
	m    map[string]interface{}
}

func (s *subMap) Get() interface{} { return s.m[s.key] }
func (s *subMap) Set(val interface{}, evenIfNil bool) {
	if val == nil && !evenIfNil {
		delete(s.m, s.key)
	} else {
		s.m[s.key] = val
	}
}
func (s *subMap) Root() interface{} { return s.root }

type subArr struct {
	root   interface{}
	idx    int
	arr    []interface{}
	parent sub
}

func (s *subArr) Get() interface{} { return s.arr[s.idx] }
func (s *subArr) Set(val interface{}, evenIfNil bool) {
	if val == nil && !evenIfNil {
		if s.parent != nil {
			s.parent.Set(append(s.arr[:s.idx], s.arr[s.idx+1:]...), false)
		} else {
			s.arr = append(s.arr[:s.idx], s.arr[s.idx+1:]...)
			s.root = s.arr
		}
	} else {
		s.arr[s.idx] = val
	}
}
func (s *subArr) Root() interface{} { return s.root }

// TransformerFn is a transformation function for data items encountered when
// resolving a path. Each data element on the path is passed to this function
// and continues with the returned element. If no transformation is needed,
// return the passed-in element unchanged.
//
// Path resolution can be stopped by the transformer function by returning false
// in the continuation flag. The returned data element will be passed back as
// the final result of the path resolution call.
//
// Any non-nil error return will fail the path resolution immediately.
//
// Params
//   - elem:     the data element
//   - path:     the path at which this element is located
//   - fullPath: the full path being resolved
//
// Returns
//   - trans: the transformed data element
//   - cont:  true to continue resolution, false otherwise
//   - err:   fails path resolution immediately if non-nil
type TransformerFn func(elem interface{}, path Path, fullPath Path) (trans interface{}, cont bool, err error)

// noopTransformerFn is a transformer function that returns the element
// unchanged.
var noopTransformerFn TransformerFn = func(elem interface{}, path Path, fullPath Path) (interface{}, bool, error) {
	return elem, true, nil
}

// Get returns the element at the given path in the target data structure.
// It's an alias for Resolve()
func Get(path Path, target interface{}) (interface{}, error) {
	return Resolve(path, target)
}

// Resolve resolves a path on the given target structure and returns the
// corresponding sub-structure.
func Resolve(path Path, target interface{}, transformerFns ...TransformerFn) (interface{}, error) {
	transformer := noopTransformerFn
	if len(transformerFns) > 0 {
		transformer = transformerFns[0]
	}
	return resolveTransform(path, target, transformer)
}

// StringAt returns the string value at the given path in the given target
// structure. The empty string "" is returned if the path does not exist or the
// value at path is not a string.
func StringAt(target interface{}, path ...string) string {
	val, err := Resolve(NewPath(path...), target)
	if err != nil {
		return ""
	}
	s, ok := val.(string)
	if ok {
		return s
	}
	return ""
}

// Float64At returns the float64 value at the given path in the given target structure.
// 0 is returned if the path does not exist or the value at path is not an int.
func Float64At(target interface{}, path ...string) float64 {
	var err error
	var res float64

	val, err := Resolve(NewPath(path...), target)
	if err != nil {
		return 0
	}

	switch n := val.(type) {
	case json.Number:
		res, err = n.Float64()
	case int64:
		res = float64(n)
	case string:
		res, err = strconv.ParseFloat(n, 64)
	case float64:
		res = n
	}

	if err != nil {
		return 0
	}

	return res
}

// Int64At returns the int64 value at the given path in the given target structure.
// 0 is returned if the path does not exist or the value at path is not an int.
func Int64At(target interface{}, path ...string) int64 {
	var err error
	var res int64

	val, err := Resolve(NewPath(path...), target)
	if err != nil {
		return 0
	}

	switch n := val.(type) {
	case json.Number:
		res, err = n.Int64()
	case float64:
		res = int64(n)
	case string:
		res, err = strconv.ParseInt(n, 10, 64)
	}

	if err != nil {
		return 0
	}

	return res
}

// BoolAt returns the bool value at the given path in the given target
// structure. False is returned if the path does not exist or the
// value at path is not a string.
func BoolAt(target interface{}, path ...string) bool {
	val, err := Resolve(NewPath(path...), target)
	if err != nil {
		return false
	}
	b, ok := val.(bool)
	if ok {
		return b
	}
	return false
}

// MapAt returns the map[string]interface{} value at the given path in the given
// target structure. Nil is returned if the path does not exist or the
// value at path is not a map (and can be used for map lookups, but not for
// putting values into the map!)
func MapAt(target interface{}, path ...string) map[string]interface{} {
	val, err := Resolve(NewPath(path...), target)
	if err != nil {
		return nil
	}
	s, ok := val.(map[string]interface{})
	if ok {
		return s
	}
	return nil
}

// SliceAt returns the []interface{} value at the given path in the given
// target structure. Nil is returned if the path does not exist or the
// value at path is not a slice.
func SliceAt(target interface{}, path ...string) []interface{} {
	val, err := Resolve(NewPath(path...), target)
	if err != nil {
		return nil
	}
	s, ok := val.([]interface{})
	if ok {
		return s
	}
	return nil
}

// StringSliceAt returns the []interface{} value at the given path in the given
// target structure as a []string. Nil is returned if the path does not exist or
// the value at path is not a []interface{}. Otherwise, each element in the
// slice is converted to a string with fmt.Sprintf("%s", element).
func StringSliceAt(target interface{}, path ...string) []string {
	is := SliceAt(target, path...)
	return stringutil.ToSlice(is)
}

// resolveSub resolves a path on the given target structure and returns a
// subtree object.
//
// params:
//   - path      : the path to resolve
//   - target    : the data structure to analyze
//   - create    : missing path segments are created if true, generate an error
//     otherwise
//   - copyStruct: create a copy of the target structure if needed in order to
//     ensure the target structure is not modified.
//
// return:
//   - a subtree object representing the value at path.
//     Its Get() method returns the value at path. Its Set() method allows to
//     replace that value. Its Root() returns the (potentially new) root of the
//     structure that was resolved.
//   - an error if the (full) path does not exist, unless the create parameter is
//     set to true. In the latter case, the given path is created in the target
//     structure by creating any missing maps and map entries, setting the final
//     path segment's value to an empty map.
func resolveSub(path Path, target interface{}, create bool, copyStruct bool) (sub, error) {
	e := errors.Template("resolve",
		"full_path", path,
		"create", create,
		"copy", copyStruct)

	cpy := func(t interface{}) interface{} { return t }
	if copyStruct {
		cpy = shallowCopy
	}

	target = dereference(target)

	if len(path) == 0 {
		return &subRoot{val: target}, nil
	} else if ifutil.IsNil(target) {
		target = map[string]interface{}{}
	}

	// copy of the target structure
	root := cpy(target)
	// current node (in the copied structure)
	node := root

	// the following three vars are used to track a node's parent, it's type
	// and key (in case of a map) or index (in case of an array). The same could
	// have been achieved by creating subMap or subArr objects for every visited
	// node, but that would obviously cause memory allocations every time. Hence
	// the more ugly but more efficient tracking using plain variables.
	var parent interface{}
	var parentKey string
	var parentIdx int

	for idx := 0; ; idx++ {
		lastPathSegment := idx+1 >= len(path)
		switch t := node.(type) {
		case map[string]interface{}:
			key := path[idx]
			v, found := t[key]
			if !found || (v == nil && !lastPathSegment) {
				if !create {
					return nil, e(errors.K.NotExist, "path", path[:idx+1])
				}
				if lastPathSegment {
					v = nil
				} else {
					v = map[string]interface{}{}
				}
				t[key] = v
			} else if !lastPathSegment {
				v = cpy(v)
				t[key] = v
			}
			if lastPathSegment {
				return &subMap{root: root, key: key, m: t}, nil
			}
			parent = t
			parentKey = key
			node = v
		case []interface{}:
			i, err := strconv.ParseInt(path[idx], 10, 32)
			if err != nil {
				return nil, e(errors.K.Invalid, "reason", "invalid array index", "path", path[:idx+1])
			}
			aidx := int(i)
			if aidx >= len(t) || aidx < 0 {
				return nil, e(errors.K.NotExist, "reason", "array index out of range", "path", path[:idx+1])
			}
			if lastPathSegment {
				var p sub
				if parent != nil {
					if parentKey != "" {
						p = &subMap{root: root, key: parentKey, m: parent.(map[string]interface{})}
					} else {
						p = &subArr{root: root, idx: parentIdx, arr: parent.([]interface{})}
					}
				}
				return &subArr{root: root, idx: aidx, arr: t, parent: p}, nil
			}
			parent = t
			parentKey = ""
			parentIdx = aidx
			node = cpy(t[aidx])
			t[aidx] = node
		case nil:
			return nil, e(errors.K.NotExist, "reason", "element is nil", "path", path[:idx+1])
		default:
			return nil, e(errors.K.Invalid, "reason", "element is leaf", "path", path[:idx+1])
		}
	}
}

func resolveTransform(path Path, target interface{}, transform TransformerFn) (interface{}, error) {
	var err error
	e := errors.Template("resolveTransform", "full_path", path)
	if transform == nil {
		transform = noopTransformerFn
	}

	node := dereference(target)

	if path == nil {
		path = Path{}
	}
	//if len(path) == 0 {
	//	node, _, err = transform(node, path, path)
	//	return node, e.IfNotNil(err, "path", path)
	//}

	for idx := 0; ; idx++ {
		var cont bool
		node, cont, err = transform(node, path[:idx], path)
		if err != nil {
			return nil, e(err, "path", path[:idx])
		}
		if !cont {
			return node, nil
		}
		if idx+1 > len(path) {
			return node, nil
		}
		switch t := node.(type) {
		case map[string]interface{}:
			v, found := t[path[idx]]
			if !found {
				return nil, e(errors.K.NotExist, "path", path[:idx+1])
			}
			node = v
		case []interface{}:
			i, err := strconv.ParseInt(path[idx], 10, 32)
			if err != nil {
				return nil, e(errors.K.Invalid, "reason", "invalid array index", "path", path[:idx+1])
			}
			aidx := int(i)
			if aidx < 0 {
				// treat negative index as offset from the end
				// i.e -1 is last element
				aidx = len(t) + aidx
			}
			if aidx >= len(t) || aidx < 0 {
				return nil, e(errors.K.NotExist, "reason", "array index out of range", "path", path[:idx+1])
			}
			node = t[aidx]
		case nil:
			return nil, e(errors.K.NotExist, "reason", "element is nil", "path", path[:idx])
		default:
			typ := reflect.TypeOf(node)
			val := reflect.ValueOf(node)

			switch typ.Kind() {
			case reflect.Map,
				reflect.Slice,
				reflect.Array,
				reflect.Ptr:

				if val.IsNil() {
					return nil, e(errors.K.NotExist,
						"reason", "element is nil",
						"path", path[:idx],
						"node_type", typ)
				}
			}

			if typ.Kind() == reflect.Ptr {
				// dereference pointer
				typ = typ.Elem()
				val = val.Elem()
			}

			if typ.Kind() == reflect.Map && stringType.ConvertibleTo(typ.Key()) {
				// MAP
				vv := val.MapIndex(reflect.ValueOf(path[idx]).Convert(typ.Key()))
				if !vv.IsValid() {
					return nil, e(errors.K.NotExist,
						"reason", "map field not found",
						"path", path[:idx+1])
				}
				node = vv.Interface()
			} else if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
				// SLICE or ARRAY
				i, err := strconv.ParseInt(path[idx], 10, 32)
				if err != nil {
					return nil, e(errors.K.Invalid,
						"reason", "invalid array index",
						"path", path[:idx+1])
				}
				aidx := int(i)
				if aidx < 0 {
					// treat negative index as offset from the end
					// i.e -1 is last element
					aidx = val.Len() + aidx
				}
				if aidx >= val.Len() || aidx < 0 {
					return nil, e(errors.K.NotExist,
						"reason", "array index out of range",
						"path", path[:idx+1])
				}
				node = val.Index(aidx).Interface()
			} else if typ.Kind() == reflect.Struct {
				// STRUCT
				if field, ok := typ.FieldByName(path[idx]); ok && (field.Tag == "" || tagMatches(field.Tag, path[idx])) {
					f := val.FieldByIndex(field.Index)
					if !f.CanInterface() {
						return nil, e(errors.K.Invalid,
							"reason", "struct field is not accessible",
							"path", path[:idx])
					}
					node = f.Interface()
				} else {
					found := false
					var squashed [][]int
					for i := 0; i < typ.NumField(); i++ {
						field = typ.Field(i)
						hasJson, name, squash := parseTag(field.Tag)
						if hasJson {
							if squash {
								squashed = append(squashed, field.Index)
							} else if name == path[idx] {
								node = val.FieldByIndex(field.Index).Interface()
								found = true
								break
							}
						}
					}
					if !found {
						for _, fieldIndex := range squashed {
							node, err = resolveTransform(
								path[idx:],
								val.FieldByIndex(fieldIndex).Interface(),
								transform)
							if err == nil {
								return node, nil
							}
						}
						return nil, e(errors.K.Invalid,
							"reason", "struct field not found",
							"path", path[:idx],
							"type", errors.TypeOf(node))
					}
				}
			} else {
				return nil, e(errors.K.Invalid,
					"reason", "element is leaf",
					"path", path[:idx],
					"node_type", typ)
			}
		}
	}
}

func tagMatches(tag reflect.StructTag, name string) bool {
	if tag == "" {
		return false
	}

	jsn, ok := tag.Lookup("json")
	if !ok {
		return false
	}

	split := strings.Split(jsn, ",")
	return len(split) > 0 && split[0] == name
}

// parseTag parses the given struct tag and determines whether it contains a
// JSON key, the JSON name and the squash flag (defined by the mapstructure
// lib).
func parseTag(tag reflect.StructTag) (hasJson bool, name string, squash bool) {
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

func shallowCopy(val interface{}) interface{} {
	switch t := val.(type) {
	case map[string]interface{}:
		return maputil.Copy(t)
	case []interface{}:
		return sliceutil.Copy(t)
	}
	return val
}
