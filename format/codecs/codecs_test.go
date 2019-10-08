package codecs

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	mc "github.com/multiformats/go-multicodec"
)

func TestGobCodec(t *testing.T) {
	runCodecTest(t, NewGobCodec())
}

func TestCborCodec(t *testing.T) {
	runCodecTest(t, NewCborCodec())

	c := NewCborCodec()
	buf, data := encode(c)
	decodeInterface(c, buf, data, t)
}

func runCodecTest(t *testing.T, c mc.Multicodec) {
	buf, data := encode(c)
	decode(c, buf, data, t)
}

func encode(c mc.Multicodec) (bytes.Buffer, []string) {
	var buf bytes.Buffer
	var data []string
	e := c.Encoder(&buf)
	for i := 0; i < 20; i++ {
		s := fmt.Sprintf("String %02d %b", i, i)
		data = append(data, s)
		e.Encode(s)
	}
	encoded := buf.Bytes()
	fmt.Printf("Encoded size %d\n", len(encoded))
	// fmt.Printf("Encoded size %d\n%s\n", len(encoded), string(encoded))
	return buf, data
}

func decode(c mc.Multicodec, buf bytes.Buffer, data []string, t *testing.T) {
	d := c.Decoder(&buf)
	var val string
	for _, s := range data {
		err := d.Decode(&val)
		assert.NoError(t, err)
		assert.Equal(t, s, val)
	}
	err := d.Decode(&val)
	assert.NotNil(t, err)
}

func decodeInterface(c mc.Multicodec, buf bytes.Buffer, data []string, t *testing.T) {
	d := c.Decoder(&buf)
	var val interface{}
	for _, s := range data {
		err := d.Decode(&val)
		assert.NoError(t, err)
		assert.Equal(t, s, val)
	}
	err := d.Decode(&val)
	assert.NotNil(t, err)
}
