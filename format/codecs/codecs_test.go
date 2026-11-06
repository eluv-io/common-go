package codecs

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/utc-go"
)

func TestGobCodec(t *testing.T) {
	runCodecTest(t, NewGobCodec())
}

func TestCborCodec(t *testing.T) {
	runCodecTest(t, NewCborV2Codec())
	runCodecTestInterface(t, NewCborV2Codec())
}

// TestCborV1Codec encodes data with CborV1MultiCodec and decodes it with CborV2MuxCodec.
func TestCborV1Codec(t *testing.T) {
	{
		buf, data := encode(t, CborV1MultiCodec)
		decode(CborV2MuxCodec, buf, data, t)
	}
	{
		buf, data := encode(t, CborV1MultiCodec)
		decodeInterface(CborV2MuxCodec, buf, data, t)
	}
}

// TestCborV2Unmarshal checks that CborV2Unmarshal decodes like a decoder of CborV2Codec, including the registered tags.
func TestCborV2Unmarshal(t *testing.T) {
	val := map[string]any{
		"id":   id.Generate(id.Q),
		"time": utc.UnixMilli(1_700_000_000_000),
		"list": []any{"a", uint64(1), map[string]any{"b": true}},
	}
	var buf bytes.Buffer
	require.NoError(t, CborV2Codec.Encoder(&buf).Encode(val))

	var exp any
	require.NoError(t, CborV2Codec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&exp))
	var res any
	require.NoError(t, CborV2Unmarshal(buf.Bytes(), &res))
	require.Equal(t, exp, res)
	require.Equal(t, val["id"], res.(map[string]any)["id"])

	// unlike a decoder, CborV2Unmarshal rejects data after the first data item
	require.Error(t, CborV2Unmarshal(append(buf.Bytes(), buf.Bytes()...), &res))
}

func runCodecTest(t *testing.T, c MultiCodec) {
	buf, data := encode(t, c)
	decode(c, buf, data, t)
}

func runCodecTestInterface(t *testing.T, c MultiCodec) {
	buf, data := encode(t, c)
	decodeInterface(c, buf, data, t)
}

func encode(t *testing.T, c MultiCodec) (bytes.Buffer, []string) {
	var buf bytes.Buffer
	var data []string
	e := c.Encoder(&buf)
	for i := 0; i < 20; i++ {
		s := fmt.Sprintf("String %02d %b", i, i)
		data = append(data, s)
		err := e.Encode(s)
		require.NoError(t, err)
	}
	encoded := buf.Bytes()
	fmt.Printf("Encoded size %d\n", len(encoded))
	// fmt.Printf("Encoded size %d\n%s\n", len(encoded), string(encoded))
	return buf, data
}

func decode(c MultiCodec, buf bytes.Buffer, data []string, t *testing.T) {
	d := c.Decoder(&buf)
	var val string
	for _, s := range data {
		err := d.Decode(&val)
		require.NoError(t, err)
		require.Equal(t, s, val)
	}
	err := d.Decode(&val)
	require.NotNil(t, err)
}

func decodeInterface(c MultiCodec, buf bytes.Buffer, data []string, t *testing.T) {
	d := c.Decoder(&buf)
	var val interface{}
	for _, s := range data {
		err := d.Decode(&val)
		require.NoError(t, err)
		require.Equal(t, s, val)
	}
	err := d.Decode(&val)
	require.NotNil(t, err)
}
