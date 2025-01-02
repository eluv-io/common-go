package structured

import (
	"github.com/eluv-io/common-go/util/maputil"
	"github.com/eluv-io/common-go/util/sliceutil"
)

// standards:
//
// JSON Merge Patch - https://tools.ietf.org/html/rfc7386
// JSON Patch       - https://tools.ietf.org/html/rfc6902, http://jsonpatch.com/
// JSON Pointer     - https://tools.ietf.org/html/rfc6901
// JSON Reference   - http://tools.ietf.org/html/draft-pbryan-zyp-json-ref-03
// JSONPath         -

// Merge calls MergeWithOptions with default MergeOptions MakeCopy=false
func Merge(target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	return MergeWithOptions(MergeOptions{MakeCopy: false}, target, path, sources...)
}

// MergeCopy calls MergeWithOptions with MakeCopy=true
func MergeCopy(target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	return MergeWithOptions(MergeOptions{MakeCopy: true}, target, path, sources...)
}

type mergeCtx struct {
	MergeOptions
}

// MergeWithOptions merges the given source data structures into the target structure at the provided path. The merge
// operation first merges all source structures into a consolidated source before merging that into the target structure
// at the provided path. In case of conflicts (see below) priority is given to the sources in reverse order (i.e. last
// source wins).
//
// The merge operation traverses the target structure starting at path. For each sub-path, it finds the same sub-path in
// the source. If the data types are different, the source value replaces the target value. If the data types are the
// same, the merge is performed as follows:
//
//   - map[string]interface{}: key/value pairs from the sources are added to the
//     target structure. If a key already exists in the target, its value is
//     replaced by the value of the source.
//   - []interface{}: elements of the source slice are appended to the target's
//     slice
//   - any other value: the source value replaces the target value
//
// If sources is empty, the target structure is returned unchanged.
func MergeWithOptions(opts MergeOptions, target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	ctx := mergeCtx{opts}
	return ctx.doMerge(target, path, sources...)
}

func (ctx *mergeCtx) doMerge(target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	sub, err := resolveSub(path, target, true, ctx.MakeCopy)
	if err != nil {
		return nil, err
	}

	if sources == nil || len(sources) == 0 {
		if ctx.MakeCopy {
			return Copy(target), nil
		}
		return target, nil
	}

	// merge all sources into one
	src := dereference(sources[0])
	for i := 1; i < len(sources); i++ {
		src = ctx.merge(src, dereference(sources[i]))
	}

	res := ctx.merge(sub.Get(), src)
	sub.Set(res, false)
	return sub.Root(), nil
}

// merge merges two structures consisting of map[string]interface{}, []interface{}, or simple values (as returned by
// json.Unmarshal()).
//
// Maps are merged by adding all kv-pairs from x2 to x1, potentially replacing entries in x1. Arrays are merged
// according to the specified merge mode.
//
// If MergeOptions.MakeCopy is false, both x1 and/or x2 may be modified. Otherwise, the result is copied into a separate
// structure.
func (ctx *mergeCtx) merge(x1, x2 interface{}) interface{} {
	switch t1 := x1.(type) {
	case map[string]interface{}:
		m2, ok := x2.(map[string]interface{})
		if !ok {
			// non-map replaces existing map
			return x2
		}
		if ctx.MakeCopy {
			t1 = maputil.Copy(t1)
		}
		for k, v2 := range m2 {
			if v2 == nil {
				delete(t1, k)
			} else if v1, ok := t1[k]; ok {
				t1[k] = ctx.merge(v1, v2)
			} else {
				t1[k] = v2
			}
		}
		return t1
	case []interface{}:
		a2, ok := x2.([]interface{})
		if !ok || ctx.ArrayMergeMode == ArrayMergeModes.Replace() {
			// non-array or "replace mode" replaces existing array
			return x2
		}
		switch ctx.ArrayMergeMode {
		case ArrayMergeModes.Append():
			t1 = sliceutil.Append(a2, t1, ctx.MakeCopy)
		case ArrayMergeModes.Dedupe():
			t1 = sliceutil.SquashAndDedupe(a2, t1, ctx.MakeCopy)
		case ArrayMergeModes.Squash():
			fallthrough
		default:
			t1 = sliceutil.Squash(a2, t1, ctx.MakeCopy)
		}
		return t1
	default:
		return x2
	}
}
