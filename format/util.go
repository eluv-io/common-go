package format

import (
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/token"
	"github.com/eluv-io/common-go/format/types"
)

// ExtractQID tries to extract a content ID from the given content hash, id or write token string. Returns nil if no
// content ID is found.
func ExtractQID(qihot string) types.QID {
	qid, err := id.Q.FromString(qihot)
	if err == nil {
		return qid
	}

	hsh, err := hash.Q.FromString(qihot)
	if err == nil {
		return hsh.ID
	}

	qwt, err := token.QWrite.FromString(qihot)
	if err == nil {
		return qwt.QID
	}
	return nil
}

// ParseQhot parses the given string as a content hash or write token. Returns
//   - (id, hash, nil) if the string is a content hash
//   - (id, nil, token) if it is a write token
//   - (nil, nil, nil) if it is neither.
func ParseQhot(qhot string) (types.QID, types.QHash, types.QWriteToken) {
	hsh, err := hash.Q.FromString(qhot)
	if err == nil {
		return hsh.ID, hsh, nil
	}

	qwt, err := token.QWrite.FromString(qhot)
	if err == nil {
		return qwt.QID, nil, qwt
	}

	return nil, nil, nil
}

// ParseQihot parses the given string as a content hash or write token. Returns
//   - (id, nil, nil) if the string is a content id
//   - (id, hash, nil) if the string is a content hash
//   - (id, nil, token) if the string is a write token
//   - (nil, nil, nil) if the string is neither
func ParseQihot(qihot string) (types.QID, types.QHash, types.QWriteToken) {
	qid, err := id.Q.FromString(qihot)
	if err == nil {
		return qid, nil, nil
	}

	hsh, err := hash.Q.FromString(qihot)
	if err == nil {
		return hsh.ID, hsh, nil
	}

	qwt, err := token.QWrite.FromString(qihot)
	if err == nil {
		return qwt.QID, nil, qwt
	}

	return nil, nil, nil
}
