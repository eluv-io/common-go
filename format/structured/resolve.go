package structured

import (
	"encoding/json"
	"strconv"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/util/stringutil"
)

// sub is a structure that holds the result of a path resolution action. It
// allows to
// - get the value at the resolved path
// - set it to a new value
// - and also get the potentially new root element of the target data structure
//   on which the path was resolved with the 'create' option
type sub interface {
	// Returns the value referenced by the path that was resolved
	Get() interface{}
	// Sets the value referenced by the path that was resolved
	Set(val interface{})
	// Returns the (potentially new) root element of the structure
	Root() interface{}
}

type subRoot struct {
	val interface{}
}

func (s *subRoot) Get() interface{}    { return s.val }
func (s *subRoot) Set(val interface{}) { s.val = val }
func (s *subRoot) Root() interface{} {
	return s.val
}

type subMap struct {
	root interface{}
	key  string
	m    map[string]interface{}
}

func (s *subMap) Get() interface{} { return s.m[s.key] }
func (s *subMap) Set(val interface{}) {
	if val == nil {
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
func (s *subArr) Set(val interface{}) {
	if val == nil {
		if s.parent != nil {
			s.parent.Set(append(s.arr[:s.idx], s.arr[s.idx+1:]...))
		} else {
			s.arr = append(s.arr[:s.idx], s.arr[s.idx+1:]...)
			s.root = s.arr
		}
	} else {
		s.arr[s.idx] = val
	}
}
func (s *subArr) Root() interface{} { return s.root }

type ResolveErrorHandler func(val interface{}, path Path)

// Get returns the element at the given path in the target data structure.
// It's an alias for Resolve()
func Get(path Path, target interface{}) (interface{}, error) {
	return Resolve(path, target)
}

// Resolve resolves a path on the given target structure and returns a
// pointer to the subtree.
func Resolve(path Path, target interface{}, errHandler ...ResolveErrorHandler) (interface{}, error) {
	eh := func(val interface{}, path Path) {}
	if len(errHandler) > 0 {
		eh = errHandler[0]
	}
	res, err := resolveSubEH(path, target, false, eh)
	if err != nil {
		return nil, err
	}
	return res.Get(), nil
}

// StringAt returns the string value at the given path in the given target
// structure. The empty string "" is returned if the path does not exist or the
// value at path is not a string.
func StringAt(target interface{}, path ...string) string {
	val, err := Resolve(Path{}.Append(path...), target)
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

	val, err := Resolve(Path{}.Append(path...), target)
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

	val, err := Resolve(Path{}.Append(path...), target)
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
	val, err := Resolve(Path{}.Append(path...), target)
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
	val, err := Resolve(Path{}.Append(path...), target)
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
	val, err := Resolve(Path{}.Append(path...), target)
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

func resolveSub(path Path, target interface{}, create bool) (sub, error) {
	return resolveSubEH(path, target, create, func(val interface{}, path Path) {})
}

// resolveSubEH resolves a path on the given target structure and returns a
// subtree object.
//
// params:
//  * path      : the path to resolve
//  * target    : the data structure to analyze
//  * create    : missing path segments are created if true, generate an error otherwise
//  * errHandler: a function that gets called when an error occurs
//
// return:
//  * a subtree object representing the value at path.
//    Its Get() method returns the value at path. Its Set() method allows to
//    replace that value. Its Root() returns the (potentially new) root of the
//    structure that was resolved.
//  * an error if the (full) path does not exist, unless the create parameter is
//    set to true. In the latter case, the given path is created in the target
//    structure by creating any missing maps and map entries, setting the final
//    path segment's value to an empty map.
func resolveSubEH(path Path, target interface{}, create bool, errHandler ResolveErrorHandler) (sub, error) {
	node := dereference(target)
	root := node

	if len(path) == 0 {
		return &subRoot{val: node}, nil
	} else if root == nil {
		root = map[string]interface{}{}
		node = root
	}

	// the following three vars are used to track a node's parent, it's type
	// and key (in case of a map) or index (in case of an array). The same could
	// have been achieved by creating subMap or subArr objects for every visited
	// node, but that would obviously cause memory allocations every time. Hence
	// the more ugly but more efficient tracking using plain variables.
	var parent interface{}
	var parentKey string
	var parentIdx int

	for idx := 0; ; idx++ {
		mkerr := func() *errors.Error {
			return errors.E("resolve", "path", path[:idx+1], "full_path", path)
		}
		lastPathSegment := idx+1 >= len(path)
		switch t := node.(type) {
		case map[string]interface{}:
			v, found := t[path[idx]]
			if !found {
				if !create {
					errHandler(t, path[:idx])
					return nil, mkerr().Kind(errors.K.NotExist)
				}
				if lastPathSegment {
					v = nil
				} else {
					v = map[string]interface{}{}
				}
				t[path[idx]] = v
			}
			if lastPathSegment {
				return &subMap{root: root, key: path[idx], m: t}, nil
			}
			parent = t
			parentKey = path[idx]
			node = v
		case []interface{}:
			i, err := strconv.ParseInt(path[idx], 10, 32)
			if err != nil {
				errHandler(t, path[:idx])
				return nil, mkerr().Kind(errors.K.Invalid).With("reason", "invalid array index")
			}
			aidx := int(i)
			if aidx >= len(t) || aidx < 0 {
				errHandler(t, path[:idx])
				return nil, mkerr().Kind(errors.K.NotExist).With("reason", "array index out of range")
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
			node = t[aidx]
		case nil:
			errHandler(t, path[:idx])
			return nil, mkerr().Kind(errors.K.NotExist).With("reason", "element is nil")
		default:
			errHandler(t, path[:idx])
			return nil, mkerr().Kind(errors.K.Invalid).With("reason", "element is leaf")
		}
	}
}
