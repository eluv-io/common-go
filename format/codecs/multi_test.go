package codecs_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/codecs/header"
	"github.com/eluv-io/errors-go"
)

func TestMultiCodecs(t *testing.T) {
	testCodecs := []codecs.MultiCodec{
		codecs.JsonMultiCodec,
		codecs.CborV1MultiCodec,
		codecs.CborV2MultiCodec,
		codecs.CborMuxCodec,
	}

	tests := []struct {
		version uint
		obj     interface{}
	}{
		{
			version: 0,
			obj:     TypeV1("a string"),
		},
		{
			version: 1,
			obj:     TypeV2("another string"),
		},
		{
			version: 2,
			obj:     TypeV3{"a 3rd string"},
		},
	}

	// encode/decode each object with separate encoder/decoder instance
	for _, testCodec := range testCodecs {
		t.Run(fmt.Sprintf("separate instances %s", header.Path(testCodec.Header())), func(t *testing.T) {
			for _, test := range tests {
				t.Run(fmt.Sprintf("%T", test.obj), func(t *testing.T) {
					var err error
					buf := new(bytes.Buffer)
					enc := testCodec.VersionedEncoder(buf)

					err = enc.EncodeVersioned(test.version, test.obj)
					require.NoError(t, err)
					fmt.Println(buf.String())

					dec := testCodec.VersionedDecoder(buf)
					obj, version, err := dec.DecodeVersioned(func(version uint, codec string) interface{} {
						if mux, ok := testCodec.(*codecs.MuxCodec); ok {
							require.Equal(t, mux.Codecs[0].Header().Path(), codec)
						} else {
							require.Equal(t, testCodec.Header().Path(), codec)
						}
						return reflect.New(reflect.TypeOf(test.obj)).Interface()
					})
					require.NoError(t, err, "version %d", test.version)
					require.EqualValues(t, test.version, version)
					require.Equal(t, test.obj, toValue(obj))
					require.Equal(t, toPrt(test.obj), obj)
				})
			}
		})
	}

	// encode/decode all objects with single encoder/decoder instance
	for _, testCodec := range testCodecs {
		t.Run(fmt.Sprintf("single instance %s", header.Path(testCodec.Header())), func(t *testing.T) {
			var err error
			buf := new(bytes.Buffer)
			enc := testCodec.VersionedEncoder(buf)

			for _, test := range tests {
				err = enc.EncodeVersioned(test.version, test.obj)
				require.NoError(t, err)
			}

			fmt.Println(buf.String())

			dec := testCodec.VersionedDecoder(buf)
			for _, test := range tests {
				obj, version, err := dec.
					DecodeVersioned(func(version uint, codec string) interface{} {
						return reflect.New(reflect.TypeOf(test.obj)).Interface()
					})
				require.NoError(t, err, "version %d", test.version)
				require.EqualValues(t, test.version, version)
				require.Equal(t, test.obj, toValue(obj))
				require.Equal(t, toPrt(test.obj), obj)
			}
		})
	}
}

func TestEncoderDecoderMismatch(t *testing.T) {
	buf := &bytes.Buffer{}
	err := codecs.CborV1MultiCodec.Encoder(buf).Encode("test")
	require.NoError(t, err)

	var target string
	err = codecs.CborV2MultiCodec.Decoder(buf).Decode(&target)
	require.Error(t, err)
	field, ok := errors.GetField(err, "reason")
	require.True(t, ok)
	require.Equal(t, "invalid header", field)
}
