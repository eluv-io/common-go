package tlv

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/tlv/tlv"
	"github.com/eluv-io/common-go/util/byteutil"
)

func TestWriteHeaderParseHeader(t *testing.T) {
	var header [3]byte
	err := tlv.WriteHeader(header[:], 0x01, 7)
	require.NoError(t, err)

	typ, length := tlv.ParseHeader(header)
	require.Equal(t, byte(0x01), typ)
	require.Equal(t, uint16(7), length)
}

func TestEncapsulaterDecapsulator(t *testing.T) {
	e := NewTlvEncapsulator(0x01)
	d := NewTlvDecapsulator()

	tests := [][]byte{
		nil,
		{},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09},
		byteutil.RandomBytes(1328),
	}

	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			data := test

			res, err := e.Transform(data)
			require.NoError(t, err)
			require.Len(t, res, 3+len(data))

			res, err = d.Transform(res)
			require.NoError(t, err)

			if data == nil { // encapsulating nil and decapsulating it results in empty slice...
				data = []byte{}
			}
			require.Equal(t, data, res)
		})
	}

}
