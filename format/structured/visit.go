package structured

import (
	"io"
	"strconv"

	"github.com/eluv-io/common-go/util/maputil"
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
type ReplaceFn func(path Path, val interface{}) (replace bool, newVal interface{}, err error)

// Replace visits every element in the given target structure and calls the
// provided replacement function with it.
func Replace(target interface{}, f ReplaceFn) (interface{}, error) {
	path := make(Path, 0, 20)
	_, val, err := doReplace(path, target, f, false)
	return val, err
}

func doReplace(path Path, target interface{}, f ReplaceFn, orderMaps bool) (bool, interface{}, error) {
	if replace, n, err := f(path, target); err != nil {
		return false, nil, err
	} else if replace {
		return true, n, nil
	}

	node := dereference(target)
	switch t := node.(type) {
	case map[string]interface{}:
		if orderMaps {
			keys := maputil.SortedKeys(t)
			for _, key := range keys {
				val := t[key]
				if replace, n, err := doReplace(path.CopyAppend(key), val, f, orderMaps); err != nil {
					return false, nil, err
				} else if replace {
					t[key] = n
				}
			}
		} else {
			for key, val := range t {
				if replace, n, err := doReplace(path.CopyAppend(key), val, f, orderMaps); err != nil {
					return false, nil, err
				} else if replace {
					t[key] = n
				}
			}
		}
	case []interface{}:
		for idx, val := range t {
			if replace, n, err := doReplace(path.CopyAppend(strconv.Itoa(idx)), val, f, orderMaps); err != nil {
				return false, nil, err
			} else if replace {
				t[idx] = n
			}
		}
	}
	return false, node, nil
}
