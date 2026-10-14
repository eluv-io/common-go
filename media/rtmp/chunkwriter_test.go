package rtmp

import (
	"bytes"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/gortmplib/pkg/rawmessage"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/internal/raceflag"
	"github.com/eluv-io/common-go/media/fmp4"
)

// The chunk writer is checked against gortmplib's readers, the same ones the in-process receiver decodes with, so a
// header this writer gets wrong fails here rather than against a real server.

// decodeMessages reads back what the writer produced, at the chunk layer: the message reader above it has no way to
// be told the negotiated chunk size, which on a real connection it learns from the SetChunkSize the session sends.
// The end-to-end path through that layer is covered by the connection tests, which publish to the in-process
// receiver.
func decodeMessages(t *testing.T, raw []byte) []*rawmessage.Message {
	t.Helper()
	r := rawmessage.NewReader(bytes.NewReader(raw), nil, nil)
	require.NoError(t, r.SetChunkSize(chunkSize))
	var out []*rawmessage.Message
	for {
		msg, err := r.Read()
		if err != nil {
			return out
		}
		out = append(out, msg)
	}
}

// videoOf asserts that msg is an FLV video message and returns its access unit in AVCC form.
func videoOf(t *testing.T, msg *rawmessage.Message) (au []byte, keyframe bool, cts uint32) {
	t.Helper()
	require.EqualValues(t, message.TypeVideo, msg.Type)
	require.GreaterOrEqual(t, len(msg.Body), 5)
	require.Contains(t, []byte{flvKeyframeH264, flvInterH264}, msg.Body[0])
	require.EqualValues(t, flvAVCNALU, msg.Body[1])
	cts = uint32(msg.Body[2])<<16 | uint32(msg.Body[3])<<8 | uint32(msg.Body[4])
	return msg.Body[5:], msg.Body[0] == flvKeyframeH264, cts
}

// audioOf asserts that msg is an FLV audio message and returns its raw AAC frame.
func audioOf(t *testing.T, msg *rawmessage.Message) []byte {
	t.Helper()
	require.EqualValues(t, message.TypeAudio, msg.Type)
	require.GreaterOrEqual(t, len(msg.Body), 2)
	require.EqualValues(t, flvAAC, msg.Body[0])
	require.EqualValues(t, flvAACRaw, msg.Body[1])
	return msg.Body[2:]
}

// writeInto builds a chunk writer over a buffer and runs write on it.
func writeInto(t *testing.T, write func(w *chunkWriter) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := newChunkWriter(&buf)
	require.NoError(t, write(w))
	return buf.Bytes()
}

// nalOf returns a NAL unit of the given type and length whose body is recognisable.
func nalOf(typ byte, n int) []byte {
	out := make([]byte, n)
	out[0] = typ
	for i := 1; i < n; i++ {
		out[i] = byte(i)
	}
	return out
}

// avccOf is the AVCC serialisation the writer is expected to produce for these NAL units.
func avccOf(nalus ...[]byte) []byte {
	var out []byte
	for _, n := range nalus {
		out = append(out, byte(len(n)>>24), byte(len(n)>>16), byte(len(n)>>8), byte(len(n)))
		out = append(out, n...)
	}
	return out
}

// TestChunkWriter_ChunkBoundaries covers the sizes where the body ends exactly on, just before and just after a chunk
// boundary, with boundaries falling inside a NAL unit and inside a length prefix.
func TestChunkWriter_ChunkBoundaries(t *testing.T) {
	// the body is 5 tag bytes plus 4 + len per NAL unit, so this hits the boundary exactly
	exact := chunkSize - 5 - 4
	for _, tc := range []struct {
		name  string
		nalus [][]byte
	}{
		{"one byte short of a chunk", [][]byte{nalOf(5, exact-1)}},
		{"exactly one chunk", [][]byte{nalOf(5, exact)}},
		{"one byte over a chunk", [][]byte{nalOf(5, exact+1)}},
		{"exactly two chunks", [][]byte{nalOf(5, exact+chunkSize)}},
		{"boundary inside a length prefix", [][]byte{nalOf(5, exact+2), nalOf(1, 16)}},
		{"several units over several chunks", [][]byte{nalOf(5, 40000), nalOf(1, 40000), nalOf(1, 40000)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := writeInto(t, func(w *chunkWriter) error {
				return w.writeVideo(tc.nalus, 1000, 0, true)
			})
			msgs := decodeMessages(t, raw)
			require.Len(t, msgs, 1)
			au, keyframe, _ := videoOf(t, msgs[0])
			require.Equal(t, avccOf(tc.nalus...), au)
			require.True(t, keyframe)
			require.Equal(t, time.Second, msgs[0].Timestamp)

			// the wire length is the header plus the body plus one continuation byte per further chunk
			body := 5
			for _, n := range tc.nalus {
				body += 4 + len(n)
			}
			chunks := (body + chunkSize - 1) / chunkSize
			require.Equal(t, 12+body+(chunks-1), len(raw), "no empty trailing chunk, one byte per continuation")
		})
	}
}

// TestChunkWriter_HeaderBytes pins the header bytes themselves, which no decoder in these tests would notice: a
// receiver accepts any correct mix of chunk types, but a real server also reads the message stream id.
func TestChunkWriter_HeaderBytes(t *testing.T) {
	raw := writeInto(t, func(w *chunkWriter) error {
		if err := w.writeVideo([][]byte{nalOf(5, 3)}, 1000, 0, true); err != nil {
			return err
		}
		if err := w.writeVideo([][]byte{nalOf(1, 3)}, 1040, 0, false); err != nil {
			return err
		}
		// a timestamp that goes backwards resets the stream with a full header
		return w.writeVideo([][]byte{nalOf(1, 3)}, 500, 0, false)
	})

	// type 0: csid 6, timestamp 1000, body length 12, type 9, stream 0x01000000
	require.Equal(t, []byte{0x06, 0x00, 0x03, 0xe8, 0x00, 0x00, 0x0c, 0x09, 0x01, 0x00, 0x00, 0x00}, raw[:12])
	// type 1: same csid, delta 40, length and type again
	rest := raw[12+12:]
	require.Equal(t, []byte{0x46, 0x00, 0x00, 0x28, 0x00, 0x00, 0x0c, 0x09}, rest[:8])
	// type 0 again after the backwards jump: timestamp 500
	rest = rest[8+12:]
	require.Equal(t, []byte{0x06, 0x00, 0x01, 0xf4, 0x00, 0x00, 0x0c, 0x09, 0x01, 0x00, 0x00, 0x00}, rest[:12])
}

func TestChunkWriter_AudioHeaderBytes(t *testing.T) {
	raw := writeInto(t, func(w *chunkWriter) error {
		return w.writeAudio([]byte{1, 2, 3, 4}, 250)
	})
	// type 0 on the audio chunk stream, body length 6, type 8
	require.Equal(t, []byte{0x04, 0x00, 0x00, 0xfa, 0x00, 0x00, 0x06, 0x08, 0x01, 0x00, 0x00, 0x00}, raw[:12])
	// the FLV audio tag header: AAC, 44.1 kHz, 16 bit, stereo, raw frame
	require.Equal(t, []byte{0xaf, 0x01, 1, 2, 3, 4}, raw[12:])

	msgs := decodeMessages(t, raw)
	require.Len(t, msgs, 1)
	require.Equal(t, []byte{1, 2, 3, 4}, audioOf(t, msgs[0]))
	require.Equal(t, 250*time.Millisecond, msgs[0].Timestamp)
}

// TestChunkWriter_ExtendedTimestamps covers the values at and beyond the 24 bit limit, including the extended field
// that every continuation of such a message has to repeat.
func TestChunkWriter_ExtendedTimestamps(t *testing.T) {
	t.Run("exactly at the limit", func(t *testing.T) {
		// 0xFFFFFF itself must be sent extended: the reader treats the mark as "extended follows"
		raw := writeInto(t, func(w *chunkWriter) error {
			return w.writeVideo([][]byte{nalOf(5, 4)}, 0xFFFFFF, 0, true)
		})
		require.Equal(t, []byte{0xff, 0xff, 0xff}, raw[1:4])
		require.Equal(t, []byte{0x00, 0xff, 0xff, 0xff}, raw[12:16])
		msgs := decodeMessages(t, raw)
		require.Len(t, msgs, 1)
		require.Equal(t, 0xFFFFFF*time.Millisecond, msgs[0].Timestamp)
	})

	t.Run("five hours", func(t *testing.T) {
		far := uint32(5 * time.Hour / time.Millisecond)
		raw := writeInto(t, func(w *chunkWriter) error {
			return w.writeVideo([][]byte{nalOf(5, 4)}, far, 0, true)
		})
		msgs := decodeMessages(t, raw)
		require.Len(t, msgs, 1)
		require.Equal(t, 5*time.Hour, msgs[0].Timestamp)
	})

	t.Run("delta beyond the limit", func(t *testing.T) {
		raw := writeInto(t, func(w *chunkWriter) error {
			if err := w.writeVideo([][]byte{nalOf(5, 4)}, 0, 0, true); err != nil {
				return err
			}
			return w.writeVideo([][]byte{nalOf(1, 4)}, 0xFFFFFF+1000, 0, false)
		})
		msgs := decodeMessages(t, raw)
		require.Len(t, msgs, 2)
		require.Equal(t, time.Duration(0xFFFFFF+1000)*time.Millisecond, msgs[1].Timestamp)
	})

	t.Run("continuations repeat the extended field", func(t *testing.T) {
		far := uint32(5 * time.Hour / time.Millisecond)
		big := nalOf(5, chunkSize+1000)
		raw := writeInto(t, func(w *chunkWriter) error {
			if err := w.writeVideo([][]byte{big}, far, 0, true); err != nil {
				return err
			}
			// a following message with a small delta must go back to plain continuations
			return w.writeVideo([][]byte{big}, far+40, 0, false)
		})
		body := 5 + 4 + len(big)
		// first message: 12 byte header, 4 extended bytes, then one continuation with its own 4 extended bytes
		require.Equal(t, 12+4+chunkSize+1+4+(body-chunkSize), len(raw[:12+4+chunkSize+1+4+(body-chunkSize)]))
		firstLen := 12 + 4 + chunkSize + 1 + 4 + (body - chunkSize)
		second := raw[firstLen:]
		require.Equal(t, byte(0x46), second[0], "type 1 header")
		require.Equal(t, byte(0xc6), second[8+chunkSize], "plain continuation, no extended field")

		msgs := decodeMessages(t, raw)
		require.Len(t, msgs, 2)
		au0, _, _ := videoOf(t, msgs[0])
		au1, _, _ := videoOf(t, msgs[1])
		require.Equal(t, avccOf(big), au0)
		require.Equal(t, avccOf(big), au1)
		require.Equal(t, 5*time.Hour, msgs[0].Timestamp)
		require.Equal(t, 5*time.Hour+40*time.Millisecond, msgs[1].Timestamp)
	})
}

// TestChunkWriter_RoundTripRandomSizes runs a stream of random access units through the writer and back, which is
// where an off-by-one in the chunking shows up whatever the shape of the input.
func TestChunkWriter_RoundTripRandomSizes(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	var buf bytes.Buffer
	w := newChunkWriter(&buf)

	type want struct {
		video bool
		au    []byte
		ts    time.Duration
	}
	var expect []want
	ts := uint32(0)
	for i := 0; i < 200; i++ {
		ts += uint32(rnd.Intn(60))
		if rnd.Intn(3) == 0 {
			au := nalOf(1, 1+rnd.Intn(400))
			require.NoError(t, w.writeAudio(au, ts))
			expect = append(expect, want{au: au, ts: time.Duration(ts) * time.Millisecond})
			continue
		}
		n := 1 + rnd.Intn(4)
		nalus := make([][]byte, n)
		for j := range nalus {
			nalus[j] = nalOf(byte(1+rnd.Intn(5)), 1+rnd.Intn(100000))
		}
		require.NoError(t, w.writeVideo(nalus, ts, 0, i == 0))
		expect = append(expect, want{video: true, au: avccOf(nalus...), ts: time.Duration(ts) * time.Millisecond})
	}

	msgs := decodeMessages(t, buf.Bytes())
	require.Len(t, msgs, len(expect))
	for i, e := range expect {
		require.Equal(t, e.ts, msgs[i].Timestamp, "message %d", i)
		if e.video {
			au, _, _ := videoOf(t, msgs[i])
			require.Equal(t, e.au, au, "message %d", i)
			continue
		}
		require.Equal(t, e.au, audioOf(t, msgs[i]), "message %d", i)
	}
}

// TestChunkWriter_RejectsOversizedAccessUnit covers the 24 bit message length field.
func TestChunkWriter_RejectsOversizedAccessUnit(t *testing.T) {
	var buf bytes.Buffer
	w := newChunkWriter(&buf)
	err := w.writeVideo([][]byte{make([]byte, maxMessageLen)}, 0, 0, true)
	require.ErrorContains(t, err, "exceeds the RTMP message size")
	require.Zero(t, buf.Len(), "nothing is written when the message cannot be framed")
}

// TestSession_ControlBetweenMediaMessages pins that a control message answered by the read loop lands between two
// media messages rather than inside one, which would desync the receiver.
func TestSession_ControlBetweenMediaMessages(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	read := make(chan []byte, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(server)
		read <- buf.Bytes()
	}()

	s := newSession(client, 0)
	big := nalOf(5, chunkSize+500) // several chunks, so an interleaved write would be visible
	require.NoError(t, s.writeVideo([][]byte{big}, 0, 0, true))
	require.NoError(t, s.write(&message.UserControlPingResponse{ServerTime: 42}))
	require.NoError(t, s.writeVideo([][]byte{nalOf(1, 100)}, 40, 0, false))
	require.NoError(t, s.writeAudio([]byte{1, 2, 3}, 40))
	require.NoError(t, client.Close())

	msgs := decodeMessages(t, <-read)
	require.Len(t, msgs, 4)
	au, _, _ := videoOf(t, msgs[0])
	require.Equal(t, avccOf(big), au, "the ping did not land inside the multi-chunk message")
	require.EqualValues(t, message.TypeUserControl, msgs[1].Type)
	_, _, _ = videoOf(t, msgs[2])
	require.Equal(t, []byte{1, 2, 3}, audioOf(t, msgs[3]))
}

// TestConn_WriteAllocs is the regression test for the reason the chunk writer exists.
func TestConn_WriteAllocs(t *testing.T) {
	if raceflag.Enabled {
		t.Skip("allocation counts do not hold under the race detector")
	}
	params := fmp4.CodecParams{
		Video: &fmp4.VideoParams{Codecs: "avc1.640028"},
		Audio: &fmp4.AudioParams{Codecs: "mp4a.40.2"},
	}
	c := newConn(Config{}, params, newSession(discardConn{}, 0))

	small := [][]byte{nalOf(5, 40), nalOf(1, 20<<10)}
	big := [][]byte{nalOf(5, 40), nalOf(1, 200<<10)} // several chunks
	au := nalOf(1, 400)
	var ts time.Duration
	advance := func() time.Duration { ts += 40 * time.Millisecond; return ts }

	for _, tc := range []struct {
		name  string
		write func()
	}{
		{"video within one chunk", func() {
			require.NoError(t, c.WriteVideo(small, advance(), ts, true))
		}},
		{"video over several chunks", func() {
			require.NoError(t, c.WriteVideo(big, advance(), ts, true))
		}},
		{"audio", func() {
			require.NoError(t, c.WriteAudio(au, advance()))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Zero(t, testing.AllocsPerRun(50, tc.write), "writing must not allocate")
		})
	}
}

// TestSession_RejectsMediaOnTheMessageWriter pins the guard that keeps the two writers off each other's chunk
// streams once media is flowing.
func TestSession_RejectsMediaOnTheMessageWriter(t *testing.T) {
	var sink discardConn
	s := newSession(&sink, 0)

	// before any media, gortmplib's writer owns the stream: this is how the sequence headers go out
	require.NoError(t, s.write(&message.Video{
		ChunkStreamID:   message.VideoChunkStreamID,
		MessageStreamID: publishStreamID,
		Codec:           message.CodecH264,
		IsKeyFrame:      true,
		Type:            message.VideoTypeConfig,
		AVCConfig:       nil,
	}))
	require.NoError(t, s.writeVideo([][]byte{nalOf(5, 8)}, 0, 0, true))
	err := s.write(&message.Video{ChunkStreamID: message.VideoChunkStreamID, MessageStreamID: publishStreamID})
	require.ErrorContains(t, err, "after the chunk writer took over")
}

// discardConn is a net.Conn that swallows everything written to it.
type discardConn struct {
	net.Conn
}

func (discardConn) Write(p []byte) (int, error)     { return len(p), nil }
func (discardConn) Read([]byte) (int, error)        { return 0, net.ErrClosed }
func (discardConn) Close() error                    { return nil }
func (discardConn) SetDeadline(time.Time) error     { return nil }
func (discardConn) SetReadDeadline(time.Time) error { return nil }
func (discardConn) SetWriteDeadline(t time.Time) error {
	return nil
}
func (discardConn) LocalAddr() net.Addr  { return fakeAddr{} }
func (discardConn) RemoteAddr() net.Addr { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "test" }
func (fakeAddr) String() string  { return "test" }
