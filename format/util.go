package format

import (
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/token"
	"github.com/eluv-io/common-go/format/types"
)

// ExtractQID tries to extract a content ID from the given content hash, id or write token string. Returns nil if no
// content ID is found.
func ExtractQID(qhit string) types.QID {
	qid, err := id.Q.FromString(qhit)
	if err == nil {
		return qid
	}

	hsh, err := hash.Q.FromString(qhit)
	if err == nil {
		return hsh.ID
	}

	qwt, err := token.QWrite.FromString(qhit)
	if err == nil {
		return qwt.QID
	}
	return nil
}
