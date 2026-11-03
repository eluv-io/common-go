package structured

import (
	"io"
	"strconv"

	"github.com/eluv-io/common-go/util/maputil"
	"github.com/eluv-io/common-go/util/sliceutil"
)

// VisitFn is the visitor function. Returns true to continue the visit, false
// to stop.
type VisitFn func(path Path, val interface{}) bool

// Visit visits each element in the target data structure. Elements in maps are
// visited in alphabetical order if orderMaps is set to true.
func Visit(target interface{}, orderMaps bool, f VisitFn) {
	rep := func(path Path, val interface{}) (replace bool, newVal interface{}, err error) {
		cont := f(path, val)
		if cont {
			return false, nil, nil
		} else {
			return false, nil, io.EOF
		}
	}
	path := make(Path, 0, 20)
	_, _, _ = doReplace(path, target, rep, orderMaps)
}

// ReplaceFn is the visitor function called for each element in the target
// structure. It returns true and a new value if the the element should be
// replaced, false otherwise. If a non-nil error value is returned, the visit is
// cancelled immediately.
//
// By default, the visit does not continue within a new value. Wrap the new value with VisitReplacement to visit its
// children as well.
type ReplaceFn func(path Path, val interface{}) (replace bool, newVal interface{}, err error)

// VisitReplacement wraps a new value returned by a ReplaceFn. Replace and ReplaceCopy then continue the visit within
// the new value: the ReplaceFn is called for the children of the new value, but not for the new value itself.
func VisitReplacement(val any) any {
	return visitReplacement{val: val}
}

type visitReplacement struct {
	val any
}

// Replace visits every element in the given target structure and calls the
// provided replacement function with it. The target structure is modified in place.
func Replace(target interface{}, f ReplaceFn) (interface{}, error) {
	path := make(Path, 0, 20)
	_, val, err := doReplace(path, target, f, false)
	return val, err
}

func doReplace(path Path, target interface{}, f ReplaceFn, orderMaps bool) (bool, interface{}, error) {
	replace, n, err := f(path, target)
	if err != nil {
		return false, nil, err
	}
	if !replace {
		node, err := doReplaceChildren(path, target, f, orderMaps)
		return false, node, err
	}
	if vr, ok := n.(visitReplacement); ok {
		node, err := doReplaceChildren(path, vr.val, f, orderMaps)
		return true, node, err
	}
	return true, n, nil
}

func doReplaceChildren(path Path, target interface{}, f ReplaceFn, orderMaps bool) (interface{}, error) {
	node := dereference(target)
	switch t := node.(type) {
	case map[string]interface{}:
		if orderMaps {
			keys := maputil.SortedKeys(t)
			for _, key := range keys {
				val := t[key]
				if replace, n, err := doReplace(path.CopyAppend(key), val, f, orderMaps); err != nil {
					return nil, err
				} else if replace {
					t[key] = n
				}
			}
		} else {
			for key, val := range t {
				if replace, n, err := doReplace(path.CopyAppend(key), val, f, orderMaps); err != nil {
					return nil, err
				} else if replace {
					t[key] = n
				}
			}
		}
	case []interface{}:
		for idx, val := range t {
			if replace, n, err := doReplace(path.CopyAppend(strconv.Itoa(idx)), val, f, orderMaps); err != nil {
				return nil, err
			} else if replace {
				t[idx] = n
			}
		}
	}
	return node, nil
}

// ReplaceCopy is like Replace, but leaves the target structure unchanged. A map or slice is copied only if one of its
// elements is replaced, directly or further down. All other maps and slices are shared between the target and the
// returned structure. The ReplaceFn receives the elements of the target and must not modify them.
func ReplaceCopy(target any, f ReplaceFn) (any, error) {
	path := make(Path, 0, 20)
	_, val, err := replaceCopy(path, target, f)
	return val, err
}

// replaceCopy visits the given element and returns true and the replacement if it or one of its children was
// replaced, false and the (dereferenced) element itself otherwise.
func replaceCopy(path Path, target any, f ReplaceFn) (bool, any, error) {
	replace, n, err := f(path, target)
	if err != nil {
		return false, nil, err
	}
	if !replace {
		return replaceCopyChildren(path, target, f)
	}
	if vr, ok := n.(visitReplacement); ok {
		_, node, err := replaceCopyChildren(path, vr.val, f)
		return true, node, err
	}
	return true, n, nil
}

// replaceCopyChildren visits the children of the given element. If one of them is replaced, it returns true and a
// copy of the element with the replaced children. Otherwise, it returns false and the (dereferenced) element itself.
func replaceCopyChildren(path Path, target any, f ReplaceFn) (bool, any, error) {
	node := dereference(target)
	switch t := node.(type) {
	case map[string]any:
		var cpy map[string]any
		for key, val := range t {
			changed, n, err := replaceCopy(path.CopyAppend(key), val, f)
			if err != nil {
				return false, nil, err
			}
			if changed {
				if cpy == nil {
					cpy = maputil.Copy(t)
				}
				cpy[key] = n
			}
		}
		if cpy != nil {
			return true, cpy, nil
		}
	case []any:
		var cpy []any
		for idx, val := range t {
			changed, n, err := replaceCopy(path.CopyAppend(strconv.Itoa(idx)), val, f)
			if err != nil {
				return false, nil, err
			}
			if changed {
				if cpy == nil {
					cpy = sliceutil.Copy(t)
				}
				cpy[idx] = n
			}
		}
		if cpy != nil {
			return true, cpy, nil
		}
	}
	return false, node, nil
}
