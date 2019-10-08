package structured

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"eluvio/errors"
)

// Unflatten reverses the process of flattening: it turns a flattened structure
// into generic go maps and slices.
func Unflatten(flat [][3]string, separator ...string) (interface{}, error) {
	f := &unflatten{
		separator: "/",
	}
	if len(separator) > 0 {
		f.separator = separator[0]
	}
	if f.separator == "/" {
		f.decoder = rfc6901Decoder
	} else {
		f.decoder = strings.NewReplacer("~1", f.separator, "~0", "~")
	}
	return f.Unflatten(flat)
}

type unflatten struct {
	separator string
	decoder   *strings.Replacer
}

func (u *unflatten) Unflatten(flat [][3]string) (interface{}, error) {
	if len(flat) == 0 {
		return nil, nil
	}

	item := flat[0]
	if item[0] != u.separator {
		return nil, errors.E("unflatten", errors.K.Invalid, errors.Str("invalid path"), "path", item[0], "index", 0)
	}

	var root interface{}
	switch item[2] {
	case "object":
		root = make(map[string]interface{})
	case "array":
		root = make([]interface{}, 5)
	case "null":
		return nil, nil
	default:
		return nil, errors.E("unflatten", errors.K.Invalid, errors.Str("invalid object type"), "path", item[0], "index", 0, "type", item[2])
	}

	res, _, err := u.unflatten([]string{""}, root, flat[1:], 0)
	return res, err
}

func (u *unflatten) unflatten(parentPath []string, container interface{}, flat [][3]string, idx int) (interface{}, int, error) {
	for idx < len(flat) {
		item := flat[idx]
		segments := strings.Split(item[0], u.separator)
		if len(parentPath) >= len(segments) || !reflect.DeepEqual(parentPath, segments[0:len(parentPath)]) {
			// finished with current container
			break
		}
		lastSeg := segments[len(segments)-1]
		idx++

		var val interface{}
		var err error
		//var i int64
		//var ui uint64
		//var f float64

		valString := item[1]
		switch item[2] {
		case "object":
			val = make(map[string]interface{})
			val, idx, err = u.unflatten(segments, val, flat, idx)
		case "array":
			val = make([]interface{}, 0, 5)
			val, idx, err = u.unflatten(segments, val, flat, idx)
		case "string":
			val = valString
		case "bool":
			switch valString {
			case "true":
				val = true
			case "false":
				val = false
			default:
				return nil, -1, errors.E("unflatten", errors.K.Invalid, errors.Str("invalid bool"), "path", item[0], "value", valString)
			}
		case "number":
			val = json.Number(valString)
		case "int":
			val, err = strconv.ParseInt(valString, 10, 64)
		case "float":
			val, err = strconv.ParseFloat(valString, 64)

		// CBOR encoding only knows positive and negative integers and one
		// flavor of floats. They are unmarshaled into int64, uint64 and
		// float64. Hence the code below, even though correct, creates problems
		// converting back-and-forth between flat and structured data.
		//
		//case "int":
		//	i, err = strconv.ParseInt(valString, 10, 64)
		//	val = int(i)
		//case "int8":
		//	i, err = strconv.ParseInt(valString, 10, 8)
		//	val = int8(i)
		//case "int16":
		//	i, err = strconv.ParseInt(valString, 10, 16)
		//	val = int16(i)
		//case "int32":
		//	i, err = strconv.ParseInt(valString, 10, 32)
		//	val = int32(i)
		//case "int64":
		//	val, err = strconv.ParseInt(valString, 10, 64)
		//case "uint":
		//	ui, err = strconv.ParseUint(valString, 10, 64)
		//	val = uint(ui)
		//case "uint8":
		//	ui, err = strconv.ParseUint(valString, 10, 8)
		//	val = uint8(ui)
		//case "uint16":
		//	ui, err = strconv.ParseUint(valString, 10, 16)
		//	val = uint16(ui)
		//case "uint32":
		//	ui, err = strconv.ParseUint(valString, 10, 32)
		//	val = uint32(ui)
		//case "uint64":
		//	val, err = strconv.ParseUint(valString, 10, 64)
		//case "float32":
		//	f, err = strconv.ParseFloat(valString, 32)
		//	val = float32(f)
		//case "float64":
		//	val, err = strconv.ParseFloat(valString, 64)
		case "null":
			val = nil
		default:
			return nil, -1, errors.E("unflatten", errors.K.Invalid, errors.Str("unknown value type"), "path", item[0], "value", valString, "type", item[2])
		}

		if err != nil {
			return nil, -1, err
		}

		switch t := container.(type) {
		case map[string]interface{}:
			t[u.decoder.Replace(lastSeg)] = val
		case []interface{}:
			container = append(t, val)
		}
	}
	return container, idx, nil
}

// recursiveMerge merges maps and slices, or returns b for scalars
func recursiveMerge(a, b interface{}) (interface{}, error) {
	switch a.(type) {

	case map[string]interface{}:
		bMap, ok := b.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot merge object with non-object")
		}
		return recursiveMapMerge(a.(map[string]interface{}), bMap)

	case []interface{}:
		bSlice, ok := b.([]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot merge array with non-array")
		}
		return recursiveSliceMerge(a.([]interface{}), bSlice)

	case string, int, float64, bool, nil:
		// Can't merge them, second one wins
		return b, nil

	default:
		return nil, fmt.Errorf("unexpected data type for merge")
	}
}

// recursiveMapMerge recursively merges map[string]interface{} values
func recursiveMapMerge(a, b map[string]interface{}) (map[string]interface{}, error) {
	// Merge keys from b into a
	for k, v := range b {
		_, exists := a[k]
		if !exists {
			// Doesn't exist in a, just add it in
			a[k] = v
		} else {
			// Does exist, merge the values
			merged, err := recursiveMerge(a[k], b[k])
			if err != nil {
				return nil, err
			}

			a[k] = merged
		}
	}
	return a, nil
}

// recursiveSliceMerge recursively merged []interface{} values
func recursiveSliceMerge(a, b []interface{}) ([]interface{}, error) {
	// We need a new slice with the capacity of whichever
	// slive is biggest
	outLen := len(a)
	if len(b) > outLen {
		outLen = len(b)
	}
	out := make([]interface{}, outLen)

	// Copy the values from 'a' into the output slice
	copy(out, a)

	// Add the values from 'b'; merging existing keys
	for k, v := range b {
		if out[k] == nil {
			out[k] = v
		} else if v != nil {
			merged, err := recursiveMerge(out[k], b[k])
			if err != nil {
				return nil, err
			}
			out[k] = merged
		}
	}
	return out, nil
}
