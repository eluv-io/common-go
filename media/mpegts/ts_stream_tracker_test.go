//go:build testing

// testing flag, because it uses the test assets

package mpegts_test

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	pionrtp "github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media"
	"github.com/eluv-io/common-go/media/mpegts"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/media/tlv"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/testutil"
	"github.com/eluv-io/log-go"
)

func TestTsStreamTracker(t *testing.T) {
	tests := []struct {
		name        string
		framing     mpegts.TsFraming
		source      string
		packetizer  media.Packetizer
		transform   func(pkt []byte) ([]byte, error) // optional per-packet transform applied before tracking
		wantStreams int                              // expected mpeg streams count
	}{
		{
			name:        "raw",
			framing:     mpegts.TsFramingNone,
			source:      "ts-segment.ts",
			packetizer:  mpegts.NewTsPacketizer(true, mpegts.TsSyncModes.Modulo()),
			wantStreams: 5,
		},
		{
			name:        "rtp",
			framing:     mpegts.TsFramingRtp,
			source:      "tlv-rtp-ts-segment-00001.ts",
			packetizer:  tlv.NewTlvPacketizer(2 * 1500),
			wantStreams: 12,
		},
		{
			// Derived from the RTP-TS segment by re-wrapping each packet as ATS-TS, using the RTP timestamp as the
			// arrival time. The underlying TS payload is identical to the "rtp" case, so it must yield the same streams
			// and error count.
			name:        "ats",
			framing:     mpegts.TsFramingAts,
			source:      "tlv-rtp-ts-segment-00001.ts",
			packetizer:  tlv.NewTlvPacketizer(2 * 1500),
			transform:   rtpValueToAtsTs,
			wantStreams: 12,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := testutil.AssetsPath(2)
			if err != nil {
				t.Skip("skipping test: ", err)
			}
			source, err := os.ReadFile(filepath.Join(path, "media", "mpeg-ts", tt.source))
			require.NoError(t, err)

			for _, packetLoss := range []float64{0, .005} {
				t.Run(fmt.Sprint("packet-loss:", packetLoss), func(t *testing.T) {
					tracker := mpegts.NewTsStreamTrackerFramed("", 5*time.Second, tt.framing)
					pacer := mpegts.NewTsPacer().WithStripRtp(tt.framing == mpegts.TsFramingRtp)
					tt.packetizer.Write(source)
					for {
						pkt, err := tt.packetizer.Next()
						require.NoError(t, err)
						if pkt == nil {
							break
						}
						if tt.transform != nil {
							pkt, err = tt.transform(pkt)
							require.NoError(t, err)
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

// rtpValueToAtsTs re-wraps an RTP-TS value (an RTP packet carrying MPEG-TS) as an ATS-TS value: it strips the RTP
// header and prefixes the raw TS payload with an 8-byte big-endian arrival timestamp derived from the RTP timestamp (90
// kHz clock converted to nanoseconds). This lets the ATS-TS tracker path be exercised against the existing RTP-TS
// asset.
func rtpValueToAtsTs(rtpValue []byte) ([]byte, error) {
	var pkt pionrtp.Packet
	if err := pkt.Unmarshal(rtpValue); err != nil {
		return nil, err
	}
	tsPayload, err := rtp.StripHeader(rtpValue)
	if err != nil {
		return nil, err
	}
	arrivalNs := int64(rtp.TicksToDuration(int64(pkt.Timestamp)))
	out := make([]byte, mpegts.AtsTimestampLen+len(tsPayload))
	binary.BigEndian.PutUint64(out[:mpegts.AtsTimestampLen], uint64(arrivalNs))
	copy(out[mpegts.AtsTimestampLen:], tsPayload)
	return out, nil
}
