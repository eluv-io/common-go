package structured

// Set inserts or replaces the element of the target data structure at the given
// path with the provided data. Any path elements that do not exist are created
// as objects (maps). If the data is nil, the element at path is removed.
// However, any intermediate non-existing path elements are still created - if
// that is not desired, use Delete(target, path).
// Returns the modified structure, or an error.
func Set(target interface{}, path Path, data interface{}) (interface{}, error) {
	sub, err := resolveSub(path, target, true, false)
	if err != nil {
		return nil, err
	}
	sub.Set(dereference(data), false)
	return sub.Root(), nil
}

// Set inserts or replaces the element of the target data structure at the given
// path with the provided data. Any path elements that do not exist are created
// as objects (maps). Unlike Set(), this function will set the element at path
// even if data is nil.
// Returns the modified structure, or an error.
func SetEvenIfNil(target interface{}, path Path, data interface{}) (interface{}, error) {
	sub, err := resolveSub(path, target, true, false)
	if err != nil {
		return nil, err
	}
	sub.Set(dereference(data), true)
	return sub.Root(), nil
}
