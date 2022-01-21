package link

import (
	"encoding/base64"

	"github.com/eluv-io/errors-go"

	"github.com/qluvio/content-fabric/format/encryption"
)

// NewBlobLink creates a blob link from the given link.
// A blob link is a relative link with the "blob" selector, a "data" property that
// contains base64-encoded bytes, and an optional "encryption" property as defined
// in encryption.Scheme (defaults to "none"):
//   {
//     "/": "./blob",
//     "data": "Y2xlYXIgZGF0YQ==",
//     "encryption": "cgck"
//   }
//
// Blob links are used to include encrypted data in metadata, and have it
// automatically decrypted by the fabric node in the same way as encrypted parts.
// See LinkReader.OpenLink() in eluvio/qfab/daemon/simple/simple.go for details.
func NewBlobLink(l *Link) (*BlobLink, error) {
	e := errors.Template("blob link", errors.K.Invalid)
	var err error

	if l.Selector != S.Blob {
		return nil, e("reason", "not a blob", "selector", l.Selector)
	}

	var blob = &BlobLink{}

	switch t := l.Props["data"].(type) {
	case string:
		blob.Data, err = base64.StdEncoding.DecodeString(t)
		if err != nil {
			return nil, e(err, "reason", "invalid base64 encoding of data")
		}
	case []byte:
		blob.Data = t
	default:
		return nil, e("reason", "invalid data type")
	}

	switch t := l.Props["encryption"].(type) {
	case string:
		blob.EncryptionScheme, err = encryption.FromString(t)
		if err != nil {
			return nil, e(err)
		}
	case encryption.Scheme:
		blob.EncryptionScheme = t
	case nil:
		blob.EncryptionScheme = encryption.None
	default:
		return nil, e("reason", "invalid encryption scheme", "encryption", t)
	}

	if blob.EncryptionScheme == encryption.UNKNOWN {
		return nil, e("reason", "invalid encryption scheme", "encryption", encryption.UNKNOWN.String())
	}
	return blob, nil
}

type BlobLink struct {
	l                *Link
	Data             []byte
	EncryptionScheme encryption.Scheme
}
