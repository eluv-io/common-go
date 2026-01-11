//go:build testing

// testing flag, because it uses the test assets

package mpegts

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/testutil"
)

func TestPacketizer(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(testutil.AssetsPathT(t, 2), "media", "mpeg-ts", "ts-segment.ts"))
	if err != nil {
		t.Skip("skipping test: ", err)
	}

	fullPacketCount := 0
	t.Run("start on TS packet boundary", func(t *testing.T) {
		byteCount := 0

		packetizer := NewTsPacketizer(true, TsSyncModes.Once())
		packetizer.Write(source)
		for pkt, err := packetizer.Next(); len(pkt) > 0; pkt, err = packetizer.Next() {
			require.NoError(t, err)
			require.Equal(t, 7*packet.PacketSize, len(pkt))
			tsPkt := packet.Packet(pkt)
			require.NoError(t, tsPkt.CheckErrors())
			fullPacketCount++
			byteCount += len(pkt)
		}
		require.Equal(t, len(source), byteCount) // NOTE: source size happens to be multiple of 7 * 188!
		fmt.Println("packets:", fullPacketCount)
	})

	t.Run("start on TS packet boundary offset", func(t *testing.T) {
		for offset := 1; offset < 188; offset += 11 { // testing all offsets takes more time...
			packetCount := 0
			byteCount := 0

			packetizer := NewTsPacketizer(true, TsSyncModes.Once())
			packetizer.Write(source[offset:])
			for pkt, err := packetizer.Next(); len(pkt) > 0; pkt, err = packetizer.Next() {
				require.NoError(t, err)
				packetCount++
				if packetCount == fullPacketCount {
					// last packet is smaller, since we skipped the first TS packet in the first UDP packet
					require.Equal(t, 6*packet.PacketSize, len(pkt), "count=%d", packetCount)
				} else {
					require.Equal(t, 7*packet.PacketSize, len(pkt), "count=%d", packetCount)
				}
				tsPkt := packet.Packet(pkt)
				require.NoError(t, tsPkt.CheckErrors())
				byteCount += len(pkt)
			}
			fmt.Println("packets:", packetCount)
			require.Equal(t, fullPacketCount-1, packetCount) // -1 because we skipped the first TS packet
			require.Equal(t, len(source)-7*packet.PacketSize, byteCount)
		}
	})
}
