package rtp

import (
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestNewRtpPacketizer_SingleCompletePacket(t *testing.T) {
	p := NewRtpPacketizer()

	// default expected payload len is tsPacketCount (7) * tsPacketLen
	payloadLen := 7 * tsPacketLen

	rtpPkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 1,
			Timestamp:      12345,
			SSRC:           0x01020304,
		},
		Payload: make([]byte, payloadLen),
	}

	raw, err := rtpPkt.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(raw), minRtpHeaderLen)

	// write the full packet at once
	p.Write(raw)

	out, err := p.Next()
	require.NoError(t, err)
	require.NotNil(t, out)

	// the packetizer should return exactly the RTP packet we put in
	require.Equal(t, raw, out)
	require.Equal(t, len(raw), p.TargetPacketSize())
	require.Equal(t, len(raw), p.Consumed())

	// no more data after consuming the packet
	out2, err := p.Next()
	require.NoError(t, err)
	require.Nil(t, out2)
}

func TestNewRtpPacketizer_FragmentedWrites(t *testing.T) {
	p := NewRtpPacketizer()

	payloadLen := 7 * tsPacketLen
	rtpPkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 2,
			Timestamp:      23456,
			SSRC:           0x0A0B0C0D,
		},
		Payload: make([]byte, payloadLen),
	}

	raw, err := rtpPkt.Marshal()
	require.NoError(t, err)

	// split the RTP packet into three chunks to simulate fragmented writes
	chunk1 := raw[:5]
	chunk2 := raw[5:20]
	chunk3 := raw[20:]

	// write first chunk, header incomplete, Next should return nil
	p.Write(chunk1)
	out, err := p.Next()
	require.NoError(t, err)
	require.Nil(t, out)

	// write second chunk, still not enough for full packet
	p.Write(chunk2)
	out, err = p.Next()
	require.NoError(t, err)
	require.Nil(t, out)

	// write final chunk, now full packet should be available
	p.Write(chunk3)

	var got []byte
	for i := 0; i < 3; i++ { // safe upper bound; loop guards against internal buffering behavior
		out, err = p.Next()
		require.NoError(t, err)
		if out != nil {
			got = out
			break
		}
	}
	require.NotNil(t, got, "expected packet after providing all fragments")
	require.Equal(t, raw, got)
	require.Equal(t, len(raw), p.TargetPacketSize())
	require.Equal(t, len(raw), p.Consumed())
}

func TestNewRtpPacketizer_MultipleConcatenatedPackets(t *testing.T) {
	p := NewRtpPacketizer()

	payloadLen := 7 * tsPacketLen

	newPkt := func(seq uint16) []byte {
		rtpPkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: seq,
				Timestamp:      34567,
				SSRC:           0x0F0E0D0C,
			},
			Payload: make([]byte, payloadLen),
		}
		raw, err := rtpPkt.Marshal()
		require.NoError(t, err)
		return raw
	}

	raw1 := newPkt(10)
	raw2 := newPkt(11)

	// write two concatenated packets in a single write
	concat := append(append([]byte{}, raw1...), raw2...)
	p.Write(concat)

	// first packet
	out1, err := p.Next()
	require.NoError(t, err)
	require.NotNil(t, out1)
	require.Equal(t, raw1, out1)
	require.Equal(t, len(raw1), p.TargetPacketSize())
	require.Equal(t, len(raw1), p.Consumed())

	// second packet should still be buffered and ready
	out2, err := p.Next()
	require.NoError(t, err)
	require.NotNil(t, out2)
	require.Equal(t, raw2, out2)
	require.Equal(t, len(raw2), p.TargetPacketSize())
	require.Equal(t, len(raw1)+len(raw2), p.Consumed())

	// nothing left afterwards
	out3, err := p.Next()
	require.NoError(t, err)
	require.Nil(t, out3)
}

func TestNewRtpPacketizer_InvalidPaddingHeader(t *testing.T) {
	p := NewRtpPacketizer()

	// build a normal packet first
	payloadLen := 7 * tsPacketLen
	rtpPkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 100,
			Timestamp:      99999,
			SSRC:           0x01010101,
			Padding:        true,
			PaddingSize:    5,
		},
		Payload: make([]byte, payloadLen),
	}

	raw, err := rtpPkt.Marshal()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(raw), minRtpHeaderLen)

	p.Write(raw)

	out, err := p.Next()
	require.Nil(t, out)
	require.Error(t, err, "padding is not supported and should cause an error")
}

func TestNewRtpPacketizer_WithCustomTsPacketCount(t *testing.T) {
	const tsCount = 3
	p := NewRtpPacketizer(OptTsPacketCount(tsCount))

	payloadLen := tsCount * tsPacketLen
	rtpPkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SequenceNumber: 200,
			Timestamp:      123456,
			SSRC:           0x02020202,
		},
		Payload: make([]byte, payloadLen),
	}

	raw, err := rtpPkt.Marshal()
	require.NoError(t, err)

	p.Write(raw)

	out, err := p.Next()
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, raw, out)
	require.Equal(t, len(raw), p.TargetPacketSize())
	require.Equal(t, len(raw), p.Consumed())
}
