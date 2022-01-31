package eat_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/eluv-io/utc-go"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format"
	"github.com/eluv-io/common-go/format/eat"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/common-go/util/byteutil"
)

var qph = func() types.QPHash {
	digest := format.NewFactory().NewContentPartDigest(hash.Unencrypted)
	_, _ = digest.Write(byteutil.RandomBytes(10))
	return digest.AsHash()
}()

func TestTokenDataJSON(t *testing.T) {
	zero := eat.TokenData{}
	tokens := []eat.TokenData{
		zero,
		{
			EthTxHash:     txh,
			EthAddr:       clientAddr,
			AFGHPublicKey: "afgh",
			QPHash:        qph,
			SID:           sid,
			LID:           lid,
			QID:           qid,
			Grant:         "read",
			IssuedAt:      utc.Now().Truncate(time.Millisecond),
			Expires:       utc.Now().Truncate(time.Millisecond).Add(time.Hour),
			Ctx: map[string]interface{}{
				"key1":       "val1",
				"key2":       "val2",
				eat.ElvIPGeo: "eu-west",
			},
		},
	}

	for _, td := range tokens {
		marshalled, err := td.EncodeJSON()
		require.NoError(t, err)

		fmt.Println(string(marshalled))

		if reflect.DeepEqual(td, zero) {
			// require the zero value to marshal to an empty JSON object
			require.Equal(t, "{}", string(marshalled))
		}

		var unmarshalled eat.TokenData
		err = unmarshalled.DecodeJSON(marshalled)
		require.NoError(t, err)

		require.Equal(t, td, unmarshalled)
	}
}
