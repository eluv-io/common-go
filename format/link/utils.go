package link

import "github.com/qluvio/content-fabric/format/structured"

// ConvertLinks scans the target structure for links represented as a map with a
// single "/" key, and converts them to link objects.
func ConvertLinks(target interface{}) (interface{}, error) {
	return structured.Replace(target, func(path structured.Path, val interface{}) (replace bool, newVal interface{}, err error) {
		switch t := val.(type) {
		case map[string]interface{}:
			lo, found := t["/"]
			if found {
				if _, ok := lo.(string); ok {
					l := &Link{}
					err = l.UnmarshalMap(t)
					if err == nil {
						return true, l, nil
					} else {
						return false, nil, err
					}
				}
			}
		}
		return false, nil, nil
	})
}
