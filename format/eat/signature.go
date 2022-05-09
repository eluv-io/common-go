package eat

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/eluv-io/common-go/format/sign"
	"github.com/eluv-io/errors-go"
)

// CalcECDSA is a function that calculates the ECDSA signature for the given bytes (usually a hash of the data that
// needs signing).
type CalcECDSA func(hash []byte) (sig []byte, err error)

// signToken signs the given token with the provided signing function and according to the given signature type. The
// signAddr must correspond to the private key that is used in the signing function.
func signToken(
	token *Token,
	signAddr common.Address,
	signFunc CalcECDSA,
	sigType *TokenSigType) (err error) {

	e := errors.Template("signToken", errors.K.Invalid)
	if token == nil {
		return e("reason", "token is nil")
	}
	if !sigType.HasSig() {
		return e("reason", "invalid signature type", "sig_type", token.SigType)
	}
	if !token.Signature.IsNil() {
		return e("reason", "token already signed", "signature", token.Signature)
	}
	token.clearCaches()
	helper := SignatureHelper{
		token: token,
	}
	return helper.sign(signAddr, signFunc, sigType)
}

func VerifySignatureFrom(token *Token, trusted common.Address) error {
	helper, err := newSignatureHelper(token)
	if err == nil {
		err = helper.VerifySignatureFrom(trusted)
	}
	return err
}

func VerifySignature(token *Token) error {
	helper, err := newSignatureHelper(token)
	if err == nil {
		err = helper.VerifySignature()
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// newSignatureHelper creates a helper for creating & verifying signatures.
func newSignatureHelper(token *Token) (*SignatureHelper, error) {
	e := errors.Template("verify token", errors.K.Permission)
	if token == nil {
		return nil, e("reason", "token is nil")
	}
	if !token.SigType.HasSig() {
		return nil, e("reason", "invalid signature type", "sig_type", token.SigType)
	}
	if token.Signature.IsNil() {
		return nil, e("reason", "missing signature")
	}
	return &SignatureHelper{token: token}, nil
}

type SignatureHelper struct {
	token *Token
	base  baseFns
}

func (s *SignatureHelper) VerifySignature() error {
	return s.VerifySignatureFrom(s.token.EthAddr)
}

func (s *SignatureHelper) VerifySignatureFrom(trusted common.Address) error {
	e := errors.Template("verify token signature", errors.K.Permission)
	signerAddress, err := s.signerAddress()
	if err != nil {
		return e(err)
	}

	// verify that auth data is from trusted address
	if !bytes.Equal(trusted.Bytes(), signerAddress.Bytes()) {
		return e("reason", "invalid trust address or auth token tampered with",
			"expect_address", trusted.String(),
			"signer_address", signerAddress.String())
	}

	return nil
}

// sign signs the helper's token using the provided signing function, producing a signature of the given type.
func (s *SignatureHelper) sign(
	signAddr common.Address,
	signFunc CalcECDSA,
	sigType *TokenSigType) (err error) {

	e := errors.Template("signWith", errors.K.Invalid, "type", sigType)

	s.token.EthAddr = signAddr

	hsh, err := s.hashToken(sigType)
	if err != nil {
		return e(err)
	}

	sig, err := signFunc(hsh)
	if err != nil {
		return e(err)
	}
	if len(sig) != 65 {
		return e("reason", "signature must be 65 bytes long",
			"len", len(sig))
	}

	s.token.Signature = sign.NewSig(sigType.Code, s.base.ethAdjustBytes(sig))
	s.token.SigType = sigType
	return nil
}

func (s *SignatureHelper) hashToken(sigType *TokenSigType) (hsh []byte, err error) {
	var encoded []byte
	switch sigType {
	case SigTypes.ES256K():
		encoded, err = s.token.getPayload()
		if err == nil {
			hsh = crypto.Keccak256(encoded)
		}
	case SigTypes.EIP191Personal():
		encoded, err = s.token.getUncompressedTokenData()
		if err == nil {
			// see github.com/ethereum/go-ethereum@v1.9.11/accounts/accounts.go:193 TextAndHash()
			msg := "Eluvio Content Fabric Access Token 1.0\n" + string(encoded)
			msg = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
			hsh = crypto.Keccak256([]byte(msg))
		}
	}

	if err != nil {
		return nil, errors.E("hashToken", err)
	}

	return hsh, nil
}

/*
PENDING(LUK): Typed Data hashing will look something like this...

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

func (s *SignatureHelper) signerAddress() (addr common.Address, err error) {
	e := errors.Template("signerAddress")

	var hsh []byte
	hsh, err = s.hashToken(s.token.SigType)
	if err != nil {
		return addr, e(err)
	}

	addr, err = s.base.signerAddress(s.token.Signature, hsh)
	return addr, e.IfNotNil(err)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type baseFns struct{}

// signerAddress recovers the public address belonging to the private key that created the given sig, based on the hash
// that was signed.
func (b baseFns) signerAddress(sig sign.Sig, hash []byte) (common.Address, error) {
	e := errors.Template("SignerAddress", errors.K.Invalid)
	recoverKMSPKBytes, err := crypto.Ecrecover(hash, b.ethAdjustBytes(sig.Bytes()))
	if err != nil {
		return common.Address{}, e(err)
	}
	recoverTrustPK, err := crypto.UnmarshalPubkey(recoverKMSPKBytes)
	if err != nil {
		return common.Address{}, e(err)
	}
	return crypto.PubkeyToAddress(*recoverTrustPK), nil
}

// EthAdjustBytes adjusts ECDSA Signatures to be compatible with ETH tools.
//
// PaulO: ETH-compatible tools insist on adding a constant 27 to the v value (byte ordinal 64) of the signature. I have
// yet to see a good explanation why this was done except that it is a 'legacy' value. We want our signatures to conform
// to the standard but rather than insist that all libraries our clients (including us) use understand and account for
// this mismatch, it's saner for us to just adjust where necessary on the server.
func (baseFns) ethAdjustBytes(sig []byte) []byte {
	if len(sig) == 0 {
		return []byte{}
	}
	bts := sig
	if len(bts) <= 64 || bts[64] < 4 {
		return bts
	} else {
		adjSigBytes := make([]byte, 65)
		copy(adjSigBytes, bts)
		adjSigBytes[64] -= 27
		return adjSigBytes
	}
}
