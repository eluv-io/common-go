package ioutil

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBytesCount(t *testing.T) {
	buf := make([]byte, 1234)
	bc := &BytesCountReader{
		Reader: bytes.NewReader(buf),
	}

	n, err := bc.Read(make([]byte, 1))
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, uint64(1), bc.BytesCount)

	n, err = bc.Read(make([]byte, 2048))
	require.Equal(t, 1233, n)
	require.Equal(t, uint64(1234), bc.BytesCount)

	_, err = bc.Read(make([]byte, 1))
	require.Equal(t, io.EOF, err)
}
