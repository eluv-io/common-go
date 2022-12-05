package id

// Extract parses each of the provided strings as an ID. Empty strings are ignored, parsing errors are returned. The
// first ID that is compatible with the provided target code is returned. If an ID has an embedded ID that matches, it
// is returned.
func Extract(target Code, ids ...string) (ID, error) {
	for _, candidate := range ids {
		if candidate != "" {
			cid, err := Parse(candidate)
			if err != nil {
				return nil, err
			}
			if cid.Code().IsCompatible(target) {
				return cid, nil
			}
			cid = Decompose(cid).Embedded()
			if cid.Code().IsCompatible(target) {
				return cid, nil
			}
		}
	}
	return nil, nil
}
