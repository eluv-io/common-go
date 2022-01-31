package structured

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/util/maputil"
)

// Flatten converts the given data structure into a list of flattened paths,
// their corresponding values and type information.
//
// The data structure must consist of only basic types, map[string]interface{}
// and []interface{} (the types used when unmarshaling JSON into a generic
// interface{})
//
// The result is a slice of triplets [path, value, type]
// E.g. [ "/", "{}", "object"]
//      [ "/first", "joe", "string" ]
//      [ "/last", "doe", "string" ]
//      [ "/age", "24", "number" ]
//      [ "/children", "[]", "array" ]
//      [ "/children/0", "fred", "string" ]
//      [ "/children/1", "cathy", "string" ]
//      [ "/children/3", "jenny", "string" ]
//
// Possible types are: object, array, string, bool, number, float64, null
//
// Slashes in path elements are encoded according to the JSON Pointer format
// defined in RFC 6901: https://tools.ietf.org/html/rfc6901
//
func Flatten(structure interface{}, separator ...string) ([][3]string, error) {
	f := &flatten{
		separator: "/",
	}
	if len(separator) > 0 {
		f.separator = separator[0]
	}
	if f.separator == "/" {
		f.encoder = rfc6901Encoder
	} else {
		f.encoder = strings.NewReplacer("~", "~0", f.separator, "~1")
	}
	return f.Flatten(structure)
}

type flatten struct {
	separator string
	encoder   *strings.Replacer
}

func (f *flatten) Flatten(structure interface{}) ([][3]string, error) {
	var list []*kvt

	rootPath := "$"
	if f.separator == "/" {
		rootPath = "/"
	}
	list, err := f.doFlatten(list, rootPath, structure)
	if err != nil {
		return nil, err
	}
	var res [][3]string
	for _, kvt := range list {
		res = append(res, [3]string{kvt.key, kvt.val, kvt.typ})
	}

	return res, nil
}

func (f *flatten) doFlatten(list []*kvt, key string, v interface{}) ([]*kvt, error) {
	entry, err := f.kvtFromValue(v)
	if err != nil {
		return nil, err
	}
	entry.key = key
	list = append(list, entry)

	// Recurse into objects and arrays
	switch vv := v.(type) {
	case map[string]interface{}:
		// It's an object
		keys := maputil.SortedKeys(vv)
		for _, k := range keys {
			list, err = f.doFlatten(list, f.createKey(key, k), vv[k])
			if err != nil {
				return nil, err
			}
		}

	case []interface{}:
		// It's an array
		for idx, sub := range vv {
			list, err = f.doFlatten(list, f.createNumKey(key, idx), sub)
			if err != nil {
				return nil, err
			}
		}
	}

	return list, nil
}

// kvtFromValue takes any valid value and
// returns a value kvt to represent it
func (f *flatten) kvtFromValue(v interface{}) (*kvt, error) {
	switch vv := v.(type) {
	case map[string]interface{}:
		return &kvt{val: "{}", typ: "object"}, nil
	case []interface{}:
		return &kvt{val: "[]", typ: "array"}, nil
	case json.Number:
		return &kvt{val: vv.String(), typ: "number"}, nil
	case string:
		return &kvt{val: vv, typ: "string"}, nil
	case bool:
		if vv {
			return &kvt{val: "true", typ: "bool"}, nil
		}
		return &kvt{val: "false", typ: "bool"}, nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return &kvt{val: fmt.Sprintf("%d", vv), typ: "int"}, nil
		// return &kvt{val: fmt.Sprintf("%d", vv), typ: fmt.Sprintf("%T", v)}, nil
	case float32, float64:
		return &kvt{val: fmt.Sprintf("%f", vv), typ: "float"}, nil
		//return &kvt{val: fmt.Sprintf("%f", vv), typ: fmt.Sprintf("%T", v)}, nil
	case nil:
		return &kvt{val: "null", typ: "null"}, nil
	default:
		return nil, errors.E("flatten", errors.K.Invalid, "type", fmt.Sprintf("%T", v))
	}
}

func (f *flatten) createKey(parent string, name string) string {
	enc := f.encoder.Replace(name)
	if parent == "/" {
		return "/" + enc
	}
	return parent + f.separator + enc
}

func (f *flatten) createNumKey(parent string, index int) string {
	// return fmt.Sprintf("%s[%d]", parent, index)
	return f.createKey(parent, strconv.FormatInt(int64(index), 10))
}

type kvt struct {
	key string
	val string
	typ string
}
