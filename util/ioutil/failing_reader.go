package ioutil

import (
	"fmt"
	"io"

	"github.com/eluv-io/errors-go"
)

var (
	_ io.Reader = (*FailingReader)(nil)
	_ io.Closer = (*FailingReader)(nil)
)

// FailingReader is a test utility that fails after reading a given bytes count.
// See NewFailingReader.
type FailingReader struct {
	io.Reader
	failAt     int64
	bytesCount int64
	err        error
}

// NewFailingReader wraps the given io.Reader and fails after having read the given
// bytes count. The returned failure can be provided via the optional error parameter.
func NewFailingReader(r io.Reader, failAt int64, err ...error) *FailingReader {
	var e error
	if len(err) > 0 {
		e = err[0]
	}
	return &FailingReader{
		Reader: r,
		failAt: failAt,
		err:    e,
	}
}

func (r *FailingReader) Read(p []byte) (int, error) {
	if r.bytesCount >= r.failAt {
		if r.err != nil {
			return 0, r.err
		}
		return 0, errors.E("read", errors.K.IO,
			"reason",
			fmt.Sprintf("failing at %d, bytes-count: %d", r.failAt, r.bytesCount))
	}

	if r.bytesCount+int64(len(p)) > r.failAt {
		p = p[:int(r.failAt-r.bytesCount)]
	}
	n, err := r.Reader.Read(p)

	if int64(n)+r.bytesCount >= r.failAt {
		curr := r.bytesCount
		r.bytesCount = r.failAt
		ret := int(r.failAt - curr)
		if r.err != nil {
			return ret, r.err
		}
		return ret,
			errors.E("read", errors.K.IO,
				"reason",
				fmt.Sprintf("failing at %d, bytes-count: %d, actual-read %d", r.failAt, curr, n))
	}
	r.bytesCount += int64(n)
	return n, err
}

func (r *FailingReader) Close() error {
	if cl, ok := r.Reader.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}
