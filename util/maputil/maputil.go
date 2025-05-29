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
func SortedKeys[Map ~map[K]V, K constraints.Ordered, V any](m Map) []K {
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

// SortedValues returns a slice with sorted values of the given map.
//
// See also: Sorted.
func SortedValues[Map ~map[K]V, K comparable, V constraints.Ordered](m Map) []V {
	values := make([]V, len(m))
	i := 0
	for _, val := range m {
		values[i] = val
		i++
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i] < values[j] {
			return true
		}
		return false
	})
	return values
}

// Sorted returns a slice with the values of the given map sorted according to their keys.
//
// See also: SortedPairs, SortedValues, SortedKeys.
func Sorted[Map ~map[K]V, K constraints.Ordered, V any](m Map) []V {
	keys := SortedKeys(m)
	values := make([]V, len(keys))
	for i, key := range keys {
		values[i] = m[key]
	}
	return values
}

// SortedPairs returns a slice with the key/value pairs of the given map sorted according to their keys.
func SortedPairs[Map ~map[K]V, K constraints.Ordered, V any](m Map) []KV[K, V] {
	pairs := make([]KV[K, V], 0, len(m))
	for key, val := range m {
		pairs = append(pairs, KV[K, V]{Key: key, Val: val})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Key < pairs[j].Key {
			return true
		}
		return false
	})
	return pairs
}

// KV is a key-value pair.
type KV[K comparable, V any] struct {
	Key K
	Val V
}

// From creates a map[string]interface{} and adds all name/value pairs to it.
// The arguments of the function are expected to be pairs of (string, interface{})
// parameters, e.g. maputil.From("op", "add", "val1", 4, "val2", 32)
func From(nameValuePairs ...interface{}) map[string]interface{} {
	return Add[map[string]interface{}](nil, nameValuePairs...)
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

// Add adds the given nameValuePairs to the given map. If the map is nil, it creates a map[K]V and adds
// all name value pairs to it. Panics if the "names" are not of type K. Assigns the zero value if values are not of type
// V. If there is an odd number of nameValuePairs, the last one is ignored.
func Add[Map ~map[K]V, K comparable, V any](m Map, nameValuePairs ...interface{}) Map {
	if len(nameValuePairs)/2 == 0 {
		return m
	}
	if m == nil {
		m = make(Map, len(nameValuePairs)/2)
	}
	for idx := 0; idx < len(nameValuePairs)-1; idx += 2 {
		if v, ok := nameValuePairs[idx+1].(V); ok {
			m[nameValuePairs[idx].(K)] = v
		} else {
			var zero V
			m[nameValuePairs[idx].(K)] = zero
		}
	}
	return m
}

// Copy creates a shallow copy of the given map.
func Copy[Map ~map[K]V, K constraints.Ordered, V any](m Map) Map {
	if m == nil {
		return nil
	}
	cp := make(Map, len(m))
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
func Clear[Map ~map[K]V, K constraints.Ordered, V any](m Map) {
	for k, _ := range m {
		delete(m, k)
	}
}
