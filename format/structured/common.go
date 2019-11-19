package structured

// Transformer is an interface for arbitrary transformations of structured data.
type Transformer interface {
	// Transform transforms the given structured data and returns the
	// transformation result or an error.
	Transform(data interface{}) (transformed interface{}, err error)
}

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
