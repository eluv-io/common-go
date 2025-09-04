package codecs

import (
	"encoding/gob"
	"encoding/json"
	"io"
	"reflect"

	"github.com/fxamacker/cbor/v2"
	cd "github.com/ugorji/go/codec"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/link"
	"github.com/eluv-io/common-go/format/token"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

var (
	GobCodec    = makeGobCodec()
	JsonCodec   = makeJsonCodec()
	CborV1Codec = makeCborV1Codec()
	CborV2Codec = makeCborV2Codec()

	GobMultiCodecPath    = "/gob"
	JsonMultiCodecPath   = "/json"
	CborV1MultiCodecPath = "/cbor"
	CborV2MultiCodecPath = "/cborV2"

	GobMultiCodec    = NewMultiCodec(GobCodec, GobMultiCodecPath)
	JsonMultiCodec   = NewMultiCodec(JsonCodec, JsonMultiCodecPath)
	CborV1MultiCodec = NewMultiCodec(CborV1Codec, CborV1MultiCodecPath)
	CborV2MultiCodec = NewMultiCodec(CborV2Codec, CborV2MultiCodecPath)
	// CborV1MuxCodec is the codec producing un-versioned V1 format and supports decoding versioned V2 format.
	CborV1MuxCodec = NewMuxCodec(CborV1MultiCodec.DisableVersions(), CborV2MultiCodec)
	// CborV2MuxCodec is the codec producing versioned V2 format and supports decoding un-versioned V1 format.
	CborV2MuxCodec = NewMuxCodec(CborV2MultiCodec, CborV1MultiCodec.DisableVersions())
)

// NewGobCodec creates a new streaming MultiCodec using the encoding/gob format.
func NewGobCodec() MultiCodec {
	return GobMultiCodec
}

// NewJsonCodec creates a new streaming MultiCodec using the encoding/json format.
func NewJsonCodec() MultiCodec {
	return JsonMultiCodec
}

// NewCborCodec returns a MultiCodec using the CBOR format. It encodes/decodes un-versioned V1 format and supports
// decoding versioned V2 format.
func NewCborCodec() MultiCodec {
	return CborV1MuxCodec
}

// NewCborV2Codec returns a MultiCodec using the CBOR format. It encodes/decodes versioned V2 format and supports
// decoding un-versioned V1 format.
func NewCborV2Codec() MultiCodec {
	return CborV2MuxCodec
}

// MdsImexCodec returns the codec for metadata store exports / imports.
func MdsImexCodec() MultiCodec {
	return GobMultiCodec
}

// CborEncode encodes the given value as CBOR and writes it to the writer without MultiCodec support (i.e. no MultiCodec
// header is written).
func CborEncode(w io.Writer, v interface{}) error {
	return NewCborCodec().Encoder(w).Encode(v)
}

// CborDecode decodes cbor-encoded data from the provided reader into the given data structure. The data is not expected
// to have a MultiCodec header.
func CborDecode(r io.Reader, v interface{}) error {
	return NewCborCodec().Decoder(r).Decode(v)
}

func makeGobCodec() Codec {
	return NewCodec(
		func(w io.Writer) Encoder {
			return gob.NewEncoder(w)
		},
		func(r io.Reader) Decoder {
			return gob.NewDecoder(r)
		},
	)
}

func makeJsonCodec() Codec {
	return NewCodec(
		func(w io.Writer) Encoder {
			return json.NewEncoder(w)
		},
		func(r io.Reader) Decoder {
			return json.NewDecoder(r)
		},
	)
}

func makeCborV1Codec() Codec {
	handle := &cd.CborHandle{}
	handle.MapType = reflect.TypeOf(map[string]interface{}(nil))
	handle.Canonical = true

	for _, con := range cborConverters {
		err := handle.SetInterfaceExt(con.typ, con.tag, con.converter)
		if err != nil {
			panic(errors.E("create cbor factory", err))
		}
	}
	return NewCodec(
		func(w io.Writer) Encoder {
			return cd.NewEncoder(w, handle)
		},
		func(r io.Reader) Decoder {
			return cd.NewDecoder(r, handle)
		},
	)
}

type cborConverter struct {
	tag       uint64
	typ       reflect.Type
	converter cd.InterfaceExt
}

// cborConverters is the list of converters used for the CborV1Codec
//
// NOTE: do not remove or re-order the converters, since their position
// determines the CBOR tag ID! Only append to the end!
var cborConverters = []cborConverter{
	// Custom CBOR tags 40-60 are currently unassigned, and they are
	// encoded in a single byte.
	// See https://www.iana.org/assignments/cbor-tags/cbor-tags.xhtml
	{40, reflect.TypeOf((*id.ID)(nil)), &IDConverter{}},
	{41, reflect.TypeOf((*hash.Hash)(nil)), &HashConverter{}},
	{42, reflect.TypeOf((*link.Link)(nil)), &LinkConverter{}},
	{43, reflect.TypeOf((*utc.UTC)(nil)), &UTCConverter{}},
}

////////////////////////////////////////////////////////////////////////////////

func makeCborV2Codec() Codec {
	var err error
	tagSet := cbor.NewTagSet()
	options := cbor.TagOptions{
		DecTag: cbor.DecTagOptional, // allows en/decoding registered types to/from CBOR NULL
		EncTag: cbor.EncTagRequired,
	}

	tags := []struct {
		tag uint64
		typ reflect.Type
	}{ // do not change existing tag IDs!
		{40, reflect.TypeOf((*id.ID)(nil))},
		{41, reflect.TypeOf((*hash.Hash)(nil))},
		{42, reflect.TypeOf((*link.Link)(nil))},
		{43, reflect.TypeOf((*utc.UTC)(nil))},
		{44, reflect.TypeOf((*token.Token)(nil))},
	}

	for _, tag := range tags {
		err = tagSet.Add(options, tag.typ, tag.tag)
		if err != nil {
			log.Fatal("invalid cbor tag", err, "tag", tag)
		}
	}

	encOptions := cbor.CoreDetEncOptions()
	encOptions.HandleTagForMarshaler = true
	encOptions.Time = cbor.TimeUnixMicro
	encOptions.TimeTag = cbor.EncTagRequired
	enc, err := encOptions.EncModeWithTags(tagSet)
	if err != nil {
		log.Fatal("failed to create cbor encoder mode", err)
	}

	dec, err := cbor.DecOptions{
		DefaultMapType:          reflect.TypeOf((map[string]interface{})(nil)),
		HandleTagForUnmarshaler: true,
		MaxArrayElements:        1024 * 1024, // github.com/fxamacker/cbor/v2 default is 128 * 1024
		MaxMapPairs:             1024 * 1024, // github.com/fxamacker/cbor/v2 default is 128 * 1024
		MaxNestedLevels:         100,         // github.com/fxamacker/cbor/v2 default is 32
	}.DecModeWithTags(tagSet)
	if err != nil {
		log.Fatal("failed to create cbor decoder mode", err)
	}

	return NewCodec(
		func(w io.Writer) Encoder {
			return enc.NewEncoder(w)
		}, func(r io.Reader) Decoder {
			return dec.NewDecoder(r)
		})
}
