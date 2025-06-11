package codecs

import (
	"bytes"
	"io"

	"github.com/eluv-io/common-go/format/codecs/header"
	"github.com/eluv-io/errors-go"
)

////////////////////////////////////////////////////////////////////////////////

// MultiCodec is the interface for a Codec that produces and consumes self-describing encodings. During encoding, it
// writes a header as a prefix to the encoded data stream. On decoding, it reads the header and ensures that it
// matches.
//
// Use a MuxCodec in order to support multiple codecs. The MuxCodec chooses an encoder based on a configurable selection
// function. During decoding, it selects the correct decoder based on the MultiCodec header that is read from the byte
// stream.
//
// In addition, a MultiCodec optionally supports versioning by writing a version number to the encoded stream that is
// used on decoding in order to select the correct object to unmarshal to.
type MultiCodec interface {
	Header() header.Header
	Encoder(w io.Writer) Encoder
	Decoder(r io.Reader) Decoder
	VersionedEncoder(w io.Writer) VersionedEncoder
	VersionedDecoder(r io.Reader) VersionedMultiDecoder

	// DisableVersions returns a copy of this MultiCodec that has versioning disabled. It will return a VersionedEncoder
	// that simply ignores the version provided in its encode method, and a VersionedDecoder that will always pass 0 as
	// the version during decoding.
	//
	// This feature is needed in order to support mux codecs where encoding uses versions, but where legacy data was
	// encoded without versions. See CborV2MuxCodec and TestVersionedMux.
	DisableVersions() MultiCodec
}

type VersionedMultiDecoder interface {
	// DecodeVersioned decodes objects from the bytes of the underlying io.Reader. It first reads the version varint and
	// passes it to the given selector function. The selector chooses the target struct to use for unmarshaling the
	// given version and codec path and returns it to the decoder. Finally, the decoder unmarshals the bytes into that
	// struct.
	DecodeVersioned(
		selector func(version uint, codec string) interface{},
	) (
		obj interface{},
		version uint,
		err error,
	)
}

////////////////////////////////////////////////////////////////////////////////

func NewMultiCodec(codec Codec, path string) MultiCodec {
	return &multiCodec{
		codec:  codec,
		header: header.New(path),
	}
}

type multiCodec struct {
	codec           Codec
	header          header.Header
	disableVersions bool
}

func (m *multiCodec) Header() header.Header {
	return m.header
}

func (m *multiCodec) Encoder(w io.Writer) Encoder {
	return NewMultiEncoder(w, m.codec.Encoder(w), m.header.Path())
}

func (m *multiCodec) Decoder(r io.Reader) Decoder {
	return NewMultiDecoder(r, m.codec.Decoder(r), m.header.Path())
}

func (m *multiCodec) VersionedEncoder(w io.Writer) VersionedEncoder {
	enc := NewVersionedMultiEncoder(w, m.codec.Encoder(w), m.header.Path())
	if m.disableVersions {
		enc.versionedEncoder.versionsDisabled = true
	}
	return enc
}

func (m *multiCodec) VersionedDecoder(r io.Reader) VersionedMultiDecoder {
	dec := NewVersionedMultiDecoder(r, m.codec.Decoder(r), m.header.Path())
	if m.disableVersions {
		dec.versionedDecoder.versionsDisabled = true
	}
	return dec
}

func (m *multiCodec) DisableVersions() MultiCodec {
	clone := *m
	clone.disableVersions = true
	return &clone
}

////////////////////////////////////////////////////////////////////////////////

//goland:noinspection GoExportedFuncWithUnexportedType
func NewMultiEncoder(writer io.Writer, encoder Encoder, path string) *multiEncoder {
	return &multiEncoder{
		writer:  writer,
		encoder: encoder,
		header:  header.New(path),
	}
}

type multiEncoder struct {
	writer        io.Writer
	encoder       Encoder
	header        header.Header
	headerWritten bool
}

func (e *multiEncoder) writeHeader() (err error) {
	if !e.headerWritten {
		err = header.WriteHeader(e.writer, e.header)
		if err != nil {
			return err
		}
		e.headerWritten = true
	}
	return nil
}

func (e *multiEncoder) Encode(obj interface{}) error {
	err := e.writeHeader()
	if err == nil {
		err = e.encoder.Encode(obj)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

//goland:noinspection GoExportedFuncWithUnexportedType
func NewVersionedMultiEncoder(writer io.Writer, encoder Encoder, path string) *versionedMultiEncoder {
	versionedEncoder := NewVersionedEncoder(writer, encoder)
	return &versionedMultiEncoder{
		multiEncoder:     NewMultiEncoder(writer, nil, path),
		versionedEncoder: versionedEncoder,
	}
}

type versionedMultiEncoder struct {
	*multiEncoder
	versionedEncoder *versionedEncoder
}

func (v *versionedMultiEncoder) EncodeVersioned(version uint, obj interface{}) error {
	err := v.writeHeader()
	if err == nil {
		err = v.versionedEncoder.EncodeVersioned(version, obj)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

//goland:noinspection GoExportedFuncWithUnexportedType
func NewMultiDecoder(reader io.Reader, decoder Decoder, path string) *multiDecoder {
	return &multiDecoder{
		reader:  reader,
		decoder: decoder,
		header:  header.New(path),
	}
}

type multiDecoder struct {
	reader     io.Reader
	decoder    Decoder
	header     header.Header
	headerRead bool
}

func (d *multiDecoder) readHeader() error {
	if !d.headerRead {
		hdr, err := header.ReadHeader(d.reader)
		if err != nil {
			return err
		}
		if !bytes.Equal(hdr, d.header) {
			return errors.E("multiDecoder.readHeader", errors.K.Invalid.Default(), err,
				"reason", "invalid header",
				"expected", d.header,
				"actual", hdr)
		}
		d.headerRead = true
	}
	return nil
}

func (d *multiDecoder) Decode(obj interface{}) error {
	err := d.readHeader()
	if err == nil {
		err = d.decoder.Decode(obj)
	}
	return err
}

////////////////////////////////////////////////////////////////////////////////

//goland:noinspection GoExportedFuncWithUnexportedType
func NewVersionedMultiDecoder(reader io.Reader, decoder Decoder, path string) *versionedMultiDecoder {
	versionedDecoder := NewVersionedDecoder(reader, decoder)
	return &versionedMultiDecoder{
		multiDecoder: NewMultiDecoder(
			reader,
			nil,
			path,
		),
		versionedDecoder: versionedDecoder,
	}
}

type versionedMultiDecoder struct {
	*multiDecoder
	versionedDecoder *versionedDecoder
}

func (v *versionedMultiDecoder) DecodeVersioned(selector func(version uint, codec string) interface{}) (obj interface{}, version uint, err error) {
	err = v.readHeader()
	if err == nil {
		obj, version, err = v.versionedDecoder.DecodeVersioned(func(version uint) interface{} {
			return selector(version, v.header.Path())
		})
	}
	return obj, version, err
}
