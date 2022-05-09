package eat

import (
	"github.com/eluv-io/common-go/format/sign"
	"github.com/eluv-io/errors-go"
)

// SigTypes defines the different signature types of auth tokens
const SigTypes enumSigType = 0

type TokenSigType struct {
	Prefix string
	Name   string
	Code   sign.SigCode
}

func (s *TokenSigType) String() string {
	return s.Name
}

func (s *TokenSigType) HasSig() bool {
	switch s {
	case nil, SigTypes.Unsigned():
		return false
	}
	return true
}

func (s *TokenSigType) Validate() error {
	e := errors.Template("validate token signature type", errors.K.Invalid)
	if s == nil {
		return e("reason", "signature type is nil")
	}
	if s == SigTypes.Unknown() {
		return e("sig_type", s.Name)
	}
	return nil
}

var allSignatures = []*TokenSigType{
	{"_", "unknown", sign.SUNKNOWN},
	{"u", "unsigned", sign.SUNKNOWN},
	{"s", "ES256K", sign.ES256K},
	{"p", "EIP191Personal", sign.EIP191Personal},   // https://eips.ethereum.org/EIPS/eip-191
	{"t", "EIP712TypedData", sign.EIP712TypedData}, // https://eips.ethereum.org/EIPS/eip-712
}

type enumSigType int

func (enumSigType) Unknown() *TokenSigType         { return allSignatures[0] }
func (enumSigType) Unsigned() *TokenSigType        { return allSignatures[1] }
func (enumSigType) ES256K() *TokenSigType          { return allSignatures[2] }
func (enumSigType) EIP191Personal() *TokenSigType  { return allSignatures[3] }
func (enumSigType) EIP712TypedData() *TokenSigType { return allSignatures[4] }

var prefixToSignature = map[string]*TokenSigType{}

func init() {
	for _, s := range allSignatures {
		prefixToSignature[s.Prefix] = s
		if len(s.Prefix) != 1 {
			panic(errors.E("invalid signature prefix definition",
				"signature", s.Name,
				"prefix", s.Prefix))
		}
	}
}
