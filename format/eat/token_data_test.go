package eat_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/constants"
	"github.com/qluvio/content-fabric/format"
	"github.com/qluvio/content-fabric/format/eat"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/byteutil"
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
				"key1":             "val1",
				"key2":             "val2",
				constants.ElvIPGeo: "eu-west",
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
