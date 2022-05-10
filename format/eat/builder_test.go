package eat_test

import (
	"crypto/ecdsa"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/eat"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/util/ethutil"
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
				validate    func(t *testing.T, data *eat.Token)
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
					validate: func(t *testing.T, data *eat.Token) {
						stateChannelDefault(t, data)
						require.Equal(t, "afgh-pk", data.AFGHPublicKey)
						require.Equal(t, now, data.IssuedAt)
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
					validate: func(t *testing.T, data *eat.Token) {
						stateChannelDefault(t, data)
						require.Equal(t, "afgh-pk", data.AFGHPublicKey)
						require.Equal(t, anHourAgo, data.IssuedAt)
						require.Equal(t, anHourFromNow, data.Expires)
						require.Equal(t, ctx1, data.Ctx)
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
					validate: func(t *testing.T, data *eat.Token) {
						txDefault(t, data)
						require.Equal(t, "afgh-pk", data.AFGHPublicKey)
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
					validate: func(t *testing.T, data *eat.Token) {
						plainDefault(t, data)
						require.Equal(t, qid, data.QID)
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
					validate: func(t *testing.T, data *eat.Token) {
						anonymousDefault(t, data)
						require.Equal(t, qid, data.QID)
					},
				},
				{
					encoder: sign(eat.NewEditorSigned(sid, lid, qid).
						WithGrant(eat.Grants.Read).
						WithCtx(map[string]interface{}{"additional": "context"}),
						clientSK),
					wantType: eat.Types.EditorSigned(),
					validate: func(t *testing.T, data *eat.Token) {
						editorSignedDefault(t, data)
						require.Equal(t, qid, data.QID)
					},
				},
				{
					encoder: sign(eat.NewSignedLink(sid, lid, qid, lnk, srcQID).
						WithGrant(eat.Grants.Read).
						MergeCtx(map[string]interface{}{"additional": "context"}),
						clientSK),
					wantType: eat.Types.SignedLink(),
					validate: func(t *testing.T, data *eat.Token) {
						signedLinkDefault(t, data)
						ctx := structured.Wrap(data.Ctx)
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
					encoder: sign(eat.NewClientSigned(sid, lid, qid).
						WithGrant(eat.Grants.Read).
						WithCtx(map[string]interface{}{"additional": "context"}),
						clientSK),
					wantType: eat.Types.ClientSigned(),
					validate: func(t *testing.T, data *eat.Token) {
						clientSignedDefault(t, data)
						require.Equal(t, qid, data.QID)
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
					// fmt.Println(jsonutil.MarshalString(tok.TokenData))
					jsn, err := tok.TokenData.EncodeJSON()
					require.NoError(t, err)
					fmt.Println(jsonutil.MustPretty(string(jsn)))
					fmt.Println(eat.Describe(encoded))
				})
			}
		})
	}
}

func stateChannelDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, sub, data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}

func stateChannelNoSubject(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, "", data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}

func txDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, txh, data.EthTxHash)
}

func plainDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
}

func anonymousDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
}

func editorSignedDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, ethutil.AddressToID(data.EthAddr, id.User).String(), data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}

func signedLinkDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, ethutil.AddressToID(data.EthAddr, id.User).String(), data.Subject)
	ctx := structured.Wrap(data.Ctx).At("elv")
	require.NotZero(t, ctx.At("lnk").String())
	require.NotZero(t, ctx.At("src").String())
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}

func clientDefault(t *testing.T, data *eat.Token) {
	stateChannelDefault(t, data.Embedded)
}

func clientSignedDefault(t *testing.T, data *eat.Token) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, ethutil.AddressToID(data.EthAddr, id.User).String(), data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}
