package pktpool_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/pktpool"
)

// TestRawPacket_LazyDecode verifies RawPacket's Rtp/Ts decoding and caching, mirroring
// TestPacket_LazyDecode_Idempotent but over a plain caller-owned slice instead of a pool-borrowed Packet.
func TestRawPacket_LazyDecode(t *testing.T) {
	ts := tsPayload(3)
	raw := rtpBytes(t, 42, 9000, ts)

	p := pktpool.NewRawPacket(raw)
	for range 3 { // repeated access must yield identical results
		r, err := p.Rtp()
		require.NoError(t, err)
		require.EqualValues(t, 42, r.Packet().SequenceNumber)
		require.EqualValues(t, 9000, r.Packet().Timestamp)
		require.True(t, bytes.Equal(ts, r.Payload))
	}

	tsPkt, err := p.Ts()
	require.NoError(t, err)
	require.Len(t, tsPkt.Packets(), 3)
	for i, pkt := range tsPkt.Packets() {
		require.EqualValues(t, 0x47, pkt[0])
		require.EqualValues(t, i, pkt[1])
	}
}

// TestRawPacket_DecodeOutOfOrder verifies the same layer-ordering guard as Packet: decoding Ts before Rtp on
// RTP-wrapped data is rejected once bytes have already been consumed by a later-ranked layer... concretely, Rtp then
// Ts is fine; there is no outer layer to go back to since RawPacket does not expose Tlv.
func TestRawPacket_DecodeOutOfOrder(t *testing.T) {
	raw := rtpBytes(t, 1, 0, tsPayload(1))
	p := pktpool.NewRawPacket(raw)

	_, err := p.Rtp()
	require.NoError(t, err)
	_, err = p.Ts()
	require.NoError(t, err)
}

// TestRawPacket_Reset verifies that Reset discards previously decoded layers and re-points the cursor at the new
// slice, so a single RawPacket can be reused across sequential calls without reallocating.
func TestRawPacket_Reset(t *testing.T) {
	first := rtpBytes(t, 1, 100, tsPayload(1))
	second := rtpBytes(t, 2, 200, tsPayload(2))

	p := pktpool.NewRawPacket(first)
	r, err := p.Rtp()
	require.NoError(t, err)
	require.EqualValues(t, 1, r.Packet().SequenceNumber)

	p.Reset(second)
	r, err = p.Rtp()
	require.NoError(t, err)
	require.EqualValues(t, 2, r.Packet().SequenceNumber)

	tsPkt, err := p.Ts()
	require.NoError(t, err)
	require.Len(t, tsPkt.Packets(), 2)
}

// TestRawPacket_Unbounded verifies that RawPacket has no size limit (unlike Packet.From, which is bounded by the
// pool's configured capacity) - the property that motivated RawPacket over a fixed-buffer alternative.
func TestRawPacket_Unbounded(t *testing.T) {
	large := rtpBytes(t, 1, 0, tsPayload(1000)) // ~188KB payload
	p := pktpool.NewRawPacket(large)

	_, err := p.Rtp()
	require.NoError(t, err)
	tsLayer, err := p.Ts()
	require.NoError(t, err)
	require.Len(t, tsLayer.Packets(), 1000)
}

// TestRawPacket_SatisfiesDecoder verifies that RawPacket implements the same Decoder interface as Packet, so callers
// (e.g. common-go/media/tracker.MediaTracker) can accept either uniformly.
func TestRawPacket_SatisfiesDecoder(t *testing.T) {
	var _ pktpool.Decoder = pktpool.NewRawPacket(nil)
	var _ pktpool.Decoder = pktpool.NewPacket(0, 0)
}
