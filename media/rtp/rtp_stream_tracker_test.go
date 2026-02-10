package rtp_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/media/tlv"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/testutil"
)

func TestRtpStreamTracker(t *testing.T) {
	path, err := testutil.AssetsPath(2)
	if err != nil {
		t.Skip("skipping test: ", err)
	}
	source, err := os.ReadFile(filepath.Join(path, "media", "mpeg-ts", "tlv-rtp-ts-segment-00001.ts"))
	require.NoError(t, err)

	t.Run("valid packet", func(t *testing.T) {
		packetizer := tlv.NewTlvPacketizer(1500)
		tracker := rtp.NewStreamTracker("test", 0, 1, time.Second)

		chunkSize := len(source)
		for i := 0; i < len(source); i += chunkSize {
			packetizer.Write(source[i:min(i+chunkSize, len(source))])
			for pkt, err := packetizer.Next(); len(pkt) > 0; pkt, err = packetizer.Next() {
				require.NoError(t, err)
				tracker.Track(pkt)
			}
		}
		stats := tracker.Stats()
		fmt.Println("stats", "stats", jsonutil.MarshalString(stats))
		require.Equal(t, 1596, stats.PacketCount)
		require.Equal(t, 0, stats.ErrorCount)
		require.Empty(t, stats.Gaps)
	})
	t.Run("gap", func(t *testing.T) {
		packetizer := tlv.NewTlvPacketizer(1500)
		tracker := rtp.NewStreamTracker("test", 0, 1, time.Second)

		for i := 0; i < 5; i++ {
			packetizer.Write(source)
			first := true
			for pkt, err := packetizer.Next(); len(pkt) > 0; pkt, err = packetizer.Next() {
				require.NoError(t, err)
				_, err := tracker.Track(pkt)
				if i > 0 && first {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				first = false
			}
		}
		stats := tracker.Stats()
		fmt.Println("stats", "stats", jsonutil.MarshalString(stats))
		require.Equal(t, 5*1596, stats.PacketCount)
		require.Equal(t, 4, stats.ErrorCount)
		require.Len(t, stats.Gaps, 4)
	})
}
