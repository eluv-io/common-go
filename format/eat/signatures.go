package eat

import "github.com/eluv-io/errors-go"

// SigTypes defines the different signature types of auth tokens
const SigTypes enumSigType = 0

type tokenSigType struct {
	Prefix string
	Name   string
}

func (s *tokenSigType) String() string {
	return s.Name
}

func (s *tokenSigType) Validate() error {
	e := errors.Template("validate token signature type", errors.K.Invalid)
	if s == nil {
		return e("reason", "signature type is nil")
	}
	if s == SigTypes.Unknown() {
		return e("sig_type", s.Name)
	}
	return nil
}

var allSignatures = []*tokenSigType{
	{"_", "unknown"},
	{"u", "unsigned"},
	{"s", "ES256K"},
}

type enumSigType int

func (enumSigType) Unknown() *tokenSigType  { return allSignatures[0] }
func (enumSigType) Unsigned() *tokenSigType { return allSignatures[1] }
func (enumSigType) ES256K() *tokenSigType   { return allSignatures[2] }

var prefixToSignature = map[string]*tokenSigType{}

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
