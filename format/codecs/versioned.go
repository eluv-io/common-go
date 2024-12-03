package codecs

import (
	"io"

	"github.com/eluv-io/errors-go"
)

// VersionedCodec is an algorithm for coding data from one representation
// to another. For convenience, we define a codec in the usual
// sense: a function and its inverse, to encode and decode.
type VersionedCodec interface {
	// VersionedDecoder wraps given io.Reader and returns an object which
	// will decode bytes into objects.
	VersionedDecoder(r io.Reader) VersionedDecoder

	// VersionedEncoder wraps given io.Writer and returns an Encoder
	VersionedEncoder(w io.Writer) VersionedEncoder
}

// VersionedEncoder is an encoder that prefixes encoded data with a version number that is stored as a varint.
type VersionedEncoder interface {
	// EncodeVersioned encodes the given object into bytes and writes them to an underlying io.Writer.
	EncodeVersioned(version uint, obj interface{}) error
}

// VersionedDecoder decodes objects from bytes from an underlying io.Reader into objects according to the encoded
// version.
type VersionedDecoder interface {
	// DecodeVersioned decodes objects from the bytes of the underlying io.Reader. It first reads the version varint and
	// passes it to the given selector function. The selector chooses the target object to use for unmarshaling the
	// given version and returns it to the decoder. Finally, the decoder unmarshals the bytes into that object.
	DecodeVersioned(selector func(version uint) interface{}) (obj interface{}, version uint, err error)
}

////////////////////////////////////////////////////////////////////////////////

func NewVersionedCodec(codec Codec) VersionedCodec {
	return &versionedCodec{
		codec: codec,
	}
}

type versionedCodec struct {
	codec Codec
	path  []byte
}

func (c *versionedCodec) VersionedEncoder(w io.Writer) VersionedEncoder {
	return NewVersionedEncoder(w, c.codec.Encoder(w))
}

func (c *versionedCodec) VersionedDecoder(r io.Reader) VersionedDecoder {
	return NewVersionedDecoder(r, c.codec.Decoder(r))
}

////////////////////////////////////////////////////////////////////////////////

//goland:noinspection GoExportedFuncWithUnexportedType
func NewVersionedEncoder(writer io.Writer, encoder Encoder) *versionedEncoder {
	return &versionedEncoder{
		writer:  writer,
		encoder: encoder,
	}
}

type versionedEncoder struct {
	writer           io.Writer
	encoder          Encoder
	versionsDisabled bool
}

func (e *versionedEncoder) writeVersion(version uint) (err error) {
	if e.versionsDisabled {
		return nil
	}

	err = e.encoder.Encode(version)
	if err != nil {
		return errors.E("versionedEncoder.writeVersion", errors.K.Invalid.Default(), err, "reason", "failed to write version")
	}
	return nil
}

func (e *versionedEncoder) EncodeVersioned(version uint, obj interface{}) error {
	err := e.writeVersion(version)
	if err == nil {
		err = e.encoder.Encode(obj)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

//goland:noinspection GoExportedFuncWithUnexportedType
func NewVersionedDecoder(reader io.Reader, decoder Decoder) *versionedDecoder {
	return &versionedDecoder{
		reader:  reader,
		decoder: decoder,
	}
}

type versionedDecoder struct {
	reader           io.Reader
	decoder          Decoder
	versionsDisabled bool
}

func (d *versionedDecoder) readVersion() (version uint, err error) {
	if d.versionsDisabled {
		return 0, nil
	}

	err = d.decoder.Decode(&version)
	if err != nil {
		return 0, errors.E("versionedDecoder.readVersion", errors.K.Invalid.Default(), err, "reason", "failed to read version")
	}
	return version, nil
}

func (d *versionedDecoder) DecodeVersioned(selector func(version uint) interface{}) (obj interface{}, version uint, err error) {
	version, err = d.readVersion()
	if err == nil {
		obj = selector(version)
		err = d.decoder.Decode(obj)
	}
	return obj, version, err
}
