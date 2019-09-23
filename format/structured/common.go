package structured

// dereference dereferences pointers to maps or arrays.
func dereference(target interface{}) interface{} {
	switch t := target.(type) {
	case *map[string]interface{}:
		return *t
	case *[]interface{}:
		return *t
	case *interface{}:
		return *t
	}
	return target
}

// address returns the address of the map, array or interface{} represented by
// target.
func address(target interface{}) interface{} {
	switch t := target.(type) {
	case map[string]interface{}:
		return &t
	case []interface{}:
		return &t
	case interface{}:
		return &t
	}
	return target
}
