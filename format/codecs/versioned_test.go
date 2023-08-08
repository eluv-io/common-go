package codecs_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/codecs"
)

type TypeV1 string

type TypeV2 []byte

func (t *TypeV2) UnmarshalBinary(bts []byte) error {
	*t = bts
	return nil
}

type TypeV3 struct {
	String string `json:"string"`
}

func TestVersionedCodecs(t *testing.T) {
	testCodecs := []codecs.VersionedCodec{
		codecs.NewVersionedCodec(codecs.JsonCodec),
		codecs.NewVersionedCodec(codecs.CborV1Codec),
		codecs.NewVersionedCodec(codecs.CborV2Codec),
		// GobCodec can't handle TypeV2 for some reason:
		//	gob: decoding into local type *codecs_test.TypeV2, received remote type bytes
		// codecs.NewVersionedCodec(codecs.GobCodec),
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
		t.Run(fmt.Sprintf("sepatate instances %T", testCodec), func(t *testing.T) {
			for _, test := range tests {
				t.Run(fmt.Sprintf("%T", test.obj), func(t *testing.T) {
					var err error
					buf := new(bytes.Buffer)
					enc := testCodec.VersionedEncoder(buf)

					err = enc.EncodeVersioned(test.version, test.obj)
					require.NoError(t, err)
					fmt.Println(buf.String())

					dec := testCodec.VersionedDecoder(buf)
					obj, version, err := dec.DecodeVersioned(func(version uint) interface{} {
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
		t.Run(fmt.Sprintf("single instance %T", testCodec), func(t *testing.T) {
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
				obj, version, err := dec.DecodeVersioned(func(version uint) interface{} {
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

func toValue(obj interface{}) any {
	return reflect.Indirect(reflect.ValueOf(obj)).Interface()
}

func toPrt(obj interface{}) interface{} {
	vp := reflect.New(reflect.TypeOf(obj))
	vp.Elem().Set(reflect.ValueOf(obj))
	return vp.Interface()
}
