package sign

import (
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/keys"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/errors-go"
)

func NewSignature(format SerializationFormat, id types.UserID, publicKey keys.KID, sig Sig) Signature {
	return Signature{
		SerializationFormat: format,
		ID:                  id,
		PublicKey:           publicKey,
		Sig:                 sig,
	}
}

// Signature is a wrapper of sign.Sig that bundles the ID and public key of the signer that produced the signature.
type Signature struct {
	SerializationFormat SerializationFormat `json:"serialization_format"`
	ID                  id.ID               `json:"id,omitempty"`
	PublicKey           keys.KID            `json:"public_key"`
	Sig                 Sig                 `json:"sig"`
}

func (s *Signature) Validate() error {
	e := errors.Template("Signature.Validate", "signature", s)
	if s == nil {
		return e("reason", "signature is nil")
	}
	if err := s.SerializationFormat.Validate(); err != nil {
		return e(err)
	}
	if !s.ID.IsValid() {
		return e("reason", "invalid ID")
	}
	if !s.PublicKey.IsValid() {
		return e("reason", "invalid public key")
	}
	if !s.Sig.IsValid() {
		return e("reason", "invalid sig")
	}
	return nil
}
