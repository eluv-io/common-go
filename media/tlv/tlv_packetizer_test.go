//go:build testing

// testing flag, because it uses the test assets

package tlv_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/tlv"
	"github.com/eluv-io/common-go/util/testutil"
)

const (
	tlvHeaderSize = 3
	rtpHeaderSize = 12
	tsPacketSize  = 188
)

func TestTlvPacketizer(t *testing.T) {
	path, err := testutil.AssetsPath(2)
	if err != nil {
		t.Skip("skipping test: ", err)
	}
	source, err := os.ReadFile(filepath.Join(path, "media", "mpeg-ts", "tlv-rtp-ts-segment-00001.ts"))
	require.NoError(t, err)

	fullPacketCount := 0
	t.Run("valid packet", func(t *testing.T) {
		byteCount := 0
		consumed := 0
		packetizer := tlv.NewTlvPacketizer(1500)

		chunkSize := len(source)
		for i := 0; i < len(source); i += chunkSize {
			packetizer.Write(source[i:min(i+chunkSize, len(source))])
			for pkt, err := packetizer.Next(); len(pkt) > 0; pkt, err = packetizer.Next() {
				require.NoError(t, err)
				consumed += packetizer.Consumed()
				require.Equal(t, rtpHeaderSize+7*tsPacketSize, len(pkt))
				byteCount += len(pkt)
				pkt = pkt[rtpHeaderSize:]
				for j := 0; j < 7; j++ {
					tsPkt := packet.Packet(pkt[:tsPacketSize])
					require.NoError(t, tsPkt.CheckErrors())
					pkt = pkt[tsPacketSize:]
				}
				fullPacketCount++
			}
		}
		fmt.Println("packets:", fullPacketCount)
		require.Equal(t, len(source), byteCount+fullPacketCount*tlvHeaderSize)
		require.Equal(t, consumed, byteCount+fullPacketCount*tlvHeaderSize)
	})
}
