package codecs

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	mc "github.com/multiformats/go-multicodec"
)

var (
	ErrNoCodec = errors.New("no suitable codec")
	Header     []byte
	_          mc.Multicodec = (*MuxCodec)(nil)
)

func init() {
	Header = mc.Header([]byte("/multicodec"))
}

// NewMuxCodec creates a multicodec that muxes between given codecs - see MuxCodec.
func NewMuxCodec(codecs ...mc.Multicodec) *MuxCodec {
	return &MuxCodec{codecs, SelectFirst, false}
}

// SelectCodec is a function that selects the codec to use for marshaling a given data structure.
type SelectCodec func(v interface{}, codecs []mc.Multicodec) mc.Multicodec

// SelectFirst is the default SelectCodec functiont that selects the first codec given.
func SelectFirst(v interface{}, codecs []mc.Multicodec) mc.Multicodec {
	return codecs[0]
}

// MuxCodec is a multicodec that muxes between given codecs. The codec for encoding is chosen with a SelectCodec
// function called for the first object being encoded - per default the first codec in the list is selected. The codec
// for decoding is chosen according to the multicodec header in the data stream.
//
// NOTE: the multicodec header is written only once at the very beginning even if the same encoder is used for encoding
// multiple objects:
//
//	HEADER|object1|object2|...
//
// Likewise, the decoder expects only a single header and decodes all subsequent objects with the same codec.
//
// MuxCodec is NOT thread-safe - use from a single goroutine only or synchronize access.
type MuxCodec struct {
	Codecs []mc.Multicodec // codecs to use
	Select SelectCodec     // pick a codec for encoding
	Wrap   bool            // whether to wrap with own header
}

func (c *MuxCodec) Encoder(w io.Writer) mc.Encoder {
	return &encoderImpl{writer: w, mux: c}
}

func (c *MuxCodec) Decoder(r io.Reader) mc.Decoder {
	return &decoderImpl{reader: r, mux: c}
}

func (c *MuxCodec) Header() []byte {
	return Header
}

type encoderImpl struct {
	writer io.Writer
	mux    *MuxCodec
	enc    mc.Encoder
}

type decoderImpl struct {
	reader io.Reader
	mux    *MuxCodec
	dec    mc.Decoder
}

func (c *encoderImpl) Encode(v interface{}) error {
	if c.enc == nil {
		codec := c.mux.Select(v, c.mux.Codecs)
		if codec == nil {
			return ErrNoCodec
		}
		c.enc = codec.Encoder(c.writer)
		if c.mux.Wrap {
			// write multicodec header
			if _, err := c.writer.Write(c.mux.Header()); err != nil {
				return err
			}
		}
	}

	return c.enc.Encode(v)
}

func (c *decoderImpl) Decode(v interface{}) error {
	if c.dec == nil {
		if c.mux.Wrap {
			// read multicodec header
			if err := mc.ConsumeHeader(c.reader, c.mux.Header()); err != nil {
				return err
			}
		}

		// get next header, to select codec
		hdr, err := mc.ReadHeader(c.reader)
		if err != nil {
			return err
		}

		// "unwind" the read as the selected codec consumes header
		rdr := mc.WrapHeaderReader(hdr, c.reader)

		codec := c.codecForHeader(hdr)
		if codec == nil {
			return fmt.Errorf("no codec for %s", hdr)
		}

		c.dec = codec.Decoder(rdr)
	}
	return c.dec.Decode(v)
}

func (c *decoderImpl) codecForHeader(hdr []byte) mc.Multicodec {
	for _, c := range c.mux.Codecs {
		if bytes.Equal(hdr, c.Header()) {
			return c
		}
	}
	return nil
}
