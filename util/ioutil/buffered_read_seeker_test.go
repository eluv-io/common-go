package ioutil

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBufferedReadSeeker(t *testing.T) {
	var size int64 = 102 * 10
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	rp := NewRepeatingReader(data, size)

	// initial data
	orig := bytes.NewBuffer(make([]byte, 0))
	w, err := io.Copy(orig, rp)
	require.NoError(t, err)
	require.Equal(t, size, w)

	// read without buffering
	crs := &countingReadSeeker{
		rd: NewRepeatingReader(data, size),
	}
	d1 := newSimpleWriter()
	w, err = io.CopyBuffer(d1, crs, make([]byte, 10, 10))
	require.NoError(t, err)
	require.Equal(t, size, w)
	require.Equal(t, orig.Bytes(), d1.buf)
	require.Equal(t, 0, crs.seekCount)
	require.Equal(t, int(size/10), crs.readCount)

	// read with buffering
	type tcase struct {
		bufSize      int
		expReadCount int
	}
	tcases := []*tcase{
		{bufSize: int(size * 2), expReadCount: 1},
		{bufSize: int(size), expReadCount: 1},
		{bufSize: int(size / 2), expReadCount: 2},
	}
	for _, tc := range tcases {
		crs = &countingReadSeeker{
			rd: NewRepeatingReader(data, size),
		}
		brs := NewBufferedReadSeeker(crs, tc.bufSize)
		d1 = newSimpleWriter()
		w, err = io.CopyBuffer(d1, brs, make([]byte, 10, 10))
		require.NoError(t, err)
		require.Equal(t, size, w)
		require.Equal(t, orig.Bytes(), d1.buf)
		require.Equal(t, 0, crs.seekCount)
		require.Equal(t, tc.expReadCount, crs.readCount)
		_, err = brs.Read(make([]byte, 1))
		require.Equal(t, io.EOF, err)
	}

	// read & seek with buffering
	crs = &countingReadSeeker{
		rd: NewRepeatingReader(data, size),
	}
	brs := NewBufferedReadSeeker(crs, int(size/4))
	d1 = newSimpleWriter()
	for i := 0; i < 4; i++ {
		off := int64(i) * size / 4
		_, err := brs.Seek(off, io.SeekStart)
		require.NoError(t, err)
		w, err = io.CopyN(d1, brs, size/4)
		require.NoError(t, err, "off %d", off)
		require.Equal(t, size/4, w)
	}
	require.Equal(t, orig.Bytes(), d1.buf)
	require.Equal(t, 4, crs.seekCount)
	require.Equal(t, 4, crs.readCount)
	_, err = brs.Read(make([]byte, 1))
	require.Equal(t, io.EOF, err)
}

type countingReadSeeker struct {
	rd        io.ReadSeeker
	readCount int
	seekCount int
}

var _ io.ReadSeeker = (*countingReadSeeker)(nil)

func (c *countingReadSeeker) Read(p []byte) (n int, err error) {
	ret, err := c.rd.Read(p)
	c.readCount++
	return ret, err
}

func (c *countingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	ret, err := c.rd.Seek(offset, whence)
	c.seekCount++
	return ret, err
}

type simpleWriter struct {
	buf []byte
}

func newSimpleWriter() *simpleWriter {
	return &simpleWriter{buf: make([]byte, 0)}
}

func (s *simpleWriter) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	return len(p), nil
}
