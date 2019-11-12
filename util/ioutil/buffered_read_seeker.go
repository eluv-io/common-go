package ioutil

import (
	"bufio"
	"io"
)

var _ io.ReadSeeker = (*BufferedReadSeeker)(nil)
var _ io.Closer = (*BufferedReadSeeker)(nil)

// BufferedReadSeeker implement a buffered reader
// Each call to Seek reset the internal reader.
type BufferedReadSeeker struct {
	rd   io.ReadSeeker
	rbuf *bufio.Reader
}

// NewBufferedReadSeeker returns a BufferedReadSeeker that wraps the given
// io.ReadSeeker and will buffer the amount of data indicated by size
func NewBufferedReadSeeker(rd io.ReadSeeker, size int) *BufferedReadSeeker {
	return &BufferedReadSeeker{
		rd:   rd,
		rbuf: bufio.NewReaderSize(rd, size),
	}
}

func (b BufferedReadSeeker) Read(p []byte) (n int, err error) {
	return b.rbuf.Read(p)
}

func (b BufferedReadSeeker) Seek(offset int64, whence int) (int64, error) {
	ret, err := b.rd.Seek(offset, whence)
	// reset unconditionally
	b.rbuf.Reset(b.rd)
	return ret, err
}

func (b BufferedReadSeeker) Close() error {
	if cl, ok := b.rd.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}
