package rtp

import (
	"testing"

	pionrtp "github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestParseRtpHeaderMinimal(t *testing.T) {
	tests := []struct {
		name string
		seq  uint16
		ts   uint32
	}{
		{"basic", 0, 0},
		{"max_values", 65535, 4294967295},
		{"random", 12345, 987654321},
		{"mid_range", 32768, 2147483648},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create packet with pion library
			pkt := &pionrtp.Packet{
				Header: pionrtp.Header{
					Version:        2,
					SequenceNumber: tt.seq,
					Timestamp:      tt.ts,
					SSRC:           12345,
					PayloadType:    96,
				},
				Payload: make([]byte, 100),
			}
			bts, err := pkt.Marshal()
			require.NoError(t, err)

			// Parse with minimal parser
			hdr, err := parseRtpHeaderMinimal(bts)
			require.NoError(t, err)
			require.Equal(t, tt.seq, hdr.sequenceNumber, "sequence number mismatch")
			require.Equal(t, tt.ts, hdr.timestamp, "timestamp mismatch")
		})
	}
}

func TestParseRtpHeaderMinimal_Errors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too_short", []byte{0, 1, 2, 3}},
		{"invalid_version_0", []byte{0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"invalid_version_1", []byte{0x40, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{"invalid_version_3", []byte{0xC0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRtpHeaderMinimal(tt.data)
			require.Error(t, err, "expected error for %s", tt.name)
		})
	}
}

func TestParseRtpHeaderMinimal_ValidVersion(t *testing.T) {
	// Valid RTP version 2 packet
	validPkt := []byte{0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	_, err := parseRtpHeaderMinimal(validPkt)
	require.NoError(t, err)
}

func TestParseRtpHeaderMinimal_CompareWithFullParser(t *testing.T) {
	// Generate several test packets and compare minimal parser with full parser
	for i := uint16(0); i < 1000; i += 100 {
		for j := uint32(0); j < 1000000; j += 100000 {
			pkt := &pionrtp.Packet{
				Header: pionrtp.Header{
					Version:        2,
					SequenceNumber: i,
					Timestamp:      j,
					SSRC:           12345,
					PayloadType:    96,
				},
				Payload: make([]byte, 100),
			}
			bts, err := pkt.Marshal()
			require.NoError(t, err)

			// Parse with minimal parser
			hdrMin, err := parseRtpHeaderMinimal(bts)
			require.NoError(t, err)

			// Parse with full parser
			hdrFull, err := ParsePacket(bts)
			require.NoError(t, err)

			// Compare results
			require.Equal(t, hdrFull.SequenceNumber, hdrMin.sequenceNumber,
				"seq mismatch for i=%d, j=%d", i, j)
			require.Equal(t, hdrFull.Timestamp, hdrMin.timestamp,
				"ts mismatch for i=%d, j=%d", i, j)
		}
	}
}
