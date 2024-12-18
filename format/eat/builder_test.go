package eat_test

import (
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/eat"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/common-go/util/ethutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

var sub = "token subject"
var lnk = "./meta/some/path"
var srcQID = id.MustParse("iq__48iLSSjzN3PRyzwWqDmG5Dx1zkfL")

func TestTokenBuilders(t *testing.T) {
	now := utc.Now().Truncate(time.Second) // times in tokens have second precision
	anHourAgo := now.Add(-time.Hour)
	anHourFromNow := now.Add(time.Hour)
	defer utc.MockNow(now)()

	ctx1 := map[string]interface{}{
		"k1": "v1",
		"k2": "v2",
	}

	sigTypes := []*eat.TokenSigType{eat.SigTypes.ES256K(), eat.SigTypes.EIP191Personal()}
	for _, sigType := range sigTypes {
		sign := func(signer eat.Signer, key *ecdsa.PrivateKey) eat.Encoder {
			switch sigType {
			case eat.SigTypes.ES256K():
				return signer.Sign(key)
			case eat.SigTypes.EIP191Personal():
				return signer.SignEIP912Personal(key)
			}
			panic(errors.E("sign", errors.K.Invalid, "sig_type", sigType))
		}
		t.Run(fmt.Sprint(sigType), func(t *testing.T) {

			tests := []struct {
				encoder     eat.Encoder
				token       *eat.Token
				wantFailure bool
				wantType    eat.TokenType
				validate    func(t *testing.T, token *eat.Token)
			}{

				{
					encoder:  sign(eat.NewStateChannel(sid, lid, qid, sub), serverSK),
					wantType: eat.Types.StateChannel(),
					validate: stateChannelDefault,
				},
				{
					encoder:     sign(eat.NewStateChannel(sid, lid, qid, ""), serverSK),
					wantFailure: true, // no subject and no ctx
					wantType:    eat.Types.Unknown(),
				},
				{
					encoder:     sign(eat.NewStateChannel(sid, lid, qid, "").WithCtx(ctx1), serverSK),
					wantFailure: false, // no subject but ctx
					wantType:    eat.Types.StateChannel(),
					validate:    stateChannelNoSubject,
				},
				{
					encoder:  sign(eat.NewStateChannel(sid, lid, qid, sub).WithAfgh("afgh-pk"), serverSK),
					wantType: eat.Types.StateChannel(),
					validate: func(t *testing.T, token *eat.Token) {
						stateChannelDefault(t, token)
						require.Equal(t, "afgh-pk", token.AFGHPublicKey)
						require.Equal(t, now, token.IssuedAt)
					},
				},
				{
					encoder: sign(eat.NewStateChannel(sid, lid, qid, sub).
						WithAfgh("afgh-pk").
						WithCtx(ctx1).
						WithIssuedAt(anHourAgo).
						WithExpires(anHourFromNow),
						serverSK),
					wantType: eat.Types.StateChannel(),
					validate: func(t *testing.T, token *eat.Token) {
						stateChannelDefault(t, token)
						require.Equal(t, "afgh-pk", token.AFGHPublicKey)
						require.Equal(t, anHourAgo, token.IssuedAt)
						require.Equal(t, anHourFromNow, token.Expires)
						require.Equal(t, ctx1, token.Ctx)
					},
				},
				{
					encoder:  sign(eat.NewTx(sid, lid, txh), clientSK),
					wantType: eat.Types.Tx(),
					validate: txDefault,
				},
				{
					encoder:  sign(eat.NewTx(sid, lid, txh).WithAfgh("afgh-pk"), clientSK),
					wantType: eat.Types.Tx(),
					validate: func(t *testing.T, token *eat.Token) {
						txDefault(t, token)
						require.Equal(t, "afgh-pk", token.AFGHPublicKey)
					},
				},
				{
					encoder:  sign(eat.NewPlain(sid, lid), clientSK),
					wantType: eat.Types.Plain(),
					validate: plainDefault,
				},
				{
					encoder:  sign(eat.NewPlain(sid, lid).WithQID(qid), clientSK),
					wantType: eat.Types.Plain(),
					validate: func(t *testing.T, token *eat.Token) {
						plainDefault(t, token)
						require.Equal(t, qid, token.QID)
					},
				},
				{
					encoder:  eat.NewAnonymous(sid, lid),
					wantType: eat.Types.Anonymous(),
					validate: anonymousDefault,
				},
				{
					encoder:  eat.NewAnonymous(sid, lid).WithQID(qid),
					wantType: eat.Types.Anonymous(),
					validate: func(t *testing.T, token *eat.Token) {
						anonymousDefault(t, token)
						require.Equal(t, qid, token.QID)
					},
				},
				{
					encoder: sign(eat.NewEditorSigned(sid, lid, qid).
						WithGrant(eat.Grants.Read).
						WithAfgh("afgh-pk").
						WithCtx(map[string]interface{}{"additional": "context"}),
						clientSK),
					wantType: eat.Types.EditorSigned(),
					validate: func(t *testing.T, token *eat.Token) {
						editorSignedDefault(t, token)
						require.Equal(t, qid, token.QID)
						require.Equal(t, "afgh-pk", token.AFGHPublicKey)
					},
				},
				{
					encoder: sign(eat.NewSignedLink(sid, lid, qid, lnk, srcQID).
						WithGrant(eat.Grants.Read).
						MergeCtx(map[string]interface{}{"additional": "context"}),
						clientSK),
					wantType: eat.Types.SignedLink(),
					validate: func(t *testing.T, token *eat.Token) {
						signedLinkDefault(t, token)
						ctx := structured.Wrap(token.Ctx)
						require.Equal(t, "context", ctx.At("additional").String())
						require.Equal(t, lnk, ctx.At("elv/lnk").String())
						require.Equal(t, srcQID.String(), ctx.At("elv/src").String())

					},
				},
				{
					token: eat.Must(
						eat.NewClientToken(
							sign(
								eat.NewStateChannel(sid, lid, qid, sub).
									WithAfgh("afgh-pk").
									WithCtx(ctx1).
									WithIssuedAt(anHourAgo).
									WithExpires(anHourFromNow),
								clientSK).MustToken())),
					wantType: eat.Types.Client(),
					validate: clientDefault,
				},
				{
					encoder: sign(eat.NewClientSigned(sid).
						WithGrant(eat.Grants.Read).
						WithCtx(map[string]interface{}{"additional": "context"}),
						clientSK),
					wantType: eat.Types.ClientSigned(),
					validate: func(t *testing.T, token *eat.Token) {
						clientSignedDefault(t, token)
					},
				},
			}

			for _, test := range tests {
				t.Run(test.wantType.String(), func(t *testing.T) {
					var encoded string
					var err error
					if test.encoder != nil {
						encoded, err = test.encoder.Encode()
					} else {
						encoded, err = test.token.Encode()
					}
					if test.wantFailure {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)

					tok, err := eat.Parse(encoded)
					require.NoError(t, err)

					require.NoError(t, tok.Validate())
					require.NoError(t, tok.VerifySignature())

					require.Equal(t, test.wantType, tok.Type)
					test.validate(t, tok)

					_, err = tok.TokenData.EncodeJSON()
					require.NoError(t, err)
				})
			}
		})
	}
}

func stateChannelDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	require.Equal(t, qid, token.QID)
	require.Equal(t, sub, token.Subject)
	require.NotEqual(t, utc.Zero, token.IssuedAt)
	assertAuthorization(t, token, sub, nil)
}

func stateChannelNoSubject(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	require.Equal(t, qid, token.QID)
	require.Equal(t, "", token.Subject)
	require.NotEqual(t, utc.Zero, token.IssuedAt)
	assertAuthorization(t, token, "", nil)
}

func txDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	require.Equal(t, txh, token.EthTxHash)
	assertAuthorization(t, token, clientID.String(), clientID)
}

func plainDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	assertAuthorization(t, token, clientID.String(), clientID)
}

func anonymousDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	assertAuthorization(t, token, "", nil)
}

func editorSignedDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	require.Equal(t, qid, token.QID)
	require.Equal(t, ethutil.AddressToID(token.EthAddr, id.User).String(), token.Subject)
	require.NotEqual(t, utc.Zero, token.IssuedAt)
	assertAuthorization(t, token, clientID.String(), clientID)
}

func signedLinkDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, lid, token.LID)
	require.Equal(t, qid, token.QID)
	require.Equal(t, ethutil.AddressToID(token.EthAddr, id.User).String(), token.Subject)
	ctx := structured.Wrap(token.Ctx).At("elv")
	require.NotZero(t, ctx.At("lnk").String())
	require.NotZero(t, ctx.At("src").String())
	require.NotEqual(t, utc.Zero, token.IssuedAt)
	assertAuthorization(t, token, clientID.String(), clientID)
}

func clientDefault(t *testing.T, token *eat.Token) {
	stateChannelDefault(t, token.Embedded)
	assertAuthorization(t, token, sub, nil)
}

func clientSignedDefault(t *testing.T, token *eat.Token) {
	require.Equal(t, sid, token.SID)
	require.Equal(t, ethutil.AddressToID(token.EthAddr, id.User).String(), token.Subject)
	require.NotEqual(t, utc.Zero, token.IssuedAt)
	assertAuthorization(t, token, clientID.String(), clientID)
}

func assertAuthorization(t *testing.T, tok *eat.Token, wantUser string, wantUserId types.UserID) {
	auth, err := eat.NewAuthorization(tok)
	require.NoError(t, err)
	require.Equal(t, wantUser, auth.User())
	require.Equal(t, wantUserId, auth.UserId())
}
