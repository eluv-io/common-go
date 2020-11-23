package eat

import (
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/format/utc"
)

type Encoder interface {
	Encode() (string, error)
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
	b.enc.token.Subject = crypto.PubkeyToAddress(pk.PublicKey).Hex()
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
