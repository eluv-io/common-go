package eat

import (
	"bytes"
	"compress/flate"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mattn/go-runewidth"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-varint"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/sign"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/stringutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

const prefixLen = 6 // length of entire prefix including type, sig-type and format

// Token is an auth token, defined by it's type, format, and token data.
type Token struct {
	Type    TokenType
	Format  *tokenFormat
	SigType *TokenSigType
	TokenData
	Signature sign.Sig

	// the fully encoded token, including signature
	//
	// This is set only once, either during decoding, or during creation/signing.
	encoded string // cache for the token's encoded string form
	// The encoded & optionally compressed json/cbor token data. If there is an embedded token, it also includes its
	// payload (i.e. including the embedded token's signature). This byte slice is used to calculate the token signature
	// with ES256K.
	//
	// This is set only once, either during decoding, or during signing.
	payload []byte
	// The marshalled, but non-compressed json/cbor token data. This byte slice is used to calculate the token signature
	// with EIP191Personal.
	//
	// This is set only once, either during decoding, or during signing.
	uncompressedTokenData []byte

	Embedded       *Token
	EmbeddedLength int // the length of the embedded token within the payload

	encDetails encodingDetails // details collected during encoding used in Explain()
}

type encodingDetails struct {
	full                     int // length of full encoded token including signature
	encLegacyBody            int // length of encoded legacy body
	decLegacyBody            int // length of decoded legacy body
	encLegacySignature       int // length of encoded signature (for legacy tokens only - regular tokens have sig in body)
	decLegacySignature       int // length of decoded signature
	uncompressedTokenDataLen int // length of encoded, uncompressed token data
	compressedTokenDataLen   int // length of encoded, compressed token data
	payloadLen               int // length of payload: compressed token data + optional embedded token
	embeddedLength           int // length of embedded token's length varint
	embeddedPrefix           int // length of embedded token's prefix
	embeddedBody             int // length of embedded token's body
}

// New creates a new auth token with the given type, digest, size, and ID
func New(typ TokenType, format *tokenFormat, sig *TokenSigType) *Token {
	return &Token{
		Type:    typ,
		Format:  format,
		SigType: sig,
		TokenData: TokenData{
			Ctx: map[string]interface{}{},
		},
	}
}

// NewClientToken creates a new client token with Type = Types.Client() and
// embeds the provided token. The client token inherits the format of the server
// token.
func NewClientToken(embed *Token) (*Token, error) {
	e := errors.Template("new client token")
	if embed == nil {
		return nil, e(errors.K.Invalid, "reason", "embedded token is nil")
	}

	return &Token{
		Type:     Types.Client(),
		Format:   embed.Format,
		SigType:  SigTypes.Unsigned(),
		Embedded: embed,
		TokenData: TokenData{
			SID: embed.SID,
			LID: embed.LID,
		},
	}, nil
}

// FromString is an alias for Parse.
func FromString(s string) (*Token, error) {
	return Parse(s)
}

// Parse parses the given string as a token.
func Parse(s string) (*Token, error) {
	t, err := parse(s)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// parse parses a token from the given string and returns both the parsed token and any parsing/validation error. In
// case of errors, the token is still returned: it will be invalid and probably only partially decoded.
func parse(s string) (*Token, error) {
	t := New(Types.Unknown(), Formats.Unknown(), SigTypes.Unknown())
	err := t.Decode(s)
	return t, err
}

// MustParse parses the given string as a token. Panics if parsing fails.
func MustParse(s string) *Token {
	t, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return t
}

func (t *Token) String() string {
	if t.encoded == "" {
		var err error
		encoded, err := t.Encode()
		if err != nil {
			return "auth.token.encoding.error"
		}
		return encoded
	}
	return t.encoded
}

// OriginalBearer returns the token string that was originally parsed to create
// this token. Returns an error if the token was not parsed, but constructed.
func (t *Token) OriginalBearer() (string, error) {
	if t.encoded == "" {
		return "", errors.E("original bearer", errors.K.NotExist, "reason", "no original")

	}
	return t.encoded, nil
}

func (t *Token) AsJSON() string {
	bts, _ := json.Marshal(&t.TokenData)
	return string(bts)
}

func (t *Token) AsJSONIndent(prefix, indent string) string {
	bts, _ := json.MarshalIndent(&t.TokenData, prefix, indent)
	return string(bts)
}

func (t *Token) Encode() (s string, err error) {
	if t.IsNil() {
		return "", nil
	}

	e := errors.Template("encode auth token")

	err = t.Validate()
	if err != nil {
		return "", e(err)
	}

	switch t.Format {
	case Formats.Legacy():
		return t.encodeLegacy()
	}

	var data []byte
	data, err = t.encodeTokenAndSigBytes()
	if err != nil {
		return "", err
	}

	s = base58.Encode(data)

	t.encoded = t.encodePrefix() + s

	t.encDetails.full = len(t.encoded)
	return t.encoded, nil
}

// Decode parses the auth token from the given string representation.
func (t *Token) Decode(s string) (err error) {
	if s == "" {
		return nil
	}

	e := errors.Template("decode auth token", errors.K.Invalid, "token_string", s)

	t.encDetails.full = len(s)

	var isLegacy bool
	isLegacy, err = t.decodePrefix(s, true)
	if err != nil {
		return e(err)
	}
	if isLegacy {
		// legacy token was already parsed completely in decodePrefix
		return t.Validate()
	}

	isLegacySigned, err := t.decodeLegacySigned(s)
	if err != nil {
		return e(err)
	}
	if isLegacySigned {
		return t.Validate()
	}

	err = t.decodeString(s[prefixLen:])
	if err != nil {
		return e(err)
	}
	t.encoded = s

	return t.Validate()
}

func (t *Token) decodeLegacySigned(token string) (bool, error) {
	dotIdx := strings.LastIndex(token, ".")
	if dotIdx == -1 {
		return false, nil
	}

	e := errors.Template("decode legacy signed token", errors.K.Invalid.Default())

	// mixed legacy: new token, but client-signed the old-fashion way
	sctString := token[:dotIdx]
	legacySig := token[dotIdx+1:]

	if strings.Contains(sctString, ".") {
		// additional dots in the token, which probably correspond to multiple signatures...
		// refuse multiple embeddings!
		return false, e("reason", "multiple legacy signatures!")
	}

	t.encDetails.encLegacyBody = len(sctString)
	t.encDetails.encLegacySignature = len(legacySig) + 1 // includes the dot "."

	sct, err := Parse(sctString)
	if err != nil {
		return true, e(err)
	}

	t.embedLegacyStateChannelToken(sct)
	t.Format = Formats.LegacySigned()
	t.Type = Types.Client()
	t.payload = []byte(sctString)
	t.encoded = token

	return true, e.IfNotNil(t.decodeLegacySignature(legacySig))
}

func (t *Token) Validate() (err error) {
	e := errors.Template("validate auth token", errors.K.Invalid,
		"type", t.Type,
		"sig_type", t.SigType,
		"format", t.Format,
		"token", stringutil.Stringer(t.AsJSON))

	err = t.Type.Validate()
	if err != nil {
		return e(err)
	}

	err = t.Format.Validate()
	if err != nil {
		return e(err)
	}

	err = t.SigType.Validate()
	if err != nil {
		return e(err)
	}

	validator := tokenValidator{
		token:            t,
		errTemplate:      errors.TemplateNoTrace("validate field", errors.K.Invalid.Default()),
		accumulateErrors: true,
	}

	// signature checks
	clientTokenWithSignature := t.Type == Types.Client() && t.SigType.HasSig() // signature for client tokens is optional
	if t.Type.SignatureRequired || clientTokenWithSignature {
		if !t.SigType.HasSig() {
			validator.error(validator.errTemplate("reason", "invalid signature type"))
		} else if validator.require("signature", t.Signature) {
			// state channel tokens are signed by a "trusted authority" (i.e. KMS), whose address is not stored in
			// the token itself. Hence the signature cannot be checked here and must be checked downstream.
			if t.Type != Types.StateChannel() {
				// legacy tokens may have a missing eth addr, in which case we extract it from the signature itself...
				if !t.HasEthAddr() && (t.Format == Formats.Legacy() || t.Format == Formats.LegacySigned()) {
					t.EthAddr, err = t.SignerAddress()
					validator.error(err)
				}
				if validator.require("adr", t.EthAddr) {
					validator.error(t.verifySignature())
				}
			}
		}
	} else {
		if t.SigType.HasSig() {
			validator.error(validator.errTemplate("reason", "invalid signature type"))
		}
		validator.refuse("signature", t.Signature)
		validator.refuse("address", t.EthAddr)
	}

	validator.require("space id", t.SID)

	// lib seems optional... (because it can be specified in the URL?)
	// valid.require("lib id", t.LID)

	switch t.Type {
	case Types.StateChannel():
		// require
		validator.require("signature", t.SigType.HasSig())
		validator.require("qid", t.QID)
		// don't always require a subject since the actual subject might be 'something else' in the context. Note that
		// here we don't know what could be in the context but (in the future) we might (should ?) require the context
		// to have some known keys since a token should always be granted to someone ...
		if t.Subject == "" && (len(t.Ctx) == 0) {
			validator.require("subject", t.Subject)
		}
		validator.require("grant", t.Grant)
		validator.require("issued at", t.IssuedAt)
		validator.require("expires", t.Expires)
		if t.Expires.Before(t.IssuedAt) {
			validator.errorReason("expires before issued at",
				"expires", t.Expires,
				"issued_at", t.IssuedAt)
		}
		// refuse
		validator.refuse("tx-hash", t.EthTxHash)
		validator.refuse("qp-hash", t.QPHash)
		// additional state channel requirements are checked in auth provider
		return e.IfNotNil(validator.err)

	case Types.EditorSigned():
		// Comment by Gilles from auth-use-new-tokens branch:
		// EditorSigned tokens were originally legacy.ElvClientToken signed
		// by a user/editor of the content. These client tokens were
		// embedding a legacy.ElvAuthToken token where EthAddr was set to
		// the address of the user/editor the token was delivered to.
		// Hence the verification code was checking that the client token was
		// signed by the user whose address was in EthAddr. As a result an
		// editor signed token was signed twice: once as the authority
		// signing the ElvAuthToken and once as the user ElvClientToken.
		// As the 'subject' field did not exist in previous tokens but was
		// computed from the EthAddr field in the server token, this resulted
		// in the subject being also the signer.

		// we now want editor signed tokens able to carry a 'subject' that
		// is not necessarily the signer (use case: water-marking).

		//uid := ethutil.AddressToID(t.EthAddr, id.User)
		//subjid, err := id.User.FromString(t.Subject)
		//if err != nil {
		//	subjid, _ = ethutil.AddrToID(t.Subject, id.User)
		//}
		//if !uid.Equal(subjid) {
		//	return e("reason", "subject differs from signer",
		//		"subject", subjid,
		//		"signer", uid)
		//}

		// require
		validator.require("qid", t.QID)
		validator.require("subject", t.Subject)
		validator.require("grant", t.Grant)
		validator.require("issued at", t.IssuedAt)
		validator.require("expires", t.Expires)
		if t.Expires.Before(t.IssuedAt) {
			validator.errorReason("expires before issued at",
				"expires", t.Expires,
				"issued_at", t.IssuedAt)
		}
		// refuse
		validator.refuse("tx-hash", t.EthTxHash)
		validator.refuse("qp-hash", t.QPHash)
		// additional editor-signed requirements are checked in auth provider
		return e.IfNotNil(validator.err)

	case Types.SignedLink():
		// require
		validator.require("qid", t.QID)
		validator.require("subject", t.Subject)
		validator.require("grant", t.Grant)
		validator.require("issued at", t.IssuedAt)
		// check presence of context values
		ctx := structured.Wrap(t.Ctx).Get("elv")
		validator.require("ctx/elv/lnk", ctx.Get("lnk").Data)
		src := ctx.Get("src").String()
		validator.require("ctx/elv/src", src)
		if src != "" {
			if _, err2 := id.Q.FromString(src); err2 != nil {
				validator.errorReason("ctx/elv/src invalid", err2)
			}
		}

		// refuse
		validator.refuse("tx-hash", t.EthTxHash)
		validator.refuse("qp-hash", t.QPHash)

		// additional signed link requirements are checked in auth provider
		return e.IfNotNil(validator.err)

	case Types.ClientSigned():
		// PENDING(LUK): add additional validations for ClientSigned tokens!
		return e.IfNotNil(validator.err)
	}

	// refused for all other types
	validator.refuse("subject", t.Subject)
	validator.refuse("grant", t.Grant)
	validator.refuse("issued at", t.IssuedAt)
	validator.refuse("expires", t.Expires)
	validator.refuse("ctx", t.Ctx)

	switch t.Type {
	case Types.Client():
		// require
		validator.require("embedded token", t.Embedded)
		// refuse
		validator.refuse("tx-hash", t.EthTxHash)
		validator.refuse("qp-hash", t.QPHash)
		validator.error(t.Embedded.Validate())
		return e.IfNotNil(validator.err)
	case Types.Tx():
		// require
		validator.require("tx-hash", t.EthTxHash)
		// refuse
		validator.refuse("qp-hash", t.QPHash)
		validator.refuse("qid", t.QID)
	case Types.Plain():
		// refuse
		validator.refuse("tx-hash", t.EthTxHash)
		validator.refuse("qp-hash", t.QPHash)
	case Types.Anonymous():
		// refuse
		validator.refuse("signature", t.SigType.HasSig())
		validator.refuse("tx-hash", t.EthTxHash)
		validator.refuse("qp-hash", t.QPHash)
		validator.refuse("address", t.EthAddr)
	case Types.Node():
		// require
		validator.require("qp-hash", t.QPHash)
		// refuse
		validator.refuse("tx-hash", t.EthTxHash)
	}

	return e.IfNotNil(validator.err)
}

func (t *Token) IsNil() bool {
	return t == nil ||
		t.Type == Types.Unknown() ||
		t.Format == Formats.Unknown() ||
		t.SigType == SigTypes.Unknown()
}

// MarshalText converts this auth token to text.
func (t *Token) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText parses the auth token from the given text.
func (t *Token) UnmarshalText(text []byte) error {
	err := t.Decode(string(text))
	if err != nil {
		return errors.E("unmarshal auth token", err)
	}
	return nil
}

// With returns a copy of this auth token with the given format.
func (t *Token) With(f *tokenFormat) *Token {
	if t.IsNil() {
		return t
	} else if f == nil {
		f = defaultFormat
	}
	var res = *t   // copy the token
	res.Format = f // set the format
	res.Embedded = nil
	res.clearCaches()
	return &res
}

// Equal returns true if this auth token is equal to the provided auth token, false
// otherwise.
func (t *Token) Equal(o *Token) bool {
	if t == o {
		return true
	} else if t == nil || o == nil {
		return false
	}
	return t.String() == o.String()
}

// SignWith signs this token with the given private key, producing a signature of type ES256K.
func (t *Token) SignWith(clientSK *ecdsa.PrivateKey) (err error) {
	return t.SignWithT(clientSK, SigTypes.ES256K())
}

// SignWithT signs this token with the given private key, producing a signature of the given type.
func (t *Token) SignWithT(clientSK *ecdsa.PrivateKey, sigType *TokenSigType) (err error) {
	signFunc := func(digestHash []byte) (sig []byte, err error) {
		return crypto.Sign(digestHash, clientSK)
	}
	signAddr := crypto.PubkeyToAddress(clientSK.PublicKey)
	return t.SignWithFuncT(signAddr, signFunc, sigType)
}

// SignWithFunc signs this token using the provided signing function, producing a signature of type ES256K.
func (t *Token) SignWithFunc(
	signAddr common.Address,
	calcECDSA CalcECDSA) (err error) {

	return t.SignWithFuncT(signAddr, calcECDSA, SigTypes.ES256K())
}

// SignWithFuncT signs this token using the provided signing function, producing a signature of the given type.
func (t *Token) SignWithFuncT(
	signAddr common.Address,
	calcECDSA CalcECDSA,
	sigType *TokenSigType) (err error) {

	return t.sign(signAddr, calcECDSA, sigType)
}

// Verify verifies that the token was signed by the trust address (as returned from getTrustedAddress) and that
// the token's issued & expiration dates are valid with respect to the maximum validity time and accepted time skew.
// Returns nil if valid, an error otherwise.
func (t *Token) Verify(
	getTrustedAddress func(qid types.QID) (common.Address, error),
	maxValidity, timeSkew time.Duration) (err error) {

	e := errors.Template("verify token")

	if !t.SigType.HasSig() {
		// this should never happen and be caught in token.Validate()
		return e(errors.K.Permission, "reason", "token not signed")
	}

	trusted, err := getTrustedAddress(t.QID)
	if err != nil {
		return e(err)
	}

	if err = t.VerifySignatureFrom(trusted); err != nil {
		return e(err)
	}

	if maxValidity != -1 || timeSkew != -1 {
		if err = t.VerifyTimes(maxValidity, timeSkew); err != nil {
			return e(err)
		}
	}

	return nil
}

func (t *Token) Explain() (res string) {
	if t == nil {
		return "token is nil"
	}
	return t.explain("", false)
}

func (t *Token) explain(indent string, isEmbedded bool) (res string) {
	sb := strings.Builder{}
	write := func(label string, size int, desc string, extra ...string) {
		if len(extra) > 0 {
			desc += " | " + extra[0]
		}
		desc = runewidth.Truncate(desc, 100, "...")
		sizeStr := ""
		if size > 0 {
			sizeStr = fmt.Sprintf("%5db", size)
		}
		sb.WriteString(fmt.Sprintf("%s%-20s %-6s  %s\n", indent, label, sizeStr, desc))
	}
	writeNoTrunc := func(label string, size int, desc string, extra ...string) {
		if len(extra) > 0 {
			desc += " | " + extra[0]
		}
		sb.WriteString(fmt.Sprintf("%s%-20s %5db  %s\n", indent, label, size, desc))
	}
	writeErr := func(err error) {
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	writeTokenAndData := func(tok *Token) {
		sb.WriteString(indent)
		sb.WriteString(tok.String())
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(tok.AsJSONIndent(indent, "  "))
		sb.WriteString("\n")
	}
	uncompressedFormat := func() TokenFormat {
		switch t.Format {
		case Formats.CborCompressed():
			return Formats.Cbor()
		case Formats.JsonCompressed():
			return Formats.Json()
		}
		return t.Format
	}
	printable := func(bts []byte) string {
		return strings.Map(func(r rune) rune {
			sub := '‧'
			if unicode.IsLetter(r) ||
				unicode.IsDigit(r) ||
				unicode.IsPunct(r) ||
				r == '=' ||
				r == '+' ||
				r == '_' ||
				r == ' ' {
				return r
			}
			return sub
		}, string(bts))
	}
	prefix := func(tok *Token) string {
		return tok.Type.Prefix + "=" + tok.Type.Name + " " +
			tok.SigType.Prefix + "=" + tok.SigType.Name + " " +
			tok.Format.Prefix + "=" + tok.Format.Name
	}
	defer func() {
		res = sb.String()
	}()

	encNoCompression, err := t.encodeBytesNoCompression()
	if err != nil {
		writeErr(err)
		return
	}

	// encode the token to
	// a) get it's encoded form
	// b) make sure we have the encoding stats
	encoded := t.String()
	tokenLen := len(encoded)
	bodyLen := tokenLen - prefixLen
	sigLen := len(t.Signature)

	switch t.Format {
	case Formats.Legacy():
		sb.WriteString("legacy token: ")
		sb.WriteString(indent)
		sb.WriteString(encoded)
		sb.WriteString("\n")
		jsn := string(t.uncompressedTokenData)
		pretty, err := jsonutil.Pretty(jsn)
		if err != nil {
			sb.WriteString(stringutil.PrefixLines(jsn, indent))
		} else {
			sb.WriteString(stringutil.PrefixLines(pretty, indent))
		}
		sb.WriteString("\n")
		write("MAPPED PREFIX", 0, t.encodePrefix(), prefix(t))
		write("TOKEN", tokenLen, "BODY.SIGNATURE", encoded)
		write("BODY", t.encDetails.encLegacyBody, "base64(PAYLOAD)")
		write("PAYLOAD", t.encDetails.decLegacyBody, t.Format.String()+" (json)")
		if t.SigType == SigTypes.Unsigned() {
			write("SIGNATURE", 0, "none")
		} else {
			write("SIGNATURE", t.encDetails.encLegacySignature, "base64(SIG)")
			write("SIG", t.encDetails.decLegacySignature, t.Signature.String())
		}
		if t.Embedded != nil {
			sb.WriteString("EMBEDDED\n")
			indent = indent + "    "
			write("MAPPED PREFIX", 0, t.Embedded.encodePrefix(), prefix(t.Embedded))
			sb.WriteString(indent)
			sb.WriteString(t.Embedded.AsJSONIndent(indent, "  "))
			sb.WriteString("\n")
		}
		return
	case Formats.LegacySigned():
		sb.WriteString("legacy-signed token: ")
		sb.WriteString(indent)
		sb.WriteString(encoded)
		sb.WriteString("\n")
		write("MAPPED PREFIX", 0, t.encodePrefix(), prefix(t))
		write("TOKEN", tokenLen, "BODY.SIGNATURE", encoded)
		write("BODY", t.encDetails.encLegacyBody, "embedded non-legacy token")
		write("SIGNATURE", t.encDetails.encLegacySignature, "base64(SIG)")
		write("SIG", t.encDetails.decLegacySignature, t.Signature.String())
		if t.Embedded != nil {
			sb.WriteString("EMBEDDED\n")
			sb.WriteString(t.Embedded.explain(indent+"    ", false))
		}
		return
	}

	if !isEmbedded {
		writeTokenAndData(t)
		write("TOKEN", tokenLen, "PREFIX + BODY", encoded)
		write("PREFIX", prefixLen, encoded[:prefixLen], prefix(t))
		write("BODY", bodyLen, "base58(SIGNATURE + PAYLOAD)")
	}

	if t.Embedded == nil {
		sigPlusBodyLen := sigLen + t.encDetails.compressedTokenDataLen
		write("SIGNATURE + PAYLOAD", sigPlusBodyLen, fmt.Sprintf("%db * 138 / 100 + 1 = %db (>= %db)", sigPlusBodyLen, sigPlusBodyLen*138/100+1, bodyLen))
		write("SIGNATURE", sigLen, t.Signature.String())
		write("PAYLOAD", t.encDetails.compressedTokenDataLen, t.Format.String())
		writeNoTrunc(uncompressedFormat().String(), t.encDetails.uncompressedTokenDataLen, printable(encNoCompression))
	} else {
		sigPlusPayloadLen := sigLen + t.encDetails.payloadLen
		write("SIGNATURE + PAYLOAD", sigPlusPayloadLen, fmt.Sprintf("%db * 138 / 100 + 1 = %db (>= %db)", sigPlusPayloadLen, sigPlusPayloadLen*138/100+1, bodyLen))
		write("SIGNATURE", sigLen, t.Signature.String())
		write("PAYLOAD", t.encDetails.payloadLen, "EMBEDDED + TOKENDATA")
		write("TOKENDATA", t.encDetails.compressedTokenDataLen, t.Format.String())
		writeNoTrunc(uncompressedFormat().String(), t.encDetails.uncompressedTokenDataLen, printable(encNoCompression))
		write("EMBEDDED", t.encDetails.embeddedLength+t.encDetails.embeddedPrefix+t.encDetails.embeddedBody, "LENGTH + PREFIX + BODY")
		indent = "    "
		writeTokenAndData(t.Embedded)
		write("LENGTH", t.encDetails.embeddedLength, "varint(len(PREFIX + BODY))", fmt.Sprintf("%d", t.encDetails.embeddedPrefix+t.encDetails.embeddedBody))
		write("PREFIX", t.encDetails.embeddedPrefix, t.Embedded.encodePrefix(), prefix(t.Embedded))
		write("BODY", t.encDetails.embeddedBody, "SIGNATURE + PAYLOAD")
		sb.WriteString(t.Embedded.explain("    ", true))
	}

	return
}

func (t *Token) encodePrefix() string {
	return t.Type.Prefix + t.SigType.Prefix + t.Format.Prefix
}

func (t *Token) encodeTokenAndSigBytes() (data []byte, err error) {
	if t.SigType == SigTypes.Unsigned() {
		data, err = t.encodeBytes()
		if err != nil {
			return nil, err
		}
	} else {
		data = append(t.Signature.Bytes(), t.payload...)
	}
	return data, nil
}

func (t *Token) getPayload() ([]byte, error) {
	_, err := t.encodeBytes()
	return t.payload, err
}

func (t *Token) getUncompressedTokenData() ([]byte, error) {
	_, err := t.encodeBytes()
	return t.uncompressedTokenData, err
}

func (t *Token) encodeBytes() ([]byte, error) {
	if len(t.payload) > 0 {
		return t.payload, nil
	}

	e := errors.Template("encode auth token", errors.K.Invalid, "format", t.Format.Name)

	var err error
	var embedded []byte

	embedded, err = t.encodeEmbedded()
	if err != nil {
		return nil, e(err)
	}

	var data []byte
	data, err = t.encodeBytesNoCompression()
	if err != nil {
		return nil, e(err)
	}

	t.uncompressedTokenData = data
	t.encDetails.uncompressedTokenDataLen = len(data)

	switch t.Format {
	case Formats.JsonCompressed(), Formats.CborCompressed():
		buf := &bytes.Buffer{}
		var w *flate.Writer
		w, err = flate.NewWriter(buf, flate.BestCompression)
		if err == nil {
			_, err = w.Write(data)
			if err == nil {
				err = w.Close()
				data = buf.Bytes()
			}
		}
		if err != nil {
			return nil, e(err)
		}
	}

	t.encDetails.compressedTokenDataLen = len(data)

	if len(embedded) > 0 {
		data = append(embedded, data...)
	}

	t.encDetails.payloadLen = len(data)

	t.payload = data
	return data, nil
}

func (t *Token) encodeBytesNoCompression() (data []byte, err error) {
	if len(t.uncompressedTokenData) > 0 {
		return t.uncompressedTokenData, nil
	}

	switch t.Format {
	case Formats.Legacy():
		legData := TokenDataLegacy{}
		legData.CopyFromTokenData(t)
		data, err = json.Marshal(&legData)
	case Formats.Json(), Formats.JsonCompressed():
		data, err = t.TokenData.EncodeJSON()
	case Formats.Cbor(), Formats.CborCompressed():
		data, err = t.TokenData.EncodeCBOR()
	case Formats.Custom():
		data, err = t.TokenData.Encode()
	}
	return data, err
}

func (t *Token) encodeEmbedded() ([]byte, error) {
	if t.Type != Types.Client() {
		return nil, nil
	}

	prefix := []byte(t.Embedded.encodePrefix())

	bts, err := t.Embedded.encodeTokenAndSigBytes()
	if err != nil {
		return nil, errors.E(err, "reason", "failed to encode embedded token")
	}

	size := varint.ToUvarint(uint64(len(prefix) + len(bts)))

	t.EmbeddedLength = len(size) + len(prefix) + len(bts)

	t.encDetails.embeddedLength = len(size)
	t.encDetails.embeddedPrefix = len(prefix)
	t.encDetails.embeddedBody = len(bts)

	return bytes.Join([][]byte{size, prefix, bts}, nil), nil
}

func (t *Token) decodePrefix(s string, tryLegacy bool) (isLegacy bool, err error) {
	e := errors.Template("decode token prefix", errors.K.Invalid)

	if len(s) < prefixLen {
		return false, e("reason", "prefix too short", "prefix", s)
	}

	var typFound, sigFound, fmtFound bool
	t.Type, typFound = prefixToType[s[:3]]
	t.SigType, sigFound = prefixToSignature[s[3:4]]
	t.Format, fmtFound = prefixToFormat[s[4:prefixLen]]

	if !typFound || !sigFound || !fmtFound {
		if tryLegacy {
			// try legacy token
			t.Format = Formats.Legacy()
			err = t.decodeLegacyString(s)
			if err == nil {
				return true, nil
			}
		}
		if !typFound {
			return false, e(err, "reason", "invalid type prefix")
		}
		if !sigFound {
			return false, e(err, "reason", "invalid signature prefix")
		}
		return false, e(err, "reason", "invalid format prefix")
	}

	return false, nil
}

// decodeString decodes the given token string (stripped of the prefix)
func (t *Token) decodeString(s string) (err error) {
	e := errors.Template("decode auth token", errors.K.Invalid)

	var bts []byte
	bts, err = base58.Decode(s)
	if err != nil {
		return e(err)
	}

	return t.decodeTokenAndSigBytes(bts)
}

func (t *Token) decodeTokenAndSigBytes(bts []byte) (err error) {
	e := errors.Template("decode auth token", errors.K.Invalid)

	switch t.SigType {
	case SigTypes.ES256K(), SigTypes.EIP191Personal():
		if len(bts) <= 65 {
			return e("reason", "token too short")
		}
		t.Signature = sign.NewSig(t.SigType.Code, bts[:65])
		bts = bts[65:]
	}

	err = t.decodeBytes(bts)
	if err != nil {
		return e(err)
	}

	return e.IfNotNil(err)
}

func (t *Token) decodeBytes(bts []byte) error {
	e := errors.Template("decode bytes", errors.K.Invalid)

	t.payload = bts
	t.encDetails.payloadLen = len(bts)

	var err error
	var n int

	n, err = t.decodeEmbedded(bts)
	if err != nil {
		return e(err)
	}

	bts = bts[n:]

	t.encDetails.compressedTokenDataLen = len(bts)

	data := TokenData{}

	switch t.Format {
	case Formats.JsonCompressed(), Formats.CborCompressed():
		bts, err = io.ReadAll(flate.NewReader(bytes.NewReader(bts)))
		if err != nil {
			return e(err)
		}
	}

	t.uncompressedTokenData = bts
	t.encDetails.uncompressedTokenDataLen = len(bts)

	switch t.Format {
	case Formats.JsonCompressed(), Formats.Json():
		err = data.DecodeJSON(bts)
	case Formats.CborCompressed(), Formats.Cbor():
		err = data.DecodeCBOR(bts)
	case Formats.Custom():
		err = data.Decode(bts)
	}
	if err != nil {
		return e(err)
	}

	t.TokenData = data
	return nil
}

func (t *Token) decodeEmbedded(bts []byte) (n int, err error) {
	if t.Type != Types.Client() {
		return 0, nil
	}

	e := errors.Template("decode embedded token")

	var size uint64
	size, n, err = varint.FromUvarint(bts)

	if err != nil {
		return 0, e(err)
	}

	if size < prefixLen {
		return 0, e(errors.K.Invalid,
			"reason", "invalid size of embedded token",
			"size", size)
	}

	bts = bts[n:] // remove "varint"

	embedded := &Token{}

	_, err = embedded.decodePrefix(string(bts[:prefixLen]), false)
	if err == nil {
		err = embedded.decodeTokenAndSigBytes(bts[prefixLen:size])
		if err == nil {
			// init the encoded cache
			_ = embedded.String()
		}
	}

	t.Embedded = embedded
	return int(size) + n, e.IfNotNil(err)
}

func (t *Token) VerifyTimes(maxValidity, timeSkew time.Duration) error {
	e := errors.Template("verify validity times", errors.K.Permission)

	now := utc.Now()

	if now.Before(t.IssuedAt.Add(-timeSkew)) {
		return e("reason", "token not yet valid",
			"issued_at", t.IssuedAt,
			"now", now)
	}

	if now.After(t.Expires) {
		return e("reason", "token expired",
			"expired_at", t.Expires,
			"now", now)
	}

	if maxValidity > 0 {
		if now.After(t.IssuedAt.Add(maxValidity)) {
			return e("reason", "max token validity period expired",
				"issued_at", t.IssuedAt,
				"now", now,
				"max_validity", duration.Spec(maxValidity))
		}
	}

	return nil
}

func (t *Token) LegacyAddr() string {
	if t.HasEthAddr() {
		return t.EthAddr.Hex()
	}
	return t.Subject
}

func (t *Token) LegacyTxID() string {
	if t.HasEthTxHash() {
		return t.EthTxHash.Hex()
	}
	return ""
}

func (t *Token) HasEthAddr() bool {
	return t.EthAddr != zeroAddr
}

func (t *Token) HasEthTxHash() bool {
	return t.EthTxHash != zeroHash
}

func (t *Token) GetQSpaceID() types.QSpaceID {
	if t.SID != nil {
		return t.SID
	}
	if t.Embedded != nil {
		return t.Embedded.GetQSpaceID()
	}
	return nil
}

func (t *Token) GetQLibID() types.QLibID {
	if t.LID != nil {
		return t.LID
	}
	if t.Embedded != nil {
		return t.Embedded.GetQLibID()
	}
	return nil
}

func (t *Token) GetQID() types.QID {
	if t.QID != nil {
		return t.QID
	}
	if t.Embedded != nil {
		return t.Embedded.GetQID()
	}
	return nil
}

func (t *Token) VerifySignedLink(srcQID, linkPath string) error {
	log.Debug("verifying signed link",
		"src", srcQID,
		"path", linkPath,
		"tok", stringutil.Stringer(t.AsJSON))

	e := errors.Template("verify signed link", errors.K.Permission)

	err := t.VerifySignature()
	if err != nil {
		return e(err)
	}

	elv := structured.Wrap(t.Ctx).Get("elv")

	src := elv.Get("src").String()
	if src != srcQID {
		return e("reason", "src differs", "expected", src, "actual", srcQID)
	}

	lnk := elv.Get("lnk").String()
	if lnk != linkPath {
		return e("reason", "link path differs", "expected", lnk, "actual", linkPath)
	}

	return nil
}

func (t *Token) clearCaches() {
	t.encoded = ""
	t.payload = nil
	t.uncompressedTokenData = nil
}

func Describe(tok string) (res string) {
	t, err := parse(tok)
	if err != nil {
		res = errors.E("describe", err).Error() + "\n"
	}
	return res + t.Explain()
}
