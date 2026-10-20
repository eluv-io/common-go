package rtmp

import (
	"bufio"
	"io"

	"github.com/bluenviron/gortmplib/pkg/message"

	"github.com/eluv-io/errors-go"
)

// The RTMP chunk stream, as far as publishing media needs it (RTMP 1.0 section 5.3).
const (
	// maxMessageLen is what the 24 bit length field of a chunk header can express.
	maxMessageLen = 0xFFFFFF
	// extTimestampMark is written in place of a timestamp that does not fit in 24 bits; the real value follows the
	// message header, and again after every continuation header of the same message.
	extTimestampMark = 0xFFFFFF
	// writeBufSize is the size of the buffer media is assembled in before it goes to the socket. A chunk is at most
	// chunkSize plus a header, so one buffer holds a whole chunk and the socket sees one write per chunk.
	writeBufSize = 64 << 10

	fmtType0 = 0 << 6 // full header: absolute timestamp, length, type and message stream
	fmtType1 = 1 << 6 // timestamp delta, length and type; same message stream as before
	fmtType3 = 3 << 6 // no header: the continuation of a message already described
)

// FLV tag headers, the first bytes of a video or audio message body (Adobe's Video File Format v10.1, annex E).
const (
	flvKeyframeH264 = 0x17 // frame type 1 (keyframe), codec 7 (AVC)
	flvInterH264    = 0x27 // frame type 2 (inter frame), codec 7 (AVC)
	flvAVCNALU      = 1    // AVC packet type 1: one or more NAL units
	// flvAAC is sound format 10 (AAC), rate 44 kHz, 16 bit, stereo. For AAC those three fields are fixed by
	// convention and the real rate and channel count are read from the AudioSpecificConfig instead.
	flvAAC    = 0xAF
	flvAACRaw = 1 // AAC packet type 1: a raw frame
)

// chunkStream is the writer's state for one chunk stream: what the receiver can infer from what it was sent before.
type chunkStream struct {
	csid    byte
	started bool
	lastTS  uint32
	ext     bool   // the current message's timestamp field is extended
	extVal  uint32 // its value, repeated after every continuation header
}

// chunkWriter writes RTMP media messages onto a connection. It exists because the media path is the only hot one:
// gortmplib assembles a message by allocating a body, copying the payload into it, then allocating a chunk buffer and
// copying it again, which is two copies and about ten allocations per access unit. Here the header goes into a
// scratch array on the writer and the payload slices are handed to the buffered writer as they are, so a frame is
// copied once, into the buffer that feeds the socket.
//
// It handles media only: control messages, the handshake and the sequence headers stay with gortmplib's writer, which
// owns the chunk streams it uses. Nothing here depends on the rest of this package, so it can move into gortmplib as
// a writev-style message writer.
type chunkWriter struct {
	bw    *bufio.Writer
	video chunkStream
	audio chunkStream

	hdr [16]byte // chunk header: basic header, message header and extended timestamp
	tag [5]byte  // FLV tag header
	lb  [4]byte  // NAL unit length prefix
}

func newChunkWriter(w io.Writer) *chunkWriter {
	return &chunkWriter{
		bw:    bufio.NewWriterSize(w, writeBufSize),
		video: chunkStream{csid: message.VideoChunkStreamID},
		audio: chunkStream{csid: message.AudioChunkStreamID},
	}
}

// started reports whether a media message has been written, i.e. whether this writer now owns its chunk streams.
func (w *chunkWriter) started() bool {
	return w.video.started || w.audio.started
}

// writeVideo sends one H.264 access unit as an FLV video message. The NAL units are written with their four byte
// length prefixes, which is the AVCC form the sequence header announced, without being assembled into one buffer.
func (w *chunkWriter) writeVideo(nalus [][]byte, ts, cts uint32, keyframe bool) error {
	bodyLen := len(w.tag)
	for _, n := range nalus {
		if len(n) == 0 {
			continue
		}
		bodyLen += 4 + len(n)
	}
	if bodyLen > maxMessageLen {
		return errors.NoTrace("rtmp.writeVideo", errors.K.Invalid, "reason", "access unit exceeds the RTMP message size",
			"size", bodyLen, "max", maxMessageLen)
	}

	w.tag[0] = flvInterH264
	if keyframe {
		w.tag[0] = flvKeyframeH264
	}
	w.tag[1] = flvAVCNALU
	w.tag[2], w.tag[3], w.tag[4] = byte(cts>>16), byte(cts>>8), byte(cts)

	body, err := w.beginMessage(&w.video, byte(message.TypeVideo), ts, uint32(bodyLen))
	if err != nil {
		return err
	}
	body.write(w.tag[:])
	for _, n := range nalus {
		if len(n) == 0 {
			continue
		}
		w.lb[0], w.lb[1] = byte(len(n)>>24), byte(len(n)>>16)
		w.lb[2], w.lb[3] = byte(len(n)>>8), byte(len(n))
		body.write(w.lb[:])
		body.write(n)
	}
	return w.endMessage(&w.video, ts, body)
}

// writeAudio sends one raw AAC frame as an FLV audio message.
func (w *chunkWriter) writeAudio(au []byte, ts uint32) error {
	bodyLen := 2 + len(au)
	if bodyLen > maxMessageLen {
		return errors.NoTrace("rtmp.writeAudio", errors.K.Invalid, "reason", "access unit exceeds the RTMP message size",
			"size", bodyLen, "max", maxMessageLen)
	}
	w.tag[0], w.tag[1] = flvAAC, flvAACRaw

	body, err := w.beginMessage(&w.audio, byte(message.TypeAudio), ts, uint32(bodyLen))
	if err != nil {
		return err
	}
	body.write(w.tag[:2])
	body.write(au)
	return w.endMessage(&w.audio, ts, body)
}

// beginMessage writes the message header and returns the cursor its body is written through.
//
// The first message of a chunk stream, and any whose timestamp goes backwards, gets a type 0 header, which states
// everything and resets the receiver's state for that stream. Every other message gets a type 1 header, which carries
// the delta to the previous one and is four bytes shorter. Type 2 would save three more but only for a constant frame
// rate, which a stream with B frames or a variable rate does not have.
func (w *chunkWriter) beginMessage(cs *chunkStream, msgType byte, ts, bodyLen uint32) (bodyCursor, error) {
	var val uint32
	n := 0
	if !cs.started || ts < cs.lastTS {
		w.hdr[0] = fmtType0 | cs.csid
		val = ts
		w.putTimestamp(1, val)
		w.hdr[4], w.hdr[5], w.hdr[6] = byte(bodyLen>>16), byte(bodyLen>>8), byte(bodyLen)
		w.hdr[7] = msgType
		// The message stream id goes out big-endian, as gortmplib's own writer and reader treat it, so both writers
		// on this connection describe the same stream.
		msid := uint32(publishStreamID)
		w.hdr[8], w.hdr[9] = byte(msid>>24), byte(msid>>16)
		w.hdr[10], w.hdr[11] = byte(msid>>8), byte(msid)
		n = 12
	} else {
		w.hdr[0] = fmtType1 | cs.csid
		val = ts - cs.lastTS
		w.putTimestamp(1, val)
		w.hdr[4], w.hdr[5], w.hdr[6] = byte(bodyLen>>16), byte(bodyLen>>8), byte(bodyLen)
		w.hdr[7] = msgType
		n = 8
	}
	cs.ext = val >= extTimestampMark
	cs.extVal = val
	if cs.ext {
		w.hdr[n], w.hdr[n+1] = byte(val>>24), byte(val>>16)
		w.hdr[n+2], w.hdr[n+3] = byte(val>>8), byte(val)
		n += 4
	}
	if _, err := w.bw.Write(w.hdr[:n]); err != nil {
		return bodyCursor{}, err
	}
	return bodyCursor{w: w, cs: cs, left: chunkSize}, nil
}

// endMessage flushes the message and records what the receiver now knows about the chunk stream.
func (w *chunkWriter) endMessage(cs *chunkStream, ts uint32, body bodyCursor) error {
	if body.err != nil {
		return body.err
	}
	if err := w.bw.Flush(); err != nil {
		return err
	}
	cs.lastTS = ts
	cs.started = true
	return nil
}

func (w *chunkWriter) putTimestamp(at int, val uint32) {
	if val >= extTimestampMark {
		w.hdr[at], w.hdr[at+1], w.hdr[at+2] = 0xFF, 0xFF, 0xFF
		return
	}
	w.hdr[at], w.hdr[at+1], w.hdr[at+2] = byte(val>>16), byte(val>>8), byte(val)
}

// bodyCursor writes the body of one message, splitting it into chunks of at most chunkSize bytes. The caller hands it
// the pieces of the body in order and never has to know where a chunk boundary falls.
type bodyCursor struct {
	w    *chunkWriter
	cs   *chunkStream
	left uint32 // bytes still allowed in the current chunk
	err  error
}

func (b *bodyCursor) write(p []byte) {
	for len(p) > 0 && b.err == nil {
		if b.left == 0 {
			// A continuation header is only written when bytes remain, so a body that is a multiple of the chunk
			// size does not end with an empty chunk.
			b.err = b.w.writeContinuation(b.cs)
			if b.err != nil {
				return
			}
			b.left = chunkSize
		}
		n := uint32(len(p))
		if n > b.left {
			n = b.left
		}
		var written int
		written, b.err = b.w.bw.Write(p[:n])
		b.left -= uint32(written)
		p = p[written:]
	}
}

// writeContinuation starts the next chunk of the message in progress: a type 3 header, which says only that this is
// more of the same, plus the extended timestamp again when the message header used one.
func (w *chunkWriter) writeContinuation(cs *chunkStream) error {
	w.hdr[0] = fmtType3 | cs.csid
	n := 1
	if cs.ext {
		w.hdr[1], w.hdr[2] = byte(cs.extVal>>24), byte(cs.extVal>>16)
		w.hdr[3], w.hdr[4] = byte(cs.extVal>>8), byte(cs.extVal)
		n = 5
	}
	_, err := w.bw.Write(w.hdr[:n])
	return err
}
