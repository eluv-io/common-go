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

// Code is the type of a Sig
type Code uint8

// SigCode is the legacy alias of Code
// @deprecated use Code instead
type SigCode = Code

func (c Code) String() string {
	return codeToPrefix[c]
}

// FromString parses the given string and returns the Sig. Returns an error
// if the string is not a Sig or an Sig of the wrong type.
func (c Code) FromString(s string) (Sig, error) {
	sig, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return sig, sig.AssertCode(c)
}

func (c Code) ToString(b []byte) string {
	return NewSig(c, b).String()
}

// SigLen returns the expected length of a signature for the given code. Returns -1 if unknown or not constant.
func (c Code) SigLen() int {
	switch c {
	case ES256K:
		return 64
	case EIP191Personal:
		return 64
	case EIP712TypedData:
		return 64
	case SR25519:
		// see github.com/!chain!safe/go-schnorrkel@v1.0.0/sign.go:11
		// 	// SignatureSize is the length in bytes of a signature
		//	const SignatureSize = 64
		return 64
	case ED25519:
		// see sgo@1.18/1.18.6/libexec/src/crypto/ed25519/ed25519.go:32
		// 	// SignatureSize is the size, in bytes, of signatures generated and verified by this package.
		//	SignatureSize = 64
		return 64
	default:
		return -1
	}
}

// lint disable
const (
	UNKNOWN Code = iota

	// ES256K is a standard Ethereum ECDSA Signature with secp256k1 curve and SHA-256 hash function
	// See:
	//  - https://datatracker.ietf.org/doc/rfc8812/
	ES256K

	// EIP191Personal is an Ethereum "personal sign" signature
	// See:
	//  - https://eips.ethereum.org/EIPS/eip-191
	EIP191Personal

	// EIP712TypedData is an Ethereum "typed data" signature
	// See:
	//  - https://eips.ethereum.org/EIPS/eip-712
	EIP712TypedData

	// SR25519 is a Schnorr signature on Ristretto compressed Ed25519 points.
	//
	// Schnorr signatures bring some noticeable features over the ECDSA/EdDSA schemes:
	//  - better for hierarchical deterministic key derivations.
	//  - allow for native multi-signature through signature aggregation
	//  - generally more resistant to misuse.
	// From https://doc.deepernetwork.org/v3/advanced/cryptography/#sr25519
	//
	// See also:
	//  - https://github.com/w3f/schnorrkel
	//  - https://en.wikipedia.org/wiki/Schnorr_signature,
	//  - https://wiki.polkadot.network/docs/learn-cryptography#keypairs-and-signing
	SR25519

	// ED25519 is the Edwards-curve Digital Signature Algorithm (EdDSA) with SHA256 on curve 25519
	// For details see:
	//	- https://en.wikipedia.org/wiki/EdDSA#Ed25519
	//  - https://www.rfc-editor.org/rfc/rfc8032
	ED25519

	// SUNKNOWN is the legacy alias for UNKNOWN
	// @deprecated use UNKNOWN instead
	SUNKNOWN = UNKNOWN
)

const codeLen = 1
const prefixLen = 7

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"sunk___": UNKNOWN,
	"ES256K_": ES256K,
	"EIP191P": EIP191Personal,
	"EIP712T": EIP712TypedData,
	"SR25519": SR25519,
	"ED25519": ED25519,
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid Signature prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
}

// Sig is the type representing a Signature.
//
// Signature prefixes should follow standard values for the 'alg' claim in JWT.
// see:
//   - JWA: https://tools.ietf.org/html/rfc7518
//   - JWT: https://tools.ietf.org/html/rfc7519
//
// Sigs follow the multiformat principle and are prefixed with their type
// (a varint). Unlike other multiformat implementations like multihash, the type
// is serialized to textual form (String(), JSON) as a short text prefix instead
// of their encoded varint for increased readability.
type Sig []byte

func (sig Sig) String() string {
	if len(sig) <= codeLen {
		return ""
	}
	return sig.prefix() + base58.Encode(sig[codeLen:])
}

// AssertCode checks whether the Sig's Code equals the provided Code
func (sig Sig) AssertCode(c Code) error {
	if sig.Code() != c {
		return errors.E("SIG Code check", errors.K.Invalid,
			"expected", codeToPrefix[c],
			"actual", sig.prefix())
	}
	return nil
}

func (sig Sig) prefix() string {
	p, found := codeToPrefix[sig.Code()]
	if !found {
		return codeToPrefix[UNKNOWN]
	}
	return p
}

func (sig Sig) Code() Code {
	if len(sig) == 0 {
		return UNKNOWN
	}
	return Code(sig[0])
}

func (sig Sig) IsNil() bool {
	return sig == nil || len(sig) <= codeLen
}

func (sig Sig) IsValid() bool {
	return len(sig) > codeLen
}

// MarshalText implements custom marshaling using the string representation.
func (sig Sig) MarshalText() ([]byte, error) {
	return []byte(sig.String()), nil
}

func (sig Sig) Bytes() []byte {
	if len(sig) == 0 {
		return nil
	}
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
//   - the signature that doesn't allow signer recovery
//   - or an error during recovery occurs
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

func NewSig(code Code, codeBytes []byte) Sig {
	return append([]byte{byte(code)}, EthAdjustBytes(code, codeBytes)...)
}

// FromString parses a Sig from the given string representation.
func FromString(s string) (Sig, error) {
	if len(s) <= prefixLen {
		return nil, errors.E("parse Sig", errors.K.Invalid).With("string", s)
	}

	code, found := prefixToCode[s[:prefixLen]]
	if !found {
		return nil, errors.E("parse Sig", errors.K.Invalid, "reason", "unknown prefix", "string", s)
	}

	dec, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, errors.E("parse Sig", errors.K.Invalid, err, "string", s)
	}
	b := []byte{byte(code)}
	return append(b, dec...), nil
}

// EthAdjustBytes adjusts ECDSA Signatures to be compatible with ETH tools. The signature must conform to the secp256k1
// curve R, S and V values, where the V value must be 27 or 28 for legacy reasons.
//
// Resources:
//
//   - https://bitcoin.stackexchange.com/questions/38351/ecdsa-v-r-s-what-is-v
//
//   - From "The Magic of Digital Signatures on Ethereum"
//     https://medium.com/mycrypto/the-magic-of-digital-signatures-on-ethereum-98fe184dc9c7
//
//     The recovery identifier (“v”)
//
//     v is the last byte of the signature, and is either 27 (0x1b) or 28 (0x1c). This identifier is important because
//     since we are working with elliptic curves, multiple points on the curve can be calculated from r and s alone. This
//     would result in two different public keys (thus addresses) that can be recovered. The v simply indicates which one
//     of these points to use.
//
//     In most implementations, the v is just 0 or 1 internally, but 27 was added as arbitrary number for signing Bitcoin
//     messages and Ethereum adapted that as well.
//
//     Since EIP-155, we also use the chain ID to calculate the v value. This prevents replay attacks across different
//     chains: A transaction signed for Ethereum cannot be used for Ethereum Classic, and vice versa. Currently, this is
//     only used for signing transaction however, and is not used for signing messages.
func EthAdjustBytes(code Code, bts []byte) []byte {
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
	return crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), string(message))))
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
