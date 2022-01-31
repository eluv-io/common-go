package structured

import (
	"github.com/eluv-io/common-go/util/maputil"
)

// standards:
//
// JSON Merge Patch - https://tools.ietf.org/html/rfc7386
// JSON Patch       - https://tools.ietf.org/html/rfc6902, http://jsonpatch.com/
// JSON Pointer     - https://tools.ietf.org/html/rfc6901
// JSON Reference   - http://tools.ietf.org/html/draft-pbryan-zyp-json-ref-03
// JSONPath         -

// Merge calls MergeExt with copyStruct=false
func Merge(target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	return MergeExt(false, target, path, sources...)
}

// MergeCopy calls MergeExt with copyStruct=true
func MergeCopy(target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	return MergeExt(true, target, path, sources...)
}

// MergeExt merges the given source data structures into the target structure at
// the provided path. The merge operation first merges all source structures
// into a consolidated source before merging that into the target structure at
// the provided path. In case of conflicts (see below) priority is given to
// the sources in reverse order (i.e. last source wins).
//
// The merge operation traverses the target structure starting at path. For each
// subpath, it finds the same subpath in the source. If the data types are
// different, the source value replaces the target value. If the data types are
// the same, the merge is performed as follows:
// * map[string]interface{}: key/value pairs from the sources are added to the
//   target structure. If a key already exists in the target, its value is
//   replaced by the value of the source.
// * []interface{}: elements of the source slice are appended to the target's
//   slice
// * any other value: the source value replaces the target value
//
// If copyStruct is false, the target or source structures may be modified.
//
// If copyStruct is true, the target and source structures remain unmodified.
// Instead, the merged data is copied into a separate structure. Note, however,
// that the copy is shallow, so the result might still refer back to maps,
// slices or other objects in the target or source structures. Modifying those
// subsequently will therefore modify target or sources.
//
// If sources is empty, the target structure is returned unchanged.
func MergeExt(copyStruct bool, target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	sub, err := resolveSub(path, target, true, copyStruct)
	if err != nil {
		return nil, err
	}

	if sources == nil || len(sources) == 0 {
		return target, nil
	}

	mergeFn := merge
	if copyStruct {
		mergeFn = mergeCopy
	}

	// merge all sources into one
	src := dereference(sources[0])
	for i := 1; i < len(sources); i++ {
		src = mergeFn(src, dereference(sources[i]))
	}

	res := mergeFn(sub.Get(), src)
	sub.Set(res, false)
	return sub.Root(), nil
}

// merge merges two structures consisting of map[string]interface{},
// []interface{}, or simple values (as returned by json.Unmarshal()).
//
// Maps are merged by adding all kv-pairs from x2 to x1, potentially replacing
// entries in x1.
//
// Arrays are merged by appending x2 elements to x1.
//
// Both x1 and/or x2 may be modified.
func merge(x1, x2 interface{}) interface{} {
	switch x1 := x1.(type) {
	case map[string]interface{}:
		m2, ok := x2.(map[string]interface{})
		if !ok {
			// non-map replaces existing map
			return x2
		}
		for k, v2 := range m2 {
			if v2 == nil {
				delete(x1, k)
			} else if v1, ok := x1[k]; ok {
				x1[k] = merge(v1, v2)
			} else {
				x1[k] = v2
			}
		}
	case []interface{}:
		a2, ok := x2.([]interface{})
		if !ok {
			// non-array replaces existing array
			return x2
		}
		for _, v2 := range a2 {
			x1 = append(x1, v2)
		}
		return x1
	default:
		return x2
	}
	return x1
}

// mergeCopy is like merge, but creates a copy for the resulting data structure
// and ensures that x1 and x2 are never modified.
func mergeCopy(x1, x2 interface{}) interface{} {
	switch t1 := x1.(type) {
	case map[string]interface{}:
		m2, ok := x2.(map[string]interface{})
		if !ok {
			// non-map replaces existing map
			return x2
		}
		t1 = maputil.Copy(t1)
		for k, v2 := range m2 {
			if v2 == nil {
				delete(t1, k)
			} else if v1, ok := t1[k]; ok {
				t1[k] = mergeCopy(v1, v2)
			} else {
				t1[k] = v2
			}
		}
		return t1
	case []interface{}:
		a2, ok := x2.([]interface{})
		if !ok {
			// non-array replaces existing array
			return x2
		}
		t1 = copySlice(t1)
		for _, v2 := range a2 {
			t1 = append(t1, v2)
		}
		return t1
	default:
		return x2
	}
}

func copySlice(src []interface{}) []interface{} {
	res := make([]interface{}, len(src))
	copy(res, src)
	return res
}
