package codecs

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"
	"reflect"

	"github.com/fxamacker/cbor/v2"
	mc "github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multicodec/base"
	cd "github.com/ugorji/go/codec"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/link"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

type codecFactory interface {
	makeEncoder(w io.Writer, path []byte) mc.Encoder
	makeDecoder(r io.Reader, path []byte) mc.Decoder
}

///////////////////////////////////////////////////////////////////////////////

var _ mc.Multicodec = (*codec)(nil) // ensure codec implements Multicodec!

type codec struct {
	codecFactory
	path []byte
}

func (c *codec) Encoder(w io.Writer) mc.Encoder {
	return c.makeEncoder(w, c.path)
}

func (c *codec) Decoder(r io.Reader) mc.Decoder {
	return c.makeDecoder(r, c.path)
}

func (c *codec) Header() []byte {
	return mc.Header(c.path)
}

///////////////////////////////////////////////////////////////////////////////

type decoder struct {
	r          io.Reader
	path       []byte
	headerRead bool
}

func (d *decoder) Decode(v interface{}) error {
	out, ok := v.([]byte)
	if !ok {
		// return base.ErrExpectedByteSlice
		return fmt.Errorf("expected []byte as input, but received %T", v)
	}

	if !d.headerRead {
		header, err := mc.ReadHeader(d.r)
		if err != nil {
			return err
		}
		if !bytes.Equal(header[1:len(header)-1], d.path) {
			return mc.ErrHeaderInvalid
		}
		d.headerRead = true
	}

	_, err := d.r.Read(out)

	return err
}

///////////////////////////////////////////////////////////////////////////////

type encoder struct {
	w             io.WriteCloser
	path          []byte
	headerWritten bool
}

func (e *encoder) Encode(v interface{}) error {
	slice, ok := v.([]byte)
	if !ok {
		return base.ErrExpectedByteSlice
	}
	if !e.headerWritten {
		err := mc.WriteHeader(e.w, e.path)
		if err != nil {
			return err
		}
		e.headerWritten = true
	}
	_, err := e.w.Write(slice)
	if err != nil {
		return err
	}
	return e.w.Close()
}

///////////////////////////////////////////////////////////////////////////////

type wrappedEncoder struct {
	w             io.Writer
	encoder       mc.Encoder
	path          []byte
	headerWritten bool
}

func (e *wrappedEncoder) Encode(v interface{}) error {
	if !e.headerWritten {
		err := mc.WriteHeader(e.w, e.path)
		if err != nil {
			return err
		}
		e.headerWritten = true
	}
	return e.encoder.Encode(v)
}

///////////////////////////////////////////////////////////////////////////////

type wrappedDecoder struct {
	r          io.Reader
	decoder    mc.Decoder
	path       []byte
	headerRead bool
}

func (d *wrappedDecoder) Decode(v interface{}) error {
	if !d.headerRead {
		header, err := mc.ReadHeader(d.r)
		if err != nil {
			return err
		}
		if !bytes.Equal(header[1:len(header)-1], d.path) {
			return mc.ErrHeaderInvalid
		}
		d.headerRead = true
	}

	return d.decoder.Decode(v)
}

///////////////////////////////////////////////////////////////////////////////

// PENDING(LUK): doesn't currently work. Encoder must write separator (e.g. \n)
// after base64 encoding of the value, so that decoder knows how far to read...
type base64Factory struct{}

func (c *base64Factory) makeEncoder(w io.Writer, path []byte) mc.Encoder {
	return &encoder{w: base64.NewEncoder(base64.StdEncoding, w), path: path}
}

func (c *base64Factory) makeDecoder(r io.Reader, path []byte) mc.Decoder {
	return &decoder{r: base64.NewDecoder(base64.StdEncoding, r), path: path}
}

// NewBase64Codec creates a new base 64 codec.
func NewBase64Codec() mc.Multicodec {
	return &codec{codecFactory: &base64Factory{}, path: []byte("/base64")}
}

///////////////////////////////////////////////////////////////////////////////

type gobFactory struct{}

func (c *gobFactory) makeEncoder(w io.Writer, path []byte) mc.Encoder {
	return &wrappedEncoder{w: w, encoder: gob.NewEncoder(w), path: path}
}

func (c *gobFactory) makeDecoder(r io.Reader, path []byte) mc.Decoder {
	return &wrappedDecoder{r: r, decoder: gob.NewDecoder(r), path: path}
}

// NewGobCodec creates a new streaming multicodec using the encoding/gob format.
func NewGobCodec() mc.Multicodec {
	return &codec{&gobFactory{}, []byte("/gob")}
}

///////////////////////////////////////////////////////////////////////////////

type cborFactory struct {
	cd.CborHandle
}

func (c *cborFactory) makeEncoder(w io.Writer, path []byte) mc.Encoder {
	return &wrappedEncoder{w: w, encoder: cd.NewEncoder(w, c), path: path}
}

func (c *cborFactory) makeDecoder(r io.Reader, path []byte) mc.Decoder {
	return &wrappedDecoder{r: r, decoder: cd.NewDecoder(r, c), path: path}
}

// NewCborCodec creates a new streaming multicodec using the
// CBOR format.
func NewCborCodec() mc.Multicodec {
	return NewMuxCodec(
		&codec{cborV2FactoryInstance, []byte("/cborV2")},
		&codec{cborFactoryInstance, []byte("/cbor")},
	)
}

func NewCborCodecV1() mc.Multicodec {
	return &codec{cborFactoryInstance, []byte("/cbor")}
}

// CborEncode encodes the given value as CBOR and writes it to the writer without multicodec support (i.e. no multicodec
// header is written).
func CborEncode(w io.Writer, v interface{}) error {
	return cborV2FactoryInstance.encMode.NewEncoder(w).Encode(v)
}

// CborDecode decodes cbor-encoded data from the provided reader into the given data structure. The data is not expected
// to have a multicodec header.
func CborDecode(r io.Reader, v interface{}) error {
	return cborV2FactoryInstance.decMode.NewDecoder(r).Decode(v)
}

var cborFactoryInstance = func() *cborFactory {
	f := &cborFactory{}
	f.MapType = reflect.TypeOf(map[string]interface{}(nil))
	f.Canonical = true

	for _, con := range cborConverters {
		err := f.SetInterfaceExt(con.typ, con.tag, con.converter)
		if err != nil {
			panic(errors.E("create cbor factory", err))
		}
	}
	return f
}()

type cborConverter struct {
	tag       uint64
	typ       reflect.Type
	converter cd.InterfaceExt
}

// NOTE: do not change the CBOR tag ID of existing converters!
var cborConverters = []cborConverter{
	// Custom CBOR tags 40-60 are currently unassigned, and they are
	// encoded in a single byte.
	// See https://www.iana.org/assignments/cbor-tags/cbor-tags.xhtml
	{40, reflect.TypeOf((*id.ID)(nil)), &IDConverter{}},
	{41, reflect.TypeOf((*hash.Hash)(nil)), &HashConverter{}},
	{42, reflect.TypeOf((*link.Link)(nil)), &LinkConverter{}},
	{43, reflect.TypeOf((*utc.UTC)(nil)), &UTCConverter{}},
}

///////////////////////////////////////////////////////////////////////////////

type cborV2Factory struct {
	encMode cbor.EncMode
	decMode cbor.DecMode
}

func (c *cborV2Factory) makeEncoder(w io.Writer, path []byte) mc.Encoder {
	return &wrappedEncoder{w: w, encoder: c.encMode.NewEncoder(w), path: path}
}

func (c *cborV2Factory) makeDecoder(r io.Reader, path []byte) mc.Decoder {
	return &wrappedDecoder{r: r, decoder: c.decMode.NewDecoder(r), path: path}
}

var cborV2FactoryInstance = func() *cborV2Factory {
	var err error
	tagSet := cbor.NewTagSet()
	options := cbor.TagOptions{
		DecTag: cbor.DecTagOptional, // allows en/decoding registered types to/from CBOR NULL
		EncTag: cbor.EncTagRequired,
	}

	tags := []struct {
		tag uint64
		typ reflect.Type
	}{
		{40, reflect.TypeOf((*id.ID)(nil))},
		{41, reflect.TypeOf((*hash.Hash)(nil))},
		{42, reflect.TypeOf((*link.Link)(nil))},
		{43, reflect.TypeOf((*utc.UTC)(nil))},
	}

	for _, tag := range tags {
		err = tagSet.Add(options, tag.typ, tag.tag)
		if err != nil {
			log.Fatal("invalid cbor tag", err, "tag", tag)
		}
	}

	encOptions := cbor.CoreDetEncOptions()
	encOptions.HandleTagForMarshaler = true
	enc, err := encOptions.EncModeWithTags(tagSet)
	if err != nil {
		log.Fatal("failed to create cbor encoder mode", err)
	}

	dec, err := cbor.DecOptions{
		DefaultMapType:          reflect.TypeOf((map[string]interface{})(nil)),
		HandleTagForUnmarshaler: true,
	}.DecModeWithTags(tagSet)
	if err != nil {
		log.Fatal("failed to create cbor decoder mode", err)
	}

	f := &cborV2Factory{
		encMode: enc,
		decMode: dec,
	}
	return f
}()

var CborEncoder = cborV2FactoryInstance.encMode
var CborDecoder = cborV2FactoryInstance.decMode
