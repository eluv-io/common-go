//go:build testing

// testing flag, because it uses the test assets

package mpegts_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media"
	"github.com/eluv-io/common-go/media/mpegts"
	"github.com/eluv-io/common-go/media/tlv"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/testutil"
	"github.com/eluv-io/log-go"
)

func TestTsStreamTracker(t *testing.T) {
	tests := []struct {
		stripRtp    bool             // whether to strip rtp headers
		source      string           // the source filename
		packetizer  media.Packetizer // the packetizer to use
		wantStreams int              // expected mpeg streams count
	}{
		{
			stripRtp:    false,
			source:      "ts-segment.ts",
			packetizer:  mpegts.NewTsPacketizer(true, mpegts.TsSyncModes.Modulo()),
			wantStreams: 5,
		},
		{
			stripRtp:    true,
			source:      "tlv-rtp-ts-segment-00001.ts",
			packetizer:  tlv.NewTlvPacketizer(2 * 1500),
			wantStreams: 12,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint("stripRtp", tt.stripRtp), func(t *testing.T) {
			path, err := testutil.AssetsPath(2)
			if err != nil {
				t.Skip("skipping test: ", err)
			}
			source, err := os.ReadFile(filepath.Join(path, "media", "mpeg-ts", tt.source))
			require.NoError(t, err)

			for _, packetLoss := range []float64{0, .001} {
				t.Run(fmt.Sprint("packet-loss:", packetLoss), func(t *testing.T) {
					tracker := mpegts.NewTsStreamTracker("", 5*time.Second, tt.stripRtp)
					pacer := mpegts.NewTsPacer().WithStripRtp(tt.stripRtp)
					tt.packetizer.Write(source)
					for {
						pkt, err := tt.packetizer.Next()
						require.NoError(t, err)
						if pkt == nil {
							break
						}
						if false {
							pacer.Wait(pkt)
						}
						if packetLoss > 0 {
							if rand.Float64() <= packetLoss {
								continue
							}
							_, err := tracker.Track(pkt)
							if err != nil {
								log.Info("packet validation", err)
							}
						} else {
							_, err := tracker.Track(pkt)
							require.NoError(t, err)
						}
					}

					stats := tracker.Stats()
					log.Info("tracker", "stats", jsonutil.MarshalString(stats))

					if packetLoss > 0 {
						require.Greater(t, stats.ErrorCount, 0)
					} else {
						require.Equal(t, 0, stats.ErrorCount)
					}
					require.Len(t, stats.Streams, tt.wantStreams)
				})
			}
		})
	}
}
