package eat_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/jsonutil"

	"github.com/qluvio/content-fabric/format/utc"

	"github.com/qluvio/content-fabric/format/eat"
)

var sub = "token subject"

func TestTokenBuilders(t *testing.T) {
	now := utc.Now().Truncate(time.Second) // times in tokens have second precision
	anHourAgo := now.Add(-time.Hour)
	anHourFromNow := now.Add(time.Hour)
	defer utc.MockNow(now)()

	ctx1 := map[string]interface{}{
		"k1": "v1",
		"k2": "v2",
	}
	tests := []struct {
		encoder     eat.Encoder
		wantFailure bool
		wantType    eat.TokenType
		validate    func(t *testing.T, data *eat.TokenData)
	}{

		{
			encoder:  eat.NewStateChannel(sid, lid, qid, sub).Sign(serverSK),
			wantType: eat.Types.StateChannel(),
			validate: stateChannelDefault,
		},
		{
			encoder:     eat.NewStateChannel(sid, lid, qid, "").Sign(serverSK),
			wantFailure: true, // no subject and no ctx
			wantType:    eat.Types.Unknown(),
		},
		{
			encoder: eat.NewStateChannel(sid, lid, qid, "").
				WithCtx(ctx1).
				Sign(serverSK),
			wantFailure: false, // no subject but ctx
			wantType:    eat.Types.StateChannel(),
			validate:    stateChannelNoSubject,
		},
		{
			encoder: eat.NewStateChannel(sid, lid, qid, sub).
				WithAfgh("afgh-pk").
				Sign(serverSK),
			wantType: eat.Types.StateChannel(),
			validate: func(t *testing.T, data *eat.TokenData) {
				stateChannelDefault(t, data)
				require.Equal(t, "afgh-pk", data.AFGHPublicKey)
				require.Equal(t, now, data.IssuedAt)
			},
		},
		{
			encoder: eat.NewStateChannel(sid, lid, qid, sub).
				WithAfgh("afgh-pk").
				WithCtx(ctx1).
				WithIssuedAt(anHourAgo).
				WithExpires(anHourFromNow).
				Sign(serverSK),
			wantType: eat.Types.StateChannel(),
			validate: func(t *testing.T, data *eat.TokenData) {
				stateChannelDefault(t, data)
				require.Equal(t, "afgh-pk", data.AFGHPublicKey)
				require.Equal(t, anHourAgo, data.IssuedAt)
				require.Equal(t, anHourFromNow, data.Expires)
				require.Equal(t, ctx1, data.Ctx)
			},
		},
		{
			encoder:  eat.NewTx(sid, lid, txh).Sign(clientSK),
			wantType: eat.Types.Tx(),
			validate: txDefault,
		},
		{
			encoder:  eat.NewTx(sid, lid, txh).WithAfgh("afgh-pk").Sign(clientSK),
			wantType: eat.Types.Tx(),
			validate: func(t *testing.T, data *eat.TokenData) {
				txDefault(t, data)
				require.Equal(t, "afgh-pk", data.AFGHPublicKey)
			},
		},
		{
			encoder:  eat.NewPlain(sid, lid).Sign(clientSK),
			wantType: eat.Types.Plain(),
			validate: plainDefault,
		},
		{
			encoder:  eat.NewPlain(sid, lid).WithQID(qid).Sign(clientSK),
			wantType: eat.Types.Plain(),
			validate: func(t *testing.T, data *eat.TokenData) {
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
			validate: func(t *testing.T, data *eat.TokenData) {
				anonymousDefault(t, data)
				require.Equal(t, qid, data.QID)
			},
		},
		{
			encoder: eat.NewEditorSigned(sid, lid, qid).
				WithGrant(eat.Grants.Read).
				WithCtx(map[string]interface{}{"additional": "context"}).
				Sign(clientSK),
			wantType: eat.Types.EditorSigned(),
			validate: func(t *testing.T, data *eat.TokenData) {
				editorSignedDefault(t, data)
				require.Equal(t, qid, data.QID)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.wantType.String(), func(t *testing.T) {
			encoded, err := test.encoder.Encode()
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
			test.validate(t, &tok.TokenData)
			// fmt.Println(jsonutil.MarshalString(tok.TokenData))
			jsn, err := tok.TokenData.EncodeJSON()
			require.NoError(t, err)
			fmt.Println(jsonutil.MustPretty(string(jsn)))
		})
	}
}

func stateChannelDefault(t *testing.T, data *eat.TokenData) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, sub, data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}

func stateChannelNoSubject(t *testing.T, data *eat.TokenData) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, "", data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}

func txDefault(t *testing.T, data *eat.TokenData) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, txh, data.EthTxHash)
}

func plainDefault(t *testing.T, data *eat.TokenData) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
}

func anonymousDefault(t *testing.T, data *eat.TokenData) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
}

func editorSignedDefault(t *testing.T, data *eat.TokenData) {
	require.Equal(t, sid, data.SID)
	require.Equal(t, lid, data.LID)
	require.Equal(t, qid, data.QID)
	require.Equal(t, data.EthAddr.Hex(), data.Subject)
	require.NotEqual(t, utc.Zero, data.IssuedAt)
}
