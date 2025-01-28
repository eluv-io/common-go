package codecs

import (
	"io"
)

// Codec is an algorithm for coding data from one representation to another. For convenience, it's defined in the
// usual sense: a function and its inverse, to encode and decode.
type Codec interface {
	// Decoder wraps given io.Reader and returns an object which will decode bytes into objects.
	Decoder(r io.Reader) Decoder

	// Encoder wraps given io.Writer and returns an Encoder
	Encoder(w io.Writer) Encoder
}

// Encoder encodes objects into bytes and writes them to an underlying io.Writer. Works like encoding.Marshal
type Encoder interface {
	Encode(obj interface{}) error
}

// Decoder decodes objects from bytes from an underlying io.Reader, into given object. Works like encoding.Unmarshal
type Decoder interface {
	Decode(obj interface{}) error
}

////////////////////////////////////////////////////////////////////////////////

type CreateEncoderFn func(w io.Writer) Encoder
type CreateDecoderFn func(io.Reader) Decoder

// NewCodec creates a new Codec from an encoder and a decoder creation function.
func NewCodec(enc CreateEncoderFn, dec CreateDecoderFn) Codec {
	return &codec{encoderFn: enc, decoderFn: dec}
}

type codec struct {
	encoderFn func(w io.Writer) Encoder
	decoderFn func(r io.Reader) Decoder
}

func (c *codec) Decoder(r io.Reader) Decoder {
	return c.decoderFn(r)
}

func (c *codec) Encoder(w io.Writer) Encoder {
	return c.encoderFn(w)
}
