package structured

// Delete removes the element of the target data structure at the given
// path and returns the potentially modified structure and a bool indicating
// whether the structure was modified. Returns the structure unchanged if
// the path does not exist.
func Delete(target interface{}, path Path) (interface{}, bool) {
	// NOTE: before this function was added, delete was simulated with a
	// Set(target, path, nil). That is incorrect, however, if the given path
	// consists of multiple path segments that do not exist. That's because
	// Set() will actually create the inexistent path except for the last
	// segment (the last segment is omitted since setting its value to nil
	// corresponds to removing that segment).
	// So Set(nil, Path{"path", "to", "element"}, nil) will result in the following
	// structure:
	// {
	//   "path": {
	//     "to": {}
	//   }
	// }

	if dereference(target) == nil {
		// cannot delete anything from nil...
		return nil, false
	}
	sub, err := resolveSub(path, target, false)
	if err != nil {
		// the path does not exist, so there is nothing to delete...
		return target, false
	}
	sub.Set(nil, false)
	return sub.Root(), true
}
