package maputil

import (
	"sort"
)

// SortedKeys returns a slice with the sorted keys of the given map.
func SortedKeys(m map[string]interface{}) []string {
	keys := make([]string, len(m))
	i := 0
	for key, _ := range m {
		keys[i] = key
		i++
	}
	sort.Strings(keys)
	return keys
}

// From creates a map[string]interface{} and adds all name value pairs to it.
// The arguments of the function are expected to be pairs of (string, interface{})
// parameters, e.g. maputil.From("op", "add", "val1", 4, "val2", 32)
func From(nameValuePairs ...interface{}) map[string]interface{} {
	return Add(nil, nameValuePairs...)
}

// Add adds the given nameValuePairs to the given map. If the map is nil, it
// creates a map[string]interface{} and adds all name value pairs to it.
func Add(m map[string]interface{}, nameValuePairs ...interface{}) map[string]interface{} {
	if len(nameValuePairs)/2 == 0 {
		return m
	}
	if m == nil {
		m = make(map[string]interface{}, len(nameValuePairs)/2)
	}
	for idx := 0; idx < len(nameValuePairs)-1; idx += 2 {
		m[nameValuePairs[idx].(string)] = nameValuePairs[idx+1]
	}
	return m
}
