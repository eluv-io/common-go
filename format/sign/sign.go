package sign

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mr-tron/base58/base58"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
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
	SUNKNOWN        SigCode = iota
	ES256K                  // ECDSA Signature with secp256k1 Curve - https://tools.ietf.org/id/draft-jones-webauthn-secp256k1-00.html
	EIP191Personal          // Ethereum personal sign - https://eips.ethereum.org/EIPS/eip-191
	EIP712TypedData         // Ethereum type data signatures - https://eips.ethereum.org/EIPS/eip-712
)

const sigCodeLen = 1
const sigPrefixLen = 7

var sigCodeToPrefix = map[SigCode]string{}
var sigPrefixToCode = map[string]SigCode{
	"sunk___": SUNKNOWN,
	"ES256K_": ES256K,
	"EIP191P": EIP191Personal,
	"EIP712T": EIP712TypedData,
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

// UnmarshalText implements custom unmarshalling from the string representation.
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

// SignerAddress returns the address that was used to sign the given bytes, yielding this signature. Returns an error if
//   * the signature that doesn't allow signer recovery
//   * or an error during recovery occurs
func (sig *Sig) SignerAddress(signedBytes []byte) (common.Address, error) {
	return sig.SignerAddressFromHash(crypto.Keccak256(signedBytes))
}

// SignerAddressFromHash returns the address that was used to sign the given hashed bytes, yielding this signature.
// Returns an error if * the signature that doesn't allow signer recovery * or an error during recovery occurs
func (sig *Sig) SignerAddressFromHash(hash []byte) (common.Address, error) {
	e := errors.Template("SignerAddress", errors.K.Invalid)

	switch sig.Code() {
	case ES256K, EIP191Personal:
		// continue
	default:
		return common.Address{}, e("reason", "address recovery not available for signature type", "sig_type", sig.Code())
	}

	recoverKMSPKBytes, err := crypto.Ecrecover(hash, EthAdjustBytes(sig.Code(), sig.Bytes()))
	if err != nil {
		return common.Address{}, e(err)
	}
	recoverTrustPK, err := crypto.UnmarshalPubkey(recoverKMSPKBytes)
	if err != nil {
		return common.Address{}, e(err)
	}
	return crypto.PubkeyToAddress(*recoverTrustPK), nil
}

// EthAdjustBytes remains for backward compatibility
// deprecated - use standalone func EthAdjustBytes()
func (sig *Sig) EthAdjustBytes() []byte {
	return EthAdjustBytes(sig.Code(), sig.Bytes())
}

func NewSig(code SigCode, codeBytes []byte) Sig {
	return Sig(append([]byte{byte(code)}, EthAdjustBytes(code, codeBytes)...))
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

// EthAdjustBytes adjusts ECDSA Signatures to be compatible with ETH tools. The signature must conform to the secp256k1
// curve R, S and V values, where the V value must be 27 or 28 for legacy reasons.
//
// Resources:
// * https://bitcoin.stackexchange.com/questions/38351/ecdsa-v-r-s-what-is-v
//
// * From "The Magic of Digital Signatures on Ethereum"
//   https://medium.com/mycrypto/the-magic-of-digital-signatures-on-ethereum-98fe184dc9c7
//
//   The recovery identifier (“v”)
//
//   v is the last byte of the signature, and is either 27 (0x1b) or 28 (0x1c). This identifier is important because
//   since we are working with elliptic curves, multiple points on the curve can be calculated from r and s alone. This
//   would result in two different public keys (thus addresses) that can be recovered. The v simply indicates which one
//   of these points to use.
//
//   In most implementations, the v is just 0 or 1 internally, but 27 was added as arbitrary number for signing Bitcoin
//   messages and Ethereum adapted that as well.
//
//   Since EIP-155, we also use the chain ID to calculate the v value. This prevents replay attacks across different
//   chains: A transaction signed for Ethereum cannot be used for Ethereum Classic, and vice versa. Currently, this is
//   only used for signing transaction however, and is not used for signing messages.
//
func EthAdjustBytes(code SigCode, bts []byte) []byte {
	if len(bts) == 0 {
		return []byte{}
	}
	if (code != ES256K && code != EIP191Personal) || len(bts) <= 64 || bts[64] < 4 {
		return bts
	} else {
		adjSigBytes := make([]byte, 65)
		copy(adjSigBytes, bts)
		adjSigBytes[64] -= 27
		return adjSigBytes
	}
}

// HashEIP191Personal hashes the given message according to EIP-191 personal_sign
// See https://eips.ethereum.org/EIPS/eip-191
func HashEIP191Personal(message []byte) []byte {
	// see github.com/ethereum/go-ethereum@v1.9.11/accounts/accounts.go:193 TextAndHash()
	msg := "Eluvio Content Fabric Access Token 1.0\n" + string(message)
	msg = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	return crypto.Keccak256([]byte(msg))
}

/* PENDING(LUK): Typed Data hashing will look something like this...
// HashTypedData hashes EIP-712 conforming typed data
// hash = keccak256("\x19${byteVersion}${domainSeparator}${hashStruct(message)}")
// Based on github.com/ethereum/go-ethereum@v1.9.11/signer/core/signed_data.go:316 SignTypedData()
func (s *SignatureHelper) HashTypedData(typedData core.TypedData) (hexutil.Bytes, error) {
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, err
	}
	typedDataHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		return nil, err
	}
	rawData := []byte(fmt.Sprintf("\x19\x01%s%s", string(domainSeparator), string(typedDataHash)))
	hsh := crypto.Keccak256(rawData)
	return hsh, nil
}
*/
