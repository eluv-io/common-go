package pktpool_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/pktpool"
)

// tsPayload builds n synthetic 188-byte MPEG-TS packets (sync byte + filler).
func tsPayload(n int) []byte {
	data := make([]byte, n*188)
	for i := range n {
		data[i*188] = 0x47 // TS sync byte
		data[i*188+1] = byte(i)
	}
	return data
}

// rtpBytes marshals an RTP packet with the given payload.
func rtpBytes(tb testing.TB, seq uint16, ts uint32, payload []byte) []byte {
	pkt := rtp.Packet{
		Header:  rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: ts, SSRC: 0x1234},
		Payload: payload,
	}
	bts, err := pkt.Marshal()
	require.NoError(tb, err)
	return bts
}

func TestPacket_From_Overflow(t *testing.T) {
	pool := pktpool.NewPacketPool(0, 16)
	pkt := pool.Borrow()
	defer pkt.Release()

	require.NoError(t, pkt.T.From([]byte("0123456789")))
	require.Len(t, pkt.T.Data, 10)

	// A failed load must leave the packet empty, not exposing stale Data from the previous successful load.
	require.Error(t, pkt.T.From(make([]byte, 17))) // does not fit -> error, not silent truncation
	require.Empty(t, pkt.T.Data)
}

// TestPacket_FromReader_EmptyRead verifies that a read returning no bytes leaves the packet empty rather than retaining
// stale Data from a previous load.
func TestPacket_FromReader_EmptyRead(t *testing.T) {
	pool := pktpool.NewPacketPool(0, 64)
	pkt := pool.Borrow()
	defer pkt.Release()

	require.NoError(t, pkt.T.From([]byte("stale")))
	require.Len(t, pkt.T.Data, 5)

	require.NoError(t, pkt.T.FromReader(emptyReader{})) // n == 0, err == nil
	require.Empty(t, pkt.T.Data)
}

// emptyReader is an io.Reader that reads no bytes without erroring (returns 0, nil).
type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, nil }

// TestPacket_LazyDecode_Idempotent verifies that decoding a layer is cached and that repeated access does not corrupt
// the decode cursor (regression for the missing parsed=true flag).
func TestPacket_LazyDecode_Idempotent(t *testing.T) {
	ts := tsPayload(3)
	raw := rtpBytes(t, 42, 9000, ts)

	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()
	require.NoError(t, pkt.T.From(raw))

	for range 3 { // repeated access must yield identical results
		r, err := pkt.T.Rtp()
		require.NoError(t, err)
		require.EqualValues(t, 42, r.Packet().SequenceNumber)
		require.EqualValues(t, 9000, r.Packet().Timestamp)
		require.True(t, bytes.Equal(ts, r.Payload))
	}

	// inner TS layer decodes from the post-RTP cursor
	tsPkt, err := pkt.T.Ts()
	require.NoError(t, err)
	require.Len(t, tsPkt.Packets(), 3)
	for i, p := range tsPkt.Packets() {
		require.EqualValues(t, 0x47, p[0])
		require.EqualValues(t, i, p[1])
	}

	// Data still reflects the full original packet, untouched by decoding.
	require.True(t, bytes.Equal(raw, pkt.T.Data))
}

// TestPacket_DecodeOutOfOrder verifies the backward-ordering guard: decoding an outer layer after an inner one has
// consumed bytes is rejected, while skipping absent outer layers (raw RTP, then TS) is allowed.
func TestPacket_DecodeOutOfOrder(t *testing.T) {
	raw := rtpBytes(t, 1, 0, tsPayload(1))
	pool := pktpool.NewPacketPool(0, 2048)

	// raw RTP (no TLV): Rtp first is fine, then the inner TS layer.
	pkt := pool.Borrow()
	require.NoError(t, pkt.T.From(raw))
	_, err := pkt.T.Rtp()
	require.NoError(t, err)
	_, err = pkt.T.Ts()
	require.NoError(t, err)
	// going back to an outer layer (TLV) after RTP/TS were decoded is rejected
	_, err = pkt.T.Tlv()
	require.Error(t, err)
	pkt.Release()
}

func TestPacket_WrapTlv_RoundTrip(t *testing.T) {
	pool := pktpool.NewPacketPool(3, 64) // 3 bytes head room for the TLV header
	pkt := pool.Borrow()
	defer pkt.Release()

	payload := []byte("hello world")
	require.NoError(t, pkt.T.From(payload))
	require.NoError(t, pkt.T.WrapTlv(0x07))

	tlv, err := pkt.T.Tlv()
	require.NoError(t, err)
	require.EqualValues(t, 0x07, tlv.Type())
	require.EqualValues(t, len(payload), tlv.Size())
	require.Equal(t, pkt.T.Data, tlv.Data())
	require.True(t, bytes.Equal(payload, tlv.Payload))

	// Data and Write must both reflect the prepended header.
	require.Len(t, pkt.T.Data, 3+len(payload))
	var buf bytes.Buffer
	require.NoError(t, pkt.T.Write(&buf))
	require.Equal(t, pkt.T.Data, buf.Bytes())
	require.True(t, bytes.Equal(payload, buf.Bytes()[3:]))
}

// TestPacket_ReuseAfterWrapTlv verifies that From restores the load offset consumed by a prior WrapTlv, so a packet can
// be reused without going back through the pool (regression for F1: off not reset).
func TestPacket_ReuseAfterWrapTlv(t *testing.T) {
	pool := pktpool.NewPacketPool(3, 64)
	pkt := pool.Borrow()
	defer pkt.Release()

	require.NoError(t, pkt.T.From([]byte("first")))
	require.NoError(t, pkt.T.WrapTlv(0x07)) // consumes 3 bytes of head room (off -= 3)
	require.Len(t, pkt.T.Data, 3+len("first"))

	// reuse the same packet without re-borrowing: From must reset off back to the head room boundary
	require.NoError(t, pkt.T.From([]byte("second")))
	require.Equal(t, []byte("second"), pkt.T.Data)

	// the 3 bytes of head room must be available again for another WrapTlv
	require.NoError(t, pkt.T.WrapTlv(0x09))
	require.Len(t, pkt.T.Data, 3+len("second"))
	require.EqualValues(t, 0x09, pkt.T.Data[0])
	require.EqualValues(t, len("second"), int(pkt.T.Data[1])<<8|int(pkt.T.Data[2]))
}

// TestPacket_WrapTlv_Retype verifies that re-wrapping an already-wrapped packet updates the type byte in both the
// in-memory layer and the serialized buffer (regression for F2).
func TestPacket_WrapTlv_Retype(t *testing.T) {
	pool := pktpool.NewPacketPool(3, 64)
	pkt := pool.Borrow()
	defer pkt.Release()

	require.NoError(t, pkt.T.From([]byte("payload")))
	require.NoError(t, pkt.T.WrapTlv(0x07))
	require.NoError(t, pkt.T.WrapTlv(0x42)) // retype in place; must not prepend a second header

	require.Len(t, pkt.T.Data, 3+len("payload")) // still a single header

	tlv, err := pkt.T.Tlv()
	require.NoError(t, err)
	require.EqualValues(t, 0x42, tlv.Type()) // in-memory layer updated

	var buf bytes.Buffer
	require.NoError(t, pkt.T.Write(&buf))
	require.EqualValues(t, 0x42, buf.Bytes()[0]) // serialized buffer updated too
}

func TestPacket_TlvDecode_UsesLengthForInnerCursor(t *testing.T) {
	ts := tsPayload(1)
	raw := rtpBytes(t, 1, 0, ts)
	wrapped := append([]byte{0x07, byte(len(raw) >> 8), byte(len(raw))}, raw...)
	wrapped = append(wrapped, 0xff) // not part of the TLV value

	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()
	require.NoError(t, pkt.T.From(wrapped))

	tlv, err := pkt.T.Tlv()
	require.NoError(t, err)
	require.EqualValues(t, 0x07, tlv.Type())
	require.EqualValues(t, len(raw), tlv.Size())
	require.True(t, bytes.Equal(raw, tlv.Payload))

	rtp, err := pkt.T.Rtp()
	require.NoError(t, err)
	require.True(t, bytes.Equal(ts, rtp.Payload))

	tsPkt, err := pkt.T.Ts()
	require.NoError(t, err)
	require.Len(t, tsPkt.Packets(), 1)
}

func TestPacket_WrapTlv_NoHeadRoom(t *testing.T) {
	pool := pktpool.NewPacketPool(0, 64) // no head room
	pkt := pool.Borrow()
	defer pkt.Release()
	require.NoError(t, pkt.T.From([]byte("data")))
	require.Error(t, pkt.T.WrapTlv(0x07))
}

// TestPacket_FrameTlv verifies that FrameTlv returns a TLV-wrapped view without a prefix (e.g. arrival timestamp) and,
// crucially, does not mutate the packet: Data and the decode cursor are unchanged, so the packet remains usable by
// concurrent readers.
func TestPacket_FrameTlv(t *testing.T) {
	pool := pktpool.NewPacketPool(3, 64)
	pkt := pool.Borrow()
	defer pkt.Release()

	payload := []byte("hello world")
	require.NoError(t, pkt.T.From(payload))

	framed, err := pkt.T.FrameTlv(0x07, nil)
	require.NoError(t, err)
	require.Len(t, framed, 3+len(payload))
	require.EqualValues(t, 0x07, framed[0])
	require.EqualValues(t, len(payload), int(framed[1])<<8|int(framed[2]))
	require.Equal(t, payload, framed[3:])

	// FrameTlv must not touch the packet's own view.
	require.Equal(t, payload, pkt.T.Data)
}

// TestPacket_FrameTlv_WithPrefix mirrors the ATS-TS output framing: an 8-byte prefix between the TLV header and the
// payload. It verifies the head room accounting (wrapCap = header + prefix) and the encoded value length.
func TestPacket_FrameTlv_WithPrefix(t *testing.T) {
	const headerLen, prefixLen = 3, 8
	pool := pktpool.NewPacketPool(headerLen+prefixLen, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()

	payload := tsPayload(2)
	require.NoError(t, pkt.T.From(payload))

	prefix := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	framed, err := pkt.T.FrameTlv(0x04, prefix)
	require.NoError(t, err)
	require.Len(t, framed, headerLen+prefixLen+len(payload))
	require.EqualValues(t, 0x04, framed[0])
	require.EqualValues(t, prefixLen+len(payload), int(framed[1])<<8|int(framed[2]))
	require.Equal(t, prefix, framed[headerLen:headerLen+prefixLen])
	require.Equal(t, payload, framed[headerLen+prefixLen:])
	require.Equal(t, payload, pkt.T.Data) // unchanged
}

// TestPacket_FrameTlv_NoHeadRoom verifies FrameTlv fails (rather than corrupting the buffer) when the head room cannot
// hold the header plus prefix.
func TestPacket_FrameTlv_NoHeadRoom(t *testing.T) {
	pool := pktpool.NewPacketPool(3, 64) // room for the 3-byte header but not an extra prefix
	pkt := pool.Borrow()
	defer pkt.Release()
	require.NoError(t, pkt.T.From([]byte("data")))

	_, err := pkt.T.FrameTlv(0x07, nil)
	require.NoError(t, err) // header fits

	_, err = pkt.T.FrameTlv(0x04, []byte{1, 2, 3}) // header + 3-byte prefix does not
	require.Error(t, err)
}

// TestPacket_FrameTlv_Idempotent verifies FrameTlv can be called repeatedly and that it overwrites any existing the
// existing head room.
func TestPacket_FrameTlv_Idempotent(t *testing.T) {
	pool := pktpool.NewPacketPool(3, 64)
	pkt := pool.Borrow()
	defer pkt.Release()
	require.NoError(t, pkt.T.From([]byte("payload")))

	a, err := pkt.T.FrameTlv(0x07, nil)
	require.NoError(t, err)
	b, err := pkt.T.FrameTlv(0x03, nil)
	require.NoError(t, err)
	require.EqualValues(t, 0x03, b[0]) // second call rewrites the same head room
	require.Equal(t, len(a), len(b))
}

// TestPacket_ConcurrentDecode verifies that decoding a shared packet from multiple goroutines is race-free.
func TestPacket_ConcurrentDecode(t *testing.T) {
	raw := rtpBytes(t, 7, 100, tsPayload(2))
	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	require.NoError(t, pkt.T.From(raw))

	const n = 8
	// require.* must run on the test goroutine, so each worker reports its outcome back via a channel instead.
	type result struct {
		err error
		seq uint16
	}
	results := make(chan result, n)
	var wg sync.WaitGroup
	for range n {
		pkt.Reference()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer pkt.Release()
			r, err := pkt.T.Rtp()
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{seq: r.Packet().SequenceNumber}
		}()
	}
	wg.Wait()
	close(results)
	pkt.Release()

	for res := range results {
		require.NoError(t, res.err)
		require.EqualValues(t, 7, res.seq)
	}
}
