package structured

// CopyFn is a custom a function that allows implementing custom copy behavior
// for specific values by returning true and the custom copy. Returning false
// will trigger the default copy behavior.
type CopyFn func(val interface{}) (override bool, newVal interface{})

// Copy creates a copy of the target data structure. The different data types
// are handled as follows:
//
//   - map[string]interface{} are duplicated
//   - []interface{} are duplicated
//   - other types of maps & slices, pointers, channels and func
//     etc.) are "copied by reference"
//   - simple types and structs are copied by value
//
// The optional custom copy function is called for each element in the copied
// structure and may override the default data type handling by returning true
// and a value that will be used in the copied structure instead.
func Copy(src interface{}, customCopyFn ...CopyFn) interface{} {
	if len(customCopyFn) > 0 && customCopyFn[0] != nil {
		if override, val := customCopyFn[0](src); override {
			return val
		}
	}

	switch t := src.(type) {
	case map[string]interface{}:
		if t == nil {
			return src
		}
		cpy := make(map[string]interface{}, len(t))
		for k, v := range t {
			cpy[k] = Copy(v, customCopyFn...)
		}
		return cpy
	case []interface{}:
		if t == nil {
			return src
		}
		cpy := make([]interface{}, len(t))
		for idx, val := range t {
			cpy[idx] = Copy(val, customCopyFn...)
		}
		return cpy
	default:
		return src
	}
}
