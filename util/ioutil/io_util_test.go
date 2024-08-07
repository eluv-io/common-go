package ioutil_test

import (
	"io"
	"math/rand"
	"testing"

	"github.com/eluv-io/errors-go"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/ioutil"
)

var _ io.ReaderAt = (*testReader)(nil)
var _ io.Reader = (*testReader)(nil)

type testReader struct {
	data      []byte //data
	pos       int    // position of the next byte to read
	count     int    // index one greater than the last valid character in the input
	readChunk int    // max number of bytes to read at once or <=0 if not limited
	readCount int    // the number of times reatAt was called
}

func newTestReader(dataLen int, readChunk int) *testReader {
	d := make([]byte, dataLen)
	rand.Read(d)
	return &testReader{data: d, count: len(d), readChunk: readChunk}
}

func (r *testReader) Read(p []byte) (int, error) {
	n, err := r.ReadAt(p, int64(r.pos))
	r.pos += n
	return n, err
}

func (r *testReader) ReadAt(p []byte, off int64) (n int, err error) {
	r.readCount++
	if len(p) == 0 {
		return 0, errors.E("readAt", errors.K.IO, "reason", "invalid buffer")
	}
	offset := int(off)
	if offset >= r.count {
		return 0, io.EOF
	}
	avail := r.count - offset
	len := len(p)
	if len > avail {
		len = avail
	}
	if r.readChunk > 0 && len > r.readChunk {
		len = r.readChunk
	}
	if len <= 0 {
		// should not be possible
		return 0, nil
	}
	copy(p, r.data[offset:offset+len])
	return len, nil
}

func TestReadAtAtFull(t *testing.T) {

	// first test regular reading of our test class
	rd := newTestReader(12, -1)
	bb := make([]byte, 100)
	n, err := io.ReadFull(rd, bb)
	require.Error(t, err) // we tried to read more than available

	rd = newTestReader(12, -1)
	bb = make([]byte, 12)
	n, err = io.ReadFull(rd, bb)
	require.NoError(t, err)
	require.Equal(t, 12, n)
	require.EqualValues(t, rd.data, bb)

	rd = newTestReader(12, -1)
	bb = make([]byte, 5)
	n, err = io.ReadFull(rd, bb)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.EqualValues(t, rd.data[:5], bb)

	// test readAt
	rd = newTestReader(12, -1)
	bb = make([]byte, 100)
	n, err = ioutil.ReadAtFull(rd, 0, bb)
	require.Error(t, err) // we tried to read more than available

	rd = newTestReader(12, -1)
	bb = make([]byte, 12)
	n, err = ioutil.ReadAtFull(rd, 0, bb)
	require.NoError(t, err)
	require.Equal(t, 12, n)
	require.EqualValues(t, rd.data, bb)
	require.Equal(t, 1, rd.readCount)

	rd = newTestReader(12, 5)
	bb = make([]byte, 12)
	n, err = ioutil.ReadAtFull(rd, 0, bb)
	require.NoError(t, err)
	require.Equal(t, 12, n)
	require.EqualValues(t, rd.data, bb)
	require.Equal(t, 3, rd.readCount)

	rd = newTestReader(12, 2)
	bb = make([]byte, 5)
	n, err = ioutil.ReadAtFull(rd, 3, bb)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.EqualValues(t, rd.data[3:8], bb)
	require.Equal(t, 3, rd.readCount)

	rd.readCount = 0
	n, err = ioutil.ReadAtFull(rd, 7, bb)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.EqualValues(t, rd.data[7:], bb)
	require.Equal(t, 3, rd.readCount)

	n, err = ioutil.ReadAtFull(rd, 8, bb)
	require.Error(t, err)

}
