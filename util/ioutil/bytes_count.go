package ioutil

import "io"

var _ io.ReadCloser = (*BytesCountReader)(nil)

// BytesCountReader is a wrapper around an io.Reader that counts bytes read
type BytesCountReader struct {
	io.Reader
	BytesCount uint64
}

func (b *BytesCountReader) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if n > 0 {
		b.BytesCount += uint64(n)
	}
	return n, err
}

func (b *BytesCountReader) Close() error {
	if cl, ok := b.Reader.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}
