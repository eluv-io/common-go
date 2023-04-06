package codecs

import (
	"bytes"
	"io"

	"github.com/eluv-io/common-go/format/codecs/header"
	"github.com/eluv-io/errors-go"
)

var (
	muxHeader            = header.New("/multicodec")
	_         MultiCodec = (*MuxCodec)(nil)
)

// NewMuxCodec creates a MultiCodec that muxes between given codecs - see MuxCodec.
func NewMuxCodec(codecs ...MultiCodec) *MuxCodec {
	return &MuxCodec{codecs, SelectFirst, false}
}

// SelectCodec is a function that selects the codec to use for marshaling a given data structure.
type SelectCodec func(v interface{}, codecs []MultiCodec) MultiCodec

// SelectFirst is the default SelectCodec function that selects the first codec given.
func SelectFirst(v interface{}, codecs []MultiCodec) MultiCodec {
	return codecs[0]
}

// MuxCodec is a MultiCodec that multiplexes between a list of configured codecs. The codec for encoding is chosen with
// a SelectCodec function called for the first object being encoded (the provided SelectFirst function simply selects
// the first codec in the list). The codec for decoding is chosen according to the MultiCodec header read from the
// beginning of the data stream.
//
// NOTE: the MultiCodec header is written only once at the very beginning even if the same encoder is used for encoding
// multiple objects:
//
//	HEADER|object1|object2|...
//
// Likewise, the decoder expects only a single header and decodes all subsequent objects with the same codec.
//
// MuxCodec is NOT thread-safe - use from a single goroutine only or synchronize access.
type MuxCodec struct {
	Codecs []MultiCodec // codecs to use
	Select SelectCodec  // pick a codec for encoding
	Wrap   bool         // whether to wrap with own header
}

func (c *MuxCodec) Encoder(w io.Writer) Encoder {
	return &muxEncoderImpl{
		muxEncoderBase: muxEncoderBase{writer: w, mux: c},
	}
}

func (c *MuxCodec) Decoder(r io.Reader) Decoder {
	return &muxDecoderImpl{
		muxDecoderBase: muxDecoderBase{reader: r, mux: c},
	}
}

func (c *MuxCodec) VersionedEncoder(w io.Writer) VersionedEncoder {
	return &muxVersionedEncoderImpl{
		muxEncoderBase: muxEncoderBase{writer: w, mux: c},
	}
}

func (c *MuxCodec) VersionedDecoder(r io.Reader) VersionedMultiDecoder {
	return &muxVersionedDecoderImpl{
		muxDecoderBase: muxDecoderBase{
			reader: r, mux: c},
	}
}

func (c *MuxCodec) Header() header.Header {
	return muxHeader
}

func (c *MuxCodec) DisableVersions() MultiCodec {
	clone := *c
	clone.Codecs = make([]MultiCodec, len(c.Codecs))
	for i, codec := range c.Codecs {
		clone.Codecs[i] = codec.DisableVersions()
	}
	return &clone
}

////////////////////////////////////////////////////////////////////////////////

type muxEncoderBase struct {
	writer      io.Writer
	mux         *MuxCodec
	initialized bool
}

func (c *muxEncoderBase) init(v interface{}, selected func(codec MultiCodec)) error {
	if c.initialized {
		return nil
	}

	e := errors.Template("MuxEncoder.init", errors.K.Invalid)
	codec := c.mux.Select(v, c.mux.Codecs)
	if codec == nil {
		return e("reason", "no suitable encoder")
	}
	if c.mux.Wrap {
		if _, err := c.writer.Write(c.mux.Header()); err != nil {
			return e(err, "reason", "failed to write mux header")
		}
	}
	selected(codec)
	c.initialized = true

	return nil
}

////////////////////////////////////////////////////////////////////////////////

type muxEncoderImpl struct {
	muxEncoderBase
	enc Encoder
}

func (c *muxEncoderImpl) Encode(v interface{}) error {
	err := c.init(v, func(codec MultiCodec) { c.enc = codec.Encoder(c.writer) })
	if err == nil {
		err = c.enc.Encode(v)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

type muxVersionedEncoderImpl struct {
	muxEncoderBase
	enc VersionedEncoder
}

func (c *muxVersionedEncoderImpl) EncodeVersioned(version uint, obj interface{}) error {
	err := c.init(obj, func(codec MultiCodec) { c.enc = codec.VersionedEncoder(c.writer) })
	if err == nil {
		err = c.enc.EncodeVersioned(version, obj)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

type muxDecoderBase struct {
	reader      io.Reader
	mux         *MuxCodec
	initialized bool
}

func (c *muxDecoderBase) init(selected func(c MultiCodec, r io.Reader)) error {
	if c.initialized {
		return nil
	}

	e := errors.Template("MuxDecoder.init", errors.K.Invalid)
	if c.mux.Wrap {
		if err := header.ConsumeHeader(c.reader, c.mux.Header()); err != nil {
			return e(err, "reason", "failed to read mux header")
		}
	}

	// get next header, to select codec
	hdr, err := header.ReadHeader(c.reader)
	if err != nil {
		return e(err, "reason", "failed to read multicodec header")
	}

	// "unwind" the read as the selected codec consumes header
	rdr := header.WrapHeaderReader(hdr, c.reader)

	codec := c.codecForHeader(hdr)
	if codec == nil {
		return e("reason", "no suitable decoder", "codec", string(header.Path(hdr)))
	}

	c.initialized = true
	selected(codec, rdr)
	return nil
}

func (c *muxDecoderBase) codecForHeader(hdr []byte) MultiCodec {
	for _, c := range c.mux.Codecs {
		if bytes.Equal(hdr, c.Header()) {
			return c
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type muxDecoderImpl struct {
	muxDecoderBase
	dec Decoder
}

func (c *muxDecoderImpl) Decode(v interface{}) (err error) {
	err = c.init(func(codec MultiCodec, reader io.Reader) {
		c.dec = codec.Decoder(reader)
	})
	if err == nil {
		err = c.dec.Decode(v)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

type muxVersionedDecoderImpl struct {
	muxDecoderBase
	dec VersionedMultiDecoder
}

func (c *muxVersionedDecoderImpl) DecodeVersioned(
	selector func(version uint, codec string) interface{},
) (
	obj interface{},
	version uint,
	err error,
) {
	err = c.init(func(codec MultiCodec, reader io.Reader) {
		c.dec = codec.VersionedDecoder(reader)
	})
	if err == nil {
		obj, version, err = c.dec.DecodeVersioned(selector)
	}
	return obj, version, err
}
