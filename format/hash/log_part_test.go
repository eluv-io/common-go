package hash_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/utc-go"
)

func TestLogPartHash(t *testing.T) {
	require.True(t, hash.QPartLog.IsLive())
	require.False(t, hash.QPartLog.PersistsOnFinalize())
	require.False(t, hash.QPartLiveTransient.PersistsOnFinalize())
	require.True(t, hash.QPartLive.PersistsOnFinalize())
	require.False(t, hash.QPart.PersistsOnFinalize())

	digest := sha256.Sum256([]byte("stream-1/1/0"))
	expiration := utc.Now().Add(30 * 24 * time.Hour)
	for _, format := range []hash.Format{hash.Unencrypted, hash.AES128AFGH} {
		htype := hash.Type{Code: hash.QPartLog, Format: format}
		h, err := hash.NewLive(htype, digest[:], expiration)
		require.NoError(t, err)
		require.True(t, h.IsLive())
		require.NotEmpty(t, htype.Describe())

		s := h.String()
		require.Contains(t, []string{"hqg_", "hqge"}, s[:4])
		parsed, err := hash.QPartLog.FromString(s)
		require.NoError(t, err)
		require.Equal(t, h.String(), parsed.String())
		require.Equal(t, expiration.Truncate(time.Second), parsed.Expiration)

		_, err = hash.NewPart(htype, digest[:], 1, 0)
		require.Error(t, err, "a log part hash is live")
	}
}
