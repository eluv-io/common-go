package codecs

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGobCodec(t *testing.T) {
	runCodecTest(t, NewGobCodec())
}

func TestCborCodec(t *testing.T) {
	runCodecTest(t, NewCborCodec())
	runCodecTestInterface(t, NewCborCodec())
}

// TestCborV1Codec encodes data with CborV1MultiCodec and decodes it with CborMuxCodec.
func TestCborV1Codec(t *testing.T) {
	{
		buf, data := encode(t, CborV1MultiCodec)
		decode(CborMuxCodec, buf, data, t)
	}
	{
		buf, data := encode(t, CborV1MultiCodec)
		decodeInterface(CborMuxCodec, buf, data, t)
	}
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
