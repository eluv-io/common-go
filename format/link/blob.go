package link

import (
	"encoding/base64"

	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/errors-go"
)

// Blob represents the specific data of a blob link. A blob link is a relative link with the "blob" selector, a "data"
// property that contains base64-encoded bytes, and an optional "encryption" property as defined in encryption.Scheme
// (defaults to "none"):
//
//	{
//	  "/": "./blob",
//	  "data": "Y2xlYXIgZGF0YQ==",
//	  "encryption": "cgck"
//	}
//
// Blob links are used to include encrypted data in metadata, and have it automatically decrypted by the fabric node in
// the same way as encrypted parts. See LinkReader.OpenLink() in eluvio/qfab/daemon/simple/simple.go for details.
type Blob struct {
	// NOTE: DO NOT CHANGE FIELD TYPES, THEIR ORDER OR REMOVE ANY FIELDS SINCE STRUCT IS CBOR-ENCODED AS ARRAY!
	_struct          bool              `cbor:",toarray"`             // encode struct as array
	Data             []byte            `json:"data"`                 // data encrypted according to encryption scheme
	EncryptionScheme encryption.Scheme `json:"encryption,omitempty"` // the encryption scheme of the data
	KID              string            `json:"kid,omitempty"`        // key ID
}

func (b *Blob) UnmarshalValue(val *structured.Value) error {
	// data, err := base64.StdEncoding.DecodeString(val.Get("data").String())
	// if err == nil {
	// 	_ = val.Set(structured.Path{"data"}, data)
	b.EncryptionScheme = encryption.None
	err := val.Decode(b)
	// }
	if err != nil {
		return errors.NoTrace("blob.unmarshalValue", err)
	}
	return nil
}

func (b *Blob) UnmarshalValueAndRemove(val *structured.Value) error {
	err := b.UnmarshalValue(val)
	if err != nil {
		return err
	}
	val.Delete("data")
	val.Delete("encryption")
	val.Delete("kid")
	return nil
}

func (b *Blob) MarshalMap() map[string]interface{} {
	m := map[string]interface{}{}
	m["data"] = base64.StdEncoding.EncodeToString(b.Data)
	if b.EncryptionScheme != encryption.None {
		m["encryption"] = b.EncryptionScheme.String()
	}
	if b.KID != "" {
		m["kid"] = b.KID
	}
	return m
}

func (b *Blob) Validate() error {
	e := errors.Template("validate blob", errors.K.Invalid)
	if b == nil {
		return e("reason", "no blob struct")
	}
	if b.Data == nil {
		return e("reason", "no blob data")
	}
	if b.EncryptionScheme == encryption.UNKNOWN {
		return e("reason", "encryption scheme unknown")
	}
	return nil
}