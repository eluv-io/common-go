package eat

import (
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/util/ethutil"

	"github.com/qluvio/content-fabric/format/structured"

	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/format/utc"
)

type Encoder interface {
	// Encode encodes the token as a string or returns an error.
	Encode() (string, error)
	// MustEncode encodes the token as a string - panics in case of error.
	MustEncode() string
}

type Signer interface {
	Sign(pk *ecdsa.PrivateKey) Encoder
}

type TokenBuilder interface {
	Encoder
	Signer
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

func (b *signer) Token() *Token {
	return b.enc.Token()
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

	token := New(Types.StateChannel(), defaultFormat, SigTypes.Unsigned())
	token.SID = sid
	token.LID = lid
	token.QID = qid
	token.Subject = subject
	token.Grant = Grants.Read
	token.IssuedAt = utc.Now()
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

	token := New(Types.Tx(), defaultFormat, SigTypes.Unsigned())
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

	token := New(Types.Node(), defaultFormat, SigTypes.Unsigned())
	token.SID = sid
	token.QPHash = qphash
	return &NodeTokenBuilder{newSigner(token)}
}

// -----------------------------------------------------------------------------

type EditorSignedBuilder struct {
	*signer
}

func NewEditorSigned(
	sid types.QSpaceID,
	lid types.QLibID,
	qid types.QID) *EditorSignedBuilder {

	token := New(Types.EditorSigned(), defaultFormat, SigTypes.Unsigned())
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

func (b *EditorSignedBuilder) Sign(pk *ecdsa.PrivateKey) Encoder {
	b.enc.token.Subject = ethutil.AddressToID(crypto.PubkeyToAddress(pk.PublicKey), id.User).String()
	return b.signer.Sign(pk)
}

// -----------------------------------------------------------------------------

type PlainBuilder struct {
	*signer
}

func NewPlain(
	sid types.QSpaceID,
	lid types.QLibID) *PlainBuilder {

	token := New(Types.Plain(), defaultFormat, SigTypes.Unsigned())
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

	token := New(Types.Anonymous(), defaultFormat, SigTypes.Unsigned())
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

	token := New(Types.SignedLink(), defaultFormat, SigTypes.Unsigned())
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
