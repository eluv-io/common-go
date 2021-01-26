package sign

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mr-tron/base58/base58"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/log"
)

// SigCode is the type of an Sig
type SigCode uint8

// FromString parses the given string and returns the Sig. Returns an error
// if the string is not a Sig or an Sig of the wrong type.
func (c SigCode) FromString(s string) (Sig, error) {
	sig, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return sig, sig.AssertCode(c)
}

func (c SigCode) ToString(b []byte) string {
	return NewSig(c, b).String()
}

// lint disable
const (
	SUNKNOWN SigCode = iota
	ES256K           //ECDSA Signature with secp256k1 Curve
)

const sigCodeLen = 1
const sigPrefixLen = 7

var sigCodeToPrefix = map[SigCode]string{}
var sigPrefixToCode = map[string]SigCode{
	"sunk___": SUNKNOWN, // unknown
	"ES256K_": ES256K,   // ECDSA w/ secp256k1 Curve - https://tools.ietf.org/id/draft-jones-webauthn-secp256k1-00.html
}

func init() {
	for prefix, code := range sigPrefixToCode {
		if len(prefix) != sigPrefixLen {
			log.Fatal("invalid Signature prefix definition", "prefix", prefix)
		}
		sigCodeToPrefix[code] = prefix
	}
}

// Sig is the type representing a Signature.
//
// Signature prefixes should follow standard values for the 'alg' claim in JWT.
// see:
//	* JWA: https://tools.ietf.org/html/rfc7518
//	* JWT: https://tools.ietf.org/html/rfc7519
//
// Sigs follow the multiformat principle and are prefixed with their type
// (a varint). Unlike other multiformat implementations like multihash, the type
// is serialized to textual form (String(), JSON) as a short text prefix instead
// of their encoded varint for increased readability.
type Sig []byte

func (sig Sig) String() string {
	if len(sig) <= sigCodeLen {
		return ""
	}
	return sig.prefix() + base58.Encode(sig[sigCodeLen:])
}

// AssertCode checks whether the Sig's SigCode equals the provided SigCode
func (sig Sig) AssertCode(c SigCode) error {
	if sig.Code() != c {
		return errors.E("SIG SigCode check", errors.K.Invalid,
			"expected", sigCodeToPrefix[c],
			"actual", sig.prefix())
	}
	return nil
}

func (sig Sig) prefix() string {
	p, found := sigCodeToPrefix[sig.Code()]
	if !found {
		return sigCodeToPrefix[SUNKNOWN]
	}
	return p
}

func (sig Sig) Code() SigCode {
	return SigCode(sig[0])
}

func (sig Sig) IsNil() bool {
	return sig == nil || len(sig) <= sigCodeLen
}

// MarshalText implements custom marshaling using the string representation.
func (sig Sig) MarshalText() ([]byte, error) {
	return []byte(sig.String()), nil
}

func (sig Sig) Bytes() []byte {
	return sig[1:]
}

// Signatures that are created with ETH-compatible tools insist on adding a constant 27 to the v value (byte ordinal 64)
//  of the signature. I have yet to see a good explanation why this was done except that it is a 'legacy' value.
// We want our signatures to conform to the standard but rather than insist that all libraries our clients (including us)
//  use understand and account for this mismatch, it's saner for us to just adjust where necessary on the server.
func (sig Sig) EthAdjustBytes() []byte {
	if len(sig) == 0 {
		return []byte{}
	}
	if sig.Code() != ES256K || len(sig.Bytes()) <= 64 || sig.Bytes()[64] < 4 {
		return sig.Bytes()
	} else {
		adjSigBytes := make([]byte, 65)
		copy(adjSigBytes, sig.Bytes())
		adjSigBytes[64] -= 27
		return adjSigBytes
	}
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (sig *Sig) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.E("unmarshal Sig", errors.K.Invalid, err)
	}
	*sig = parsed
	return nil
}

func (sig Sig) Is(s string) bool {
	sg, err := FromString(s)
	if err != nil {
		return false
	}
	return bytes.Equal(sig, sg)
}

// SignerAddress returns the address that was used to sign the given bytes,
// yielding this signature.
func (sig *Sig) SignerAddress(signedBytes []byte) (common.Address, error) {
	e := errors.Template("SignerAddress", errors.K.Invalid)
	hash := crypto.Keccak256(signedBytes)
	recoverKMSPKBytes, err := crypto.Ecrecover(hash, sig.EthAdjustBytes())
	if err != nil {
		return common.Address{}, e(err)
	}
	recoverTrustPK, err := crypto.UnmarshalPubkey(recoverKMSPKBytes)
	if err != nil {
		return common.Address{}, e(err)
	}
	return crypto.PubkeyToAddress(*recoverTrustPK), nil
}

func NewSig(code SigCode, codeBytes []byte) Sig {
	return Sig(append([]byte{byte(code)}, codeBytes...))
}

// FromString parses an Sig from the given string representation.
func FromString(s string) (Sig, error) {
	if len(s) <= sigPrefixLen {
		return nil, errors.E("parse Sig", errors.K.Invalid).With("string", s)
	}

	code, found := sigPrefixToCode[s[:sigPrefixLen]]
	if !found {
		return nil, errors.E("parse Sig", errors.K.Invalid, "reason", "unknown prefix", "string", s)
	}

	dec, err := base58.Decode(s[sigPrefixLen:])
	if err != nil {
		return nil, errors.E("parse Sig", errors.K.Invalid, err, "string", s)
	}
	b := []byte{byte(code)}
	return Sig(append(b, dec...)), nil
}
