package codecs

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"
	"reflect"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
	mc "github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multicodec/base"
	cd "github.com/ugorji/go/codec"

	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/link"
	// cbor "github.com/whyrusleeping/cbor/go"
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

// Start value for custom CBOR tags. 40-60 is currently unassigned, and they are
// encoded in a single byte.
// See https://www.iana.org/assignments/cbor-tags/cbor-tags.xhtml
const CBORCustomTagsStart = 40

// NewCborCodec creates a new streaming multicodec using the
// CBOR format.
func NewCborCodec() mc.Multicodec {
	return &codec{cborFactoryInstance, []byte("/cbor")}
}

func NewCborEncoder(w io.Writer) *cd.Encoder {
	return cd.NewEncoder(w, cborFactoryInstance)
}

func NewCborDecoder(r io.Reader) *cd.Decoder {
	return cd.NewDecoder(r, cborFactoryInstance)
}

var cborFactoryInstance = func() *cborFactory {
	f := &cborFactory{}
	f.MapType = reflect.TypeOf(map[string]interface{}(nil))
	f.Canonical = true

	for idx, con := range cborConverters {
		err := f.SetInterfaceExt(con.t, uint64(CBORCustomTagsStart+idx), con.c)
		if err != nil {
			panic(errors.E("create cbor factory", err))
		}
	}
	return f
}()

type cborConverter struct {
	t reflect.Type
	c cd.InterfaceExt
}

// NOTE: do not remove or re-order the converters, since their position
//       determines the CBOR tag ID! Only append to the end!
var cborConverters = []cborConverter{
	{reflect.TypeOf((*id.ID)(nil)), &IDConverter{}},
	{reflect.TypeOf((*hash.Hash)(nil)), &HashConverter{}},
	{reflect.TypeOf((*link.Link)(nil)), &LinkConverter{}},
	{reflect.TypeOf((*utc.UTC)(nil)), &UTCConverter{}},
}
