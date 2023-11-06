package id

import (
	"strings"

	"github.com/eluv-io/errors-go"
)

// Extract parses each of the provided strings as an ID. Empty strings are ignored, parsing errors are returned.
// The first ID that is compatible with the provided target code and is valid is returned. If an ID has an embedded
// valid ID that matches, it is returned.
// If no valid ID is found, an error is returned.
func Extract(target Code, ids ...string) (ID, error) {
	for _, candidate := range ids {
		if candidate != "" {
			cid, err := Parse(candidate)
			if err != nil {
				return nil, err
			}
			if cid.Code().IsCompatible(target) && cid.IsValid() {
				return cid, nil
			}
			cid = Decompose(cid).Embedded()
			if cid.Code().IsCompatible(target) && cid.IsValid() {
				return cid, nil
			}
		}
	}
	return nil, errors.E("Extract", errors.K.Invalid,
		"reason", "no valid id found",
		"ids", strings.Join(ids, ","))
}
