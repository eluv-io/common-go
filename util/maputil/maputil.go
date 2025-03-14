package maputil

import (
	"encoding/json"
	"reflect"
	"sort"

	"golang.org/x/exp/constraints"

	"github.com/eluv-io/common-go/util/stringutil"
)

// SortedStringKeys returns a slice with the sorted keys of the given
// map[string]*. Returns an empty slice if m is not a map.
//
// Deprecated: use SortedKeys as it supports generic maps now
func SortedStringKeys(m interface{}) []string {
	mv := reflect.ValueOf(m)
	if mv.Kind() != reflect.Map {
		return []string{}
	}

	kvs := mv.MapKeys()
	keys := make([]string, len(kvs))
	i := 0
	for _, kv := range kvs {
		keys[i] = kv.String()
		i++
	}
	sort.Strings(keys)
	return keys
}

// SortedKeys returns a slice with the sorted keys of the given map.
func SortedKeys[K constraints.Ordered, V any](m map[K]V) []K {
	keys := make([]K, len(m))
	i := 0
	for key, _ := range m {
		keys[i] = key
		i++
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] < keys[j] {
			return true
		}
		return false
	})
	return keys
}

// From creates a map[string]interface{} and adds all name/value pairs to it.
// The arguments of the function are expected to be pairs of (string, interface{})
// parameters, e.g. maputil.From("op", "add", "val1", 4, "val2", 32)
func From(nameValuePairs ...interface{}) map[string]interface{} {
	return Add(nil, nameValuePairs...)
}

// FromJsonStruct creates a generic map from a struct with JSON tags. The
// purpose of this is to invoke the json.Marshaler hooks, albeit inefficiently.
//
// Note that mapstructure's Decoder can also do this conversion, but does not
// convert the children (or children of children, and so on).
func FromJsonStruct(i interface{}) (m interface{}, err error) {
	var jsonBytes []byte
	if jsonBytes, err = json.Marshal(i); err == nil {
		err = json.Unmarshal(jsonBytes, &m)
	}
	return
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

// Copy creates a shallow copy of the given map.
func Copy[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	cp := make(map[K]V, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// CopyMSI (MSI = Map String Interface) creates a shallow copy of the given map,
// assumed to have string keys. Returns an empty map if m is not a map.
//
// Deprecated: use Copy as it supports generic maps now
func CopyMSI(m interface{}) map[string]interface{} {
	mv := reflect.ValueOf(m)
	if mv.Kind() != reflect.Map {
		return map[string]interface{}{}
	}

	kvs := mv.MapKeys()
	ret := make(map[string]interface{})
	for _, kv := range kvs {
		i := mv.MapIndex(kv)
		ret[stringutil.ToString(kv.Interface())] = i.Interface()
	}
	return ret
}

// Clear clears the given map
func Clear[K comparable, V any](m map[K]V) {
	for k, _ := range m {
		delete(m, k)
	}
}
