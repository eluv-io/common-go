package eat

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/eluv-io/common-go/format/sign"
	"github.com/eluv-io/errors-go"
)

// CalcECDSA is a function that calculates the ECDSA signature for the given bytes (usually a hash of the data that
// needs signing).
type CalcECDSA func(hash []byte) (sig []byte, err error)

// VerifySignature validates the token and verifies its signature (if any).
func (t *Token) VerifySignature() error {
	// Validate() calls verifySignature() if required by the token/signature type
	err := t.Validate()
	if err != nil {
		return errors.E("verify token", errors.K.Permission, err)
	}
	return nil
}

// VerifySignatureFrom verifies that the token was signed by the private key belonging to the given ethereum address.
// Returns nil if the signature matches, an error otherwise.
func (t *Token) VerifySignatureFrom(trusted common.Address) error {
	e := errors.Template("verify token signature", errors.K.Permission)
	signerAddress, err := t.SignerAddress()
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

// VerifySignature verifies that the token was signed by the private key belonging to the the token's EthAddr. Returns
// nil if the signature matches, an error otherwise.
func (t *Token) verifySignature() error {
	return t.VerifySignatureFrom(t.EthAddr)
}

// SignerAddress returns the address of a token's signer. Returns an error if
//   * the token is not signed
//   * signed with a signature that doesn't allow signer recovery
//   * or an error during recovery occurs
func (t *Token) SignerAddress() (addr common.Address, err error) {
	e := errors.Template("signerAddress")

	switch t.SigType {
	case SigTypes.ES256K(), SigTypes.EIP191Personal():
		// continue
	default:
		return zeroAddr, errors.E("signerAddress", "reason", "token is not signed")
	}
	var hsh []byte
	hsh, err = t.hashToken(t.SigType)
	if err != nil {
		return addr, e(err)
	}

	addr, err = t.Signature.SignerAddressFromHash(hsh)
	return addr, e.IfNotNil(err)
}

// sign signs the token with the provided signing function and according to the given signature type. The signAddr
// must correspond to the private key that is used in the signing function.
func (t *Token) sign(
	signAddr common.Address,
	signFunc CalcECDSA,
	sigType *TokenSigType) (err error) {

	e := errors.Template("sign", errors.K.Invalid)
	if t == nil {
		return e("reason", "token is nil")
	}

	e = e.Add("sig_type", sigType)
	if !sigType.HasSig() {
		return e("reason", "invalid signature type")
	}

	t.clearCaches()
	t.EthAddr = signAddr

	hsh, err := t.hashToken(sigType)
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

	t.Signature = sign.NewSig(sigType.Code, sig)
	t.SigType = sigType
	return nil
}

func (t *Token) hashToken(sigType *TokenSigType) (hsh []byte, err error) {
	var encoded []byte
	switch sigType {
	case SigTypes.ES256K():
		encoded, err = t.getPayload()
		if err == nil {
			hsh = crypto.Keccak256(encoded)
		}
	case SigTypes.EIP191Personal():
		encoded, err = t.getUncompressedTokenData()
		if err == nil {
			hsh = sign.HashEIP191Personal(encoded)
		}
	default:
		return nil, errors.E("hashToken", "reason", "invalid signature type", "sig_type", t.SigType)
	}

	if err != nil {
		return nil, errors.E("hashToken", err)
	}

	return hsh, nil
}
