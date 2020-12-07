package eat

import (
	"bytes"
	"compress/flate"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
	"unicode"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/sign"
	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/jsonutil"
	"github.com/qluvio/content-fabric/util/stringutil"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mattn/go-runewidth"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-varint"
)

const prefixLen = 6 // length of entire prefix including type, sig-type and format

// Token is an auth token, defined by it's type, format, and token data.
type Token struct {
	Type    TokenType
	Format  *tokenFormat
	SigType *tokenSigType
	TokenData
	TokenBytes []byte
	Signature  sign.Sig
	// SignerAddr common.Address

	encoded string // cache for the token's encoded string form

	Embedded       *Token
	EmbeddedLength int // the length of the embedded token within the TokenBytes

	encDetails encodingDetails // details collected during encoding used in Explain()
}

type encodingDetails struct {
	uncompressedTokenDataLen int    // length of encoded, uncompressed token data
	compressedTokenDataLen   int    // length of encoded, compressed token data
	payloadLen               int    // length of payload: optional embedded token + token data
	embeddedLength           int    // length of embedded token's length varint
	embeddedPrefix           int    // length of embedded token's prefix
	embeddedBody             int    // length of embedded token's body
	uncompressedTokenData    []byte // the original uncompressed token data (if decoded from a string)
}

// New creates a new auth token with the given type, digest, size, and ID
func New(typ TokenType, format *tokenFormat, sig *tokenSigType) *Token {
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

func (t *Token) AsJSON(prefix, indent string) string {
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
	return t.encoded, nil
}

// Decode parses the auth token from the given string representation.
func (t *Token) Decode(s string) (err error) {
	if s == "" {
		return nil
	}

	e := errors.Template("decode auth token", errors.K.Invalid, "token_string", s)

	var isLegacy bool
	isLegacy, err = t.decodePrefix(s, true)
	if err != nil {
		return e(err)
	}
	if isLegacy {
		// legacy token was already parsed completely in decodePrefix
		return nil
	}

	isLegacySigned, err := t.decodeLegacySigned(s)
	if err != nil {
		return e(err)
	}
	if isLegacySigned {
		return nil
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

	e := errors.Template("decode legacy signed token")

	// mixed legacy: new token, but client-signed the old-fashion way
	sctString := token[:dotIdx]
	legacySig := token[dotIdx+1:]

	sct, err := Parse(sctString)
	if err != nil {
		return true, e(err)
	}

	t.embedLegacyStateChannelToken(sct)
	t.Format = Formats.Legacy()
	t.Type = Types.Client()
	t.TokenBytes = []byte(sctString)
	t.encoded = token

	return true, e.IfNotNil(t.decodeLegacySignature(legacySig))
}

func (t *Token) Validate() (err error) {
	e := errors.Template("validate auth token", errors.K.Invalid, "type", t.Type)

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

	switch t.SigType {
	case SigTypes.ES256K():
		if t.Signature.IsNil() || len(t.TokenBytes) == 0 {
			return e(
				"reason", "token requires signature, but has no signature data",
				"sig_type", t.SigType)
		}
	}

	if t.SID.IsNil() {
		return e("reason", "space id missing")
	}

	// lib seems optional... (because it can be specified in the URL?)
	//if t.LID.IsNil() {
	//	return e("reason", "lib id missing")
	//}

	switch t.Type {
	case Types.StateChannel(), Types.EditorSigned():
		// required
		if t.SigType != SigTypes.ES256K() {
			return e("reason", "signature missing")
		}
		if t.QID.IsNil() {
			return e("reason", "qid missing")
		}
		// for state-channel: don't always require a subject since the actual
		// subject might be 'something else' in the context.
		// Note that here we don't know what could be in the context but (in the
		// future) we might (should ?) require the context to have some known
		// keys since a token should always be granted to someone ...
		if t.Subject == "" && (len(t.Ctx) == 0 || t.Type != Types.StateChannel()) {
			return e("reason", "subject missing")
		}
		if t.Grant == "" {
			return e("reason", "grant missing")
		}
		if t.IssuedAt.IsZero() {
			return e("reason", "issued at missing")
		}
		if t.Expires.IsZero() {
			return e("reason", "expires missing")
		}
		if t.Expires.Before(t.IssuedAt) {
			return e("reason", "expires before issued at",
				"expires", t.Expires,
				"issues_at", t.IssuedAt)
		}
		// not allowed
		if t.HasEthTxHash() {
			return e("reason", "tx hash not allowed")
		}
		if !t.QPHash.IsNil() {
			return e("reason", "qphash not allowed")
		}
		// additional editor-signed requirements are checked in auth provider
		return nil
	}

	// not allowed for all other types
	if t.Subject != "" {
		return e("reason", "subject not allowed")
	}
	if t.Grant != "" {
		return e("reason", "grant not allowed")
	}
	if !t.IssuedAt.IsZero() {
		return e("reason", "issued at not allowed")
	}
	if !t.Expires.IsZero() {
		return e("reason", "expires not allowed")
	}
	if len(t.Ctx) > 0 {
		return e("reason", "ctx not allowed")
	}

	switch t.Type {
	case Types.Client():
		// required
		if t.Embedded == nil {
			return e("reason", "embedded token missing")
		}
		// no allowed
		if t.HasEthTxHash() {
			return e("reason", "tx hash not allowed")
		}
		if !t.QPHash.IsNil() {
			return e("reason", "qphash not allowed")
		}
		return e.IfNotNil(t.Embedded.Validate())
	case Types.Tx():
		// required
		if !t.HasEthTxHash() {
			return e("reason", "tx hash missing")
		}
		if t.SigType != SigTypes.ES256K() {
			return e("reason", "signature missing")
		}
		// no allowed
		if !t.QPHash.IsNil() {
			return e("reason", "qphash not allowed")
		}
		if !t.QID.IsNil() {
			return e("reason", "qid not allowed")
		}
	case Types.Plain():
		// required
		if t.SigType != SigTypes.ES256K() {
			return e("reason", "signature missing")
		}
		// not allowed
		if t.HasEthTxHash() {
			return e("reason", "tx hash not allowed")
		}
		if !t.QPHash.IsNil() {
			return e("reason", "qphash not allowed")
		}
	case Types.Anonymous():
		// not allowed
		if t.SigType != SigTypes.Unsigned() {
			return e("reason", "signature not allowed")
		}
		if t.HasEthTxHash() {
			return e("reason", "tx hash not allowed")
		}
		if !t.QPHash.IsNil() {
			return e("reason", "qphash not allowed")
		}
		if t.HasEthAddr() {
			return e("reason", "address not allowed")
		}
	case Types.Node():
		// required
		if t.SigType != SigTypes.ES256K() {
			return e("reason", "signature missing")
		}
		if t.QPHash.IsNil() {
			return e("reason", "qphash missing")
		}
		// not allowed
		if t.HasEthTxHash() {
			return e("reason", "tx hash not allowed")
		}
	}

	return nil
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

// As returns a copy of this auth token with the given format.
func (t *Token) With(f *tokenFormat) *Token {
	if t.IsNil() {
		return t
	} else if f == nil {
		f = defaultFormat
	}
	var res = *t   // copy the token
	res.Format = f // set the format
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

// SignWith signs this token with the given private key.
func (t *Token) SignWith(clientSK *ecdsa.PrivateKey) (err error) {
	signFunc := func(digestHash []byte) (sig []byte, err error) {
		return crypto.Sign(digestHash, clientSK)
	}
	signAddr := crypto.PubkeyToAddress(clientSK.PublicKey)
	return t.SignWithFunc(signAddr, signFunc)
}

// SignWith signs this token using the provided signing function.
func (t *Token) SignWithFunc(
	signAddr common.Address,
	signFunc func(digestHash []byte) (sig []byte, err error)) (err error) {

	e := errors.Template("signWith", errors.K.Invalid)

	t.EthAddr = signAddr

	// clear the encoded cache
	t.encoded = ""

	t.TokenBytes, err = t.encodeBytes()
	if err != nil {
		return e(err)
	}
	hsh := crypto.Keccak256(t.TokenBytes)
	sig, err := signFunc(hsh)
	if err != nil {
		return e(err)
	}
	if len(sig) != 65 {
		return e("reason", "signature must be 65 bytes long",
			"len", len(sig))
	}
	ns := sign.NewSig(sign.ES256K, sig)
	t.Signature = sign.NewSig(sign.ES256K, ns.EthAdjustBytes())
	t.SigType = SigTypes.ES256K()
	return nil
}

func (t *Token) VerifySignatureFrom(trusted common.Address) (err error) {
	return t.Verify(
		func(qid types.QID) (common.Address, error) {
			return trusted, nil
		},
		-1,
		-1)
}

func (t *Token) Verify(
	getTrustedAddress func(qid types.QID) (common.Address, error),
	maxValidity, timeSkew time.Duration) (err error) {

	switch t.SigType {
	case SigTypes.ES256K():
	default:
		return nil
	}

	e := errors.Template("verify")

	switch t.Type {
	case Types.StateChannel(), Types.EditorSigned():
		trusted, err := getTrustedAddress(t.QID)
		if err != nil {
			return e(err)
		}
		if err = t.verifySignatureFrom(trusted); err != nil {
			return e(err)
		}
		if maxValidity != -1 || timeSkew != -1 {
			if err = t.VerifyTimes(maxValidity, timeSkew); err != nil {
				return e(err)
			}
		}
	default:
		return t.VerifySignature()
	}

	return nil
}

func (t *Token) VerifySignature() error {
	e := errors.Template("verify token", errors.K.Permission)

	err := t.Validate()
	if err != nil {
		return e(err)
	}

	if t.Signature.IsNil() {
		return nil
	}

	if !t.HasEthAddr() {
		//  a signature but no address - only allowed for legacy tokens
		signerAddress, err := t.Signature.SignerAddress(t.TokenBytes)
		if err != nil {
			return e(err)
		}
		t.EthAddr = signerAddress
		return nil
	}

	return t.verifySignatureFrom(t.EthAddr)
}

func (t *Token) Explain() (res string) {
	return t.explain("", false)
}

func (t *Token) explain(indent string, isEmbedded bool) (res string) {
	sb := strings.Builder{}
	write := func(label string, size int, desc string, extra ...string) {
		if len(extra) > 0 {
			desc += " | " + extra[0]
		}
		desc = runewidth.Truncate(desc, 100, "...")
		sb.WriteString(fmt.Sprintf("%s%-20s %5db  %s\n", indent, label, size, desc))
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
		sb.WriteString(tok.AsJSON(indent, "  "))
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

	if t.Format == Formats.Legacy() {
		sb.WriteString("legacy token: ")
		sb.WriteString(indent)
		sb.WriteString(t.String())
		sb.WriteString("\n")
		jsn := string(t.encDetails.uncompressedTokenData)
		pretty, err := jsonutil.Pretty(jsn)
		if err != nil {
			sb.WriteString(stringutil.PrefixLines(jsn, indent))
		} else {
			sb.WriteString(stringutil.PrefixLines(pretty, indent))
		}
		sb.WriteString("\n")
		if t.Embedded != nil {
			sb.WriteString("EMBEDDED\n")
			indent = indent + "    "
			sb.WriteString(indent)
			sb.WriteString(t.Embedded.AsJSON(indent, "  "))
			sb.WriteString("\n")
		}
		return
	}

	// encode the token to
	// a) get it's encoded form
	// b) make sure we have the encoding stats
	encoded := t.String()

	tokenLen := len(encoded)
	bodyLen := tokenLen - prefixLen
	sigLen := len(t.Signature)

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
		data = append(t.Signature.Bytes(), t.TokenBytes...)
	}
	return data, nil
}

func (t *Token) encodeBytes() ([]byte, error) {
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

	t.encDetails.uncompressedTokenData = data
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

	return data, nil
}

func (t *Token) encodeBytesNoCompression() (data []byte, err error) {
	if t.encoded != "" {
		return t.encDetails.uncompressedTokenData, nil
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
	case SigTypes.ES256K():
		if len(bts) <= 65 {
			return e("reason", "token too short")
		}
		t.Signature = sign.NewSig(sign.ES256K, bts[:65])
		bts = bts[65:]
		t.TokenBytes = bts
	}

	err = t.decodeBytes(bts)
	if err != nil {
		return e(err)
	}

	return e.IfNotNil(err)
}

func (t *Token) decodeBytes(bts []byte) error {
	e := errors.Template("decode bytes", errors.K.Invalid)

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
		bts, err = ioutil.ReadAll(flate.NewReader(bytes.NewReader(bts)))
		if err != nil {
			return e(err)
		}
	}

	t.encDetails.uncompressedTokenData = bts
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

func (t *Token) verifySignatureFrom(trusted common.Address) error {
	e := errors.Template("verify token signature", errors.K.Permission)
	if t == nil {
		return e("reason", "token is nil")
	}
	if t.Signature == nil {
		return e("reason", "signature is nil")
	}

	signerAddress, err := t.Signature.SignerAddress(t.TokenBytes)
	if err != nil {
		return e(err)
	}

	// verify that auth data is from trusted address
	if !bytes.Equal(trusted.Bytes(), signerAddress.Bytes()) {
		return e("reason", "EAT invalid trust address or token tampered with",
			"trust_address", trusted.String(),
			"signer_address", signerAddress.String())
	}
	return nil
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

	if now.After(t.IssuedAt.Add(maxValidity)) {
		return e("reason", "max token validity period expired",
			"issued_at", t.IssuedAt,
			"now", now,
			"max_validity", maxValidity)
	}

	return nil
}

func (t *Token) LegacyAddr() string {
	if t.HasEthAddr() {
		return t.EthAddr.Hex()
	} else if t.Subject != "" {
		return t.Subject
	}
	return ""
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

func Describe(tok string) string {
	t, err := Parse(tok)
	if err != nil {
		return errors.E("describe", err).Error()
	}
	return t.Explain()
}