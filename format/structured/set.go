package structured

// Set inserts or replaces the element of the target data structure at the given
// path with the provided data. Any path elements that do not exist are created
// as objects (maps).
// Returns the modified structure, or an error.
func Set(target interface{}, path Path, data interface{}) (interface{}, error) {
	sub, err := resolveSub(path, target, true)
	if err != nil {
		return nil, err
	}
	sub.Set(dereference(data))
	return sub.Root(), nil
}
