package eat

import (
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/common-go/util/ethutil"
	"github.com/eluv-io/utc-go"
)

type Encoder interface {
	// Encode encodes the token as a string or returns an error.
	Encode() (string, error)
	// MustEncode encodes the token as a string - panics in case of error.
	MustEncode() string
	// Token returns the token as struct.
	Token() (*Token, error)
	// MustToken returns the token as struct or panics if an error occurred previously (e.g. during signing).
	MustToken() *Token
}

type Signer interface {
	// Sign signs the token with the given key, producing a ES256K signature on the token's encoded body.
	Sign(pk *ecdsa.PrivateKey) Encoder
	// SignEIP912Personal signs the token with the given key, producing a EIP191PersonalSign signature.
	SignEIP912Personal(pk *ecdsa.PrivateKey) Encoder
}

type TokenBuilder interface {
	Encoder
	Signer
}

// -----------------------------------------------------------------------------

// Must is a helper that returns the given token or panics if err is not nil.
func Must(t *Token, err error) *Token {
	if err != nil {
		panic(err)
	}
	return t
}

// -----------------------------------------------------------------------------

type builder struct {
	token *Token
	err   error
}

func newBuilder(tok *Token) *builder {
	return &builder{
		token: tok,
	}
}

func (b *builder) Token() *Token {
	return b.token
}

// -----------------------------------------------------------------------------

var _ Encoder = (*encoder)(nil)

type encoder struct {
	*builder
}

func newEncoder(tok *Token) *encoder {
	return &encoder{newBuilder(tok)}
}

func (b *encoder) Encode() (string, error) {
	if b.err != nil {
		return "", b.err
	}
	return b.token.Encode()
}

func (b *encoder) MustEncode() string {
	s, err := b.Encode()
	if err != nil {
		panic(err)
	}
	return s
}

func (b *encoder) Token() (*Token, error) {
	if b.token.encoded == "" {
		_, err := b.Encode()
		if err != nil {
			return nil, err
		}
	}
	return b.token, b.err
}

func (b *encoder) MustToken() *Token {
	if b.err != nil {
		panic(b.err)
	}
	return b.token
}

// -----------------------------------------------------------------------------

var _ Signer = (*signer)(nil)

type signer struct {
	enc *encoder
}

func newSigner(tok *Token) *signer {
	return &signer{newEncoder(tok)}
}

func (b *signer) Sign(pk *ecdsa.PrivateKey) Encoder {
	if b.enc.err != nil {
		return b.enc
	}
	b.enc.err = b.enc.token.SignWith(pk)
	return b.enc
}

func (b *signer) SignEIP912Personal(pk *ecdsa.PrivateKey) Encoder {
	if b.enc.err != nil {
		return b.enc
	}
	b.enc.err = b.enc.token.SignWithT(pk, SigTypes.EIP191Personal())
	return b.enc
}

func (b *signer) Token() *Token {
	return b.enc.token
}

// -----------------------------------------------------------------------------

type StateChannelBuilder struct {
	*signer
}

func NewStateChannel(
	sid types.QSpaceID,
	lid types.QLibID,
	qid types.QID,
	subject string) *StateChannelBuilder {

	token := New(Types.StateChannel(), defaultFormat)
	token.SID = sid
	token.LID = lid
	token.QID = qid
	token.Subject = subject
	token.Grant = Grants.Read
	token.IssuedAt = utc.Now().StripMono()
	token.Expires = token.IssuedAt.Add(time.Hour)
	return &StateChannelBuilder{newSigner(token)}
}

func (b *StateChannelBuilder) WithAfgh(afghPublicKey string) *StateChannelBuilder {
	b.enc.token.AFGHPublicKey = afghPublicKey
	return b
}

func (b *StateChannelBuilder) WithGrant(grant Grant) *StateChannelBuilder {
	b.enc.token.Grant = grant
	return b
}

func (b *StateChannelBuilder) WithIssuedAt(issuedAt utc.UTC) *StateChannelBuilder {
	b.enc.token.IssuedAt = issuedAt
	return b
}

func (b *StateChannelBuilder) WithExpires(expiresAt utc.UTC) *StateChannelBuilder {
	b.enc.token.Expires = expiresAt
	return b
}

func (b *StateChannelBuilder) WithCtx(ctx map[string]interface{}) *StateChannelBuilder {
	b.enc.token.Ctx = ctx
	return b
}

// -----------------------------------------------------------------------------

type TxBuilder struct {
	*signer
}

func NewTx(
	sid types.QSpaceID,
	lid types.QLibID,
	EthTxHash common.Hash) *TxBuilder {

	token := New(Types.Tx(), defaultFormat)
	token.SID = sid
	token.LID = lid
	token.EthTxHash = EthTxHash
	return &TxBuilder{newSigner(token)}
}

func (b *TxBuilder) WithAfgh(afghPublicKey string) *TxBuilder {
	b.enc.token.AFGHPublicKey = afghPublicKey
	return b
}

// -----------------------------------------------------------------------------

type NodeTokenBuilder struct {
	*signer
}

func NewNodeToken(
	sid types.QSpaceID,
	qphash types.QPHash) *NodeTokenBuilder {

	token := New(Types.Node(), defaultFormat)
	token.SID = sid
	token.QPHash = qphash
	return &NodeTokenBuilder{newSigner(token)}
}

func (b *NodeTokenBuilder) WithIssuedAt(issuedAt utc.UTC) *NodeTokenBuilder {
	b.enc.token.IssuedAt = issuedAt
	return b
}

func (b *NodeTokenBuilder) WithExpires(expiresAt utc.UTC) *NodeTokenBuilder {
	b.enc.token.Expires = expiresAt
	return b
}

// -----------------------------------------------------------------------------

type EditorSignedBuilder struct {
	*signer
}

func NewEditorSigned(
	sid types.QSpaceID,
	lid types.QLibID,
	qid types.QID) *EditorSignedBuilder {

	token := New(Types.EditorSigned(), defaultFormat)
	token.SID = sid
	token.LID = lid
	token.QID = qid
	token.Grant = Grants.Read
	token.IssuedAt = utc.Now()
	token.Expires = token.IssuedAt.Add(time.Hour)
	return &EditorSignedBuilder{newSigner(token)}
}

func (b *EditorSignedBuilder) WithAfgh(afghPublicKey string) *EditorSignedBuilder {
	b.enc.token.AFGHPublicKey = afghPublicKey
	return b
}

func (b *EditorSignedBuilder) WithGrant(grant Grant) *EditorSignedBuilder {
	b.enc.token.Grant = grant
	return b
}

func (b *EditorSignedBuilder) WithIssuedAt(issuedAt utc.UTC) *EditorSignedBuilder {
	b.enc.token.IssuedAt = issuedAt
	return b
}

func (b *EditorSignedBuilder) WithExpires(expiresAt utc.UTC) *EditorSignedBuilder {
	b.enc.token.Expires = expiresAt
	return b
}

func (b *EditorSignedBuilder) WithCtx(ctx map[string]interface{}) *EditorSignedBuilder {
	b.enc.token.Ctx = ctx
	return b
}

func (b *EditorSignedBuilder) WithSubject(s string) *EditorSignedBuilder {
	b.enc.token.Subject = s
	return b
}

func (b *EditorSignedBuilder) WithConfirmation(s ClientConfirmation) *EditorSignedBuilder {
	b.enc.token.Confirmation = s
	return b
}

func (b *EditorSignedBuilder) Sign(pk *ecdsa.PrivateKey) Encoder {
	if len(b.enc.token.Subject) == 0 {
		b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	}
	return b.signer.Sign(pk)
}

func (b *EditorSignedBuilder) SignEIP912Personal(pk *ecdsa.PrivateKey) Encoder {
	if len(b.enc.token.Subject) == 0 {
		b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	}
	return b.signer.SignEIP912Personal(pk)
}

// -----------------------------------------------------------------------------

type PlainBuilder struct {
	*signer
}

func NewPlain(
	sid types.QSpaceID,
	lid types.QLibID) *PlainBuilder {

	token := New(Types.Plain(), defaultFormat)
	token.SID = sid
	token.LID = lid
	return &PlainBuilder{newSigner(token)}
}

func (b *PlainBuilder) WithQID(qid types.QID) *PlainBuilder {
	b.enc.token.QID = qid
	return b
}

func (b *PlainBuilder) WithAfgh(afghPublicKey string) *PlainBuilder {
	b.enc.token.AFGHPublicKey = afghPublicKey
	return b
}

// -----------------------------------------------------------------------------

type AnonymousBuilder struct {
	*encoder
}

func NewAnonymous(
	sid types.QSpaceID,
	lid types.QLibID) *AnonymousBuilder {

	token := New(Types.Anonymous(), defaultFormat)
	token.SID = sid
	token.LID = lid
	return &AnonymousBuilder{newEncoder(token)}
}

func (b *AnonymousBuilder) WithQID(qid types.QID) *AnonymousBuilder {
	b.token.QID = qid
	return b
}

// -----------------------------------------------------------------------------

type SignedLinkBuilder struct {
	*signer
}

func NewSignedLink(
	sid types.QSpaceID,
	lid types.QLibID,
	qid types.QID,
	linkPath string,
	srcQID types.QID) *SignedLinkBuilder {

	token := New(Types.SignedLink(), defaultFormat)
	token.SID = sid
	token.LID = lid
	token.QID = qid
	token.Grant = Grants.Read
	token.IssuedAt = utc.Now()
	// no expiration time per default
	// token.Expires = token.IssuedAt.Add(time.Hour)
	token.Ctx = map[string]interface{}{
		"elv": map[string]interface{}{
			"lnk": linkPath,
			"src": srcQID.String(),
		},
	}
	return &SignedLinkBuilder{newSigner(token)}
}

func (b *SignedLinkBuilder) WithAfgh(afghPublicKey string) *SignedLinkBuilder {
	b.enc.token.AFGHPublicKey = afghPublicKey
	return b
}

func (b *SignedLinkBuilder) WithGrant(grant Grant) *SignedLinkBuilder {
	b.enc.token.Grant = grant
	return b
}

func (b *SignedLinkBuilder) WithIssuedAt(issuedAt utc.UTC) *SignedLinkBuilder {
	b.enc.token.IssuedAt = issuedAt
	return b
}

func (b *SignedLinkBuilder) WithExpires(expiresAt utc.UTC) *SignedLinkBuilder {
	b.enc.token.Expires = expiresAt
	return b
}

func (b *SignedLinkBuilder) MergeCtx(ctx map[string]interface{}) *SignedLinkBuilder {
	var res interface{}
	res, b.enc.err = structured.Merge(b.enc.token.Ctx, nil, ctx)
	b.enc.token.Ctx = res.(map[string]interface{})
	return b
}

func (b *SignedLinkBuilder) Sign(pk *ecdsa.PrivateKey) Encoder {
	b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	return b.signer.Sign(pk)
}

func (b *SignedLinkBuilder) SignEIP912Personal(pk *ecdsa.PrivateKey) Encoder {
	b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	return b.signer.SignEIP912Personal(pk)
}

// -----------------------------------------------------------------------------

// PENDING(LUK): review offered methods on ClientSignedBuilder

type ClientSignedBuilder struct {
	*signer
}

func NewClientSigned(sid types.QSpaceID) *ClientSignedBuilder {
	token := New(Types.ClientSigned(), defaultFormat)
	token.SID = sid
	token.IssuedAt = utc.Now()
	token.Expires = token.IssuedAt.Add(time.Hour)
	return &ClientSignedBuilder{newSigner(token)}
}

func (b *ClientSignedBuilder) WithLID(lid types.QLibID) *ClientSignedBuilder {
	b.enc.token.LID = lid
	return b
}

func (b *ClientSignedBuilder) WithQID(qid types.QID) *ClientSignedBuilder {
	b.enc.token.QID = qid
	return b
}

func (b *ClientSignedBuilder) WithAfgh(afghPublicKey string) *ClientSignedBuilder {
	b.enc.token.AFGHPublicKey = afghPublicKey
	return b
}

func (b *ClientSignedBuilder) WithGrant(grant Grant) *ClientSignedBuilder {
	b.enc.token.Grant = grant
	return b
}

func (b *ClientSignedBuilder) WithIssuedAt(issuedAt utc.UTC) *ClientSignedBuilder {
	b.enc.token.IssuedAt = issuedAt
	return b
}

func (b *ClientSignedBuilder) WithExpires(expiresAt utc.UTC) *ClientSignedBuilder {
	b.enc.token.Expires = expiresAt
	return b
}

func (b *ClientSignedBuilder) WithCtx(ctx map[string]interface{}) *ClientSignedBuilder {
	b.enc.token.Ctx = ctx
	return b
}

func (b *ClientSignedBuilder) WithSubject(s string) *ClientSignedBuilder {
	b.enc.token.Subject = s
	return b
}

func (b *ClientSignedBuilder) WithConfirmation(s ClientConfirmation) *ClientSignedBuilder {
	b.enc.token.Confirmation = s
	return b
}

func (b *ClientSignedBuilder) Sign(pk *ecdsa.PrivateKey) Encoder {
	if len(b.enc.token.Subject) == 0 {
		b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	}
	return b.signer.Sign(pk)
}

func (b *ClientSignedBuilder) SignEIP912Personal(pk *ecdsa.PrivateKey) Encoder {
	if len(b.enc.token.Subject) == 0 {
		b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	}
	return b.signer.SignEIP912Personal(pk)
}

// -----------------------------------------------------------------------------

type ClientConfirmationBuilder struct {
	*signer
}

func NewClientConfirmation(issuedAt utc.UTC, d ...time.Duration) *ClientConfirmationBuilder {
	token := New(Types.ClientConfirmation(), defaultFormat)
	token.IssuedAt = issuedAt
	dur := time.Second * 20
	if len(d) > 0 {
		dur = d[0]
	}
	token.Expires = token.IssuedAt.Add(dur)
	return &ClientConfirmationBuilder{newSigner(token)}
}

func (b *ClientConfirmationBuilder) WithExpires(expiresAt utc.UTC) *ClientConfirmationBuilder {
	b.enc.token.Expires = expiresAt
	return b
}

func (b *ClientConfirmationBuilder) WithCtx(ctx map[string]interface{}) *ClientConfirmationBuilder {
	b.enc.token.Ctx = ctx
	return b
}

func (b *ClientConfirmationBuilder) Sign(pk *ecdsa.PrivateKey) Encoder {
	return b.signer.Sign(pk)
}

func (b *ClientConfirmationBuilder) SignEIP912Personal(pk *ecdsa.PrivateKey) Encoder {
	return b.signer.SignEIP912Personal(pk)
}
