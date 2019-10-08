package structured

// standards:
//
// JSON Merge Patch - https://tools.ietf.org/html/rfc7386
// JSON Patch       - https://tools.ietf.org/html/rfc6902, http://jsonpatch.com/
// JSON Pointer     - https://tools.ietf.org/html/rfc6901
// JSON Reference   - http://tools.ietf.org/html/draft-pbryan-zyp-json-ref-03
// JSONPath         -

// Merge merges the given source structures (simple value, map or array) into the
// target structure at the provided path.
//
// If source is nil, the target structure is returned unchanged.
func Merge(target interface{}, path Path, sources ...interface{}) (interface{}, error) {
	sub, err := resolveSub(path, target, true)
	if err != nil {
		return nil, err
	}

	if sources == nil || len(sources) == 0 {
		return target, nil
	}

	// merge all sources into one
	src := dereference(sources[0])
	for i := 1; i < len(sources); i++ {
		src = merge(Path{}, src, dereference(sources[i]))
	}

	pathCopy := append(Path{}, path...)
	res := merge(pathCopy, sub.Get(), src)
	sub.Set(res)
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
func merge(path Path, x1, x2 interface{}) interface{} {
	switch x1 := x1.(type) {
	case map[string]interface{}:
		m2, ok := x2.(map[string]interface{})
		if !ok {
			// non-map replaces existing map
			return x2
			//return nil, errors.E("merge", errors.K.Invalid,
			//	"reason", "cannot merge non-map type into map",
			//	"type", fmt.Sprintf("%T", x2),
			//	"path", path)
		}
		for k, v2 := range m2 {
			if v2 == nil {
				delete(x1, k)
			} else if v1, ok := x1[k]; ok {
				x1[k] = merge(path.append(k), v1, v2)
			} else {
				x1[k] = v2
			}
		}
	case []interface{}:
		a2, ok := x2.([]interface{})
		if !ok {
			// non-array replaces existing array
			return x2
			//return nil, errors.E("merge", errors.K.Invalid,
			//	"reason", "cannot merge non-array type into array",
			//	"type", fmt.Sprintf("%T", x2),
			//	"path", path)
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
