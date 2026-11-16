package waveformdata

import (
	"encoding/binary"
	"io"

	"github.com/eluv-io/errors-go"
)

// Binary layout, all little-endian. Version 1 has a 20-byte header, version 2 appends the channel count. The data
// section holds Length*Channels*2 values, each an int16 (flags bit 0 clear) or an int8 (flags bit 0 set).
const (
	headerSizeV1 = 20
	headerSizeV2 = 24
	flag8Bit     = 1

	// maxDecodedValues bounds the allocation Decode makes from an untrusted header: 16 channels of 24 hours at one
	// bucket per millisecond.
	maxDecodedValues = 16 * 2 * 24 * 60 * 60 * 1000
)

// Header holds the fields of a waveform-data file header.
type Header struct {
	Version         int
	Bits            int
	SampleRate      int
	SamplesPerPixel int
	Length          int
	Channels        int
}

// DecodeHeader reads and validates a waveform-data header. A version 1 header reports one channel.
func DecodeHeader(r io.Reader) (Header, error) {
	e := errors.Template("waveformdata.DecodeHeader", errors.K.Invalid)

	var buf [headerSizeV2]byte
	if _, err := io.ReadFull(r, buf[:headerSizeV1]); err != nil {
		return Header{}, e(err, "reason", "short header")
	}
	h := Header{
		Version:         int(int32(binary.LittleEndian.Uint32(buf[0:]))),
		SampleRate:      int(int32(binary.LittleEndian.Uint32(buf[8:]))),
		SamplesPerPixel: int(int32(binary.LittleEndian.Uint32(buf[12:]))),
		Length:          int(binary.LittleEndian.Uint32(buf[16:])),
		Channels:        1,
	}
	flags := binary.LittleEndian.Uint32(buf[4:])
	h.Bits = Bits16
	if flags&flag8Bit != 0 {
		h.Bits = Bits8
	}

	switch h.Version {
	case 1:
	case 2:
		if _, err := io.ReadFull(r, buf[headerSizeV1:headerSizeV2]); err != nil {
			return Header{}, e(err, "reason", "short version 2 header")
		}
		h.Channels = int(int32(binary.LittleEndian.Uint32(buf[20:])))
	default:
		return Header{}, e("reason", "unsupported version", "version", h.Version)
	}

	if h.SampleRate <= 0 || h.SamplesPerPixel <= 0 || h.Channels <= 0 {
		return Header{}, e("reason", "invalid header", "header", h)
	}
	if h.Length*h.Channels*2 > maxDecodedValues {
		return Header{}, e("reason", "implausible length", "header", h)
	}
	return h, nil
}

// Decode reads a complete waveform-data file. Version 1 and 2 files are accepted; 8-bit values are widened to int16
// without rescaling, so an 8-bit file decodes to Bits == 8 with values in [-128, 127].
func Decode(r io.Reader) (*Waveform, error) {
	e := errors.Template("waveformdata.Decode", errors.K.Invalid)

	h, err := DecodeHeader(r)
	if err != nil {
		return nil, e(err)
	}
	w := &Waveform{
		Version:         h.Version,
		Channels:        h.Channels,
		SampleRate:      h.SampleRate,
		SamplesPerPixel: h.SamplesPerPixel,
		Bits:            h.Bits,
		Data:            make([]int16, h.Length*h.Channels*2),
	}

	switch h.Bits {
	case Bits16:
		buf := make([]byte, len(w.Data)*2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, e(err, "reason", "short data section", "header", h)
		}
		for i := range w.Data {
			w.Data[i] = int16(binary.LittleEndian.Uint16(buf[2*i:]))
		}
	case Bits8:
		buf := make([]byte, len(w.Data))
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, e(err, "reason", "short data section", "header", h)
		}
		for i, b := range buf {
			w.Data[i] = int16(int8(b))
		}
	}
	return w, nil
}

// Encode writes the waveform as a version 2 file, regardless of w.Version.
func Encode(wr io.Writer, w *Waveform) error {
	e := errors.Template("waveformdata.Encode", errors.K.Invalid)
	if err := w.Validate(); err != nil {
		return e(err)
	}

	var flags uint32
	if w.Bits == Bits8 {
		flags = flag8Bit
	}
	var hdr [headerSizeV2]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(Version))
	binary.LittleEndian.PutUint32(hdr[4:], flags)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(w.SampleRate))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(w.SamplesPerPixel))
	binary.LittleEndian.PutUint32(hdr[16:], uint32(w.Length()))
	binary.LittleEndian.PutUint32(hdr[20:], uint32(w.Channels))
	if _, err := wr.Write(hdr[:]); err != nil {
		return e(err, errors.K.IO)
	}

	var buf []byte
	switch w.Bits {
	case Bits16:
		buf = make([]byte, len(w.Data)*2)
		for i, v := range w.Data {
			binary.LittleEndian.PutUint16(buf[2*i:], uint16(v))
		}
	case Bits8:
		buf = make([]byte, len(w.Data))
		for i, v := range w.Data {
			buf[i] = byte(int8(v))
		}
	}
	if _, err := wr.Write(buf); err != nil {
		return e(err, errors.K.IO)
	}
	return nil
}
