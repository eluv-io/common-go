package ioutil

import (
	"io"

	"github.com/eluv-io/errors-go"
)

// NewValidatingWriter creates a writer that compares all bytes that are written
// to it with a reference byte stream read from the given reader ref.
// If the bytes differ, or reading from the reference reader returns an error,
// the Write() call will error out.
func NewValidatingWriter(ref io.Reader) io.WriteCloser {
	return &validatingWriter{ref: ref}
}

type validatingWriter struct {
	ref io.Reader
	buf []byte
	off int64
}

func (w *validatingWriter) Close() error {
	return nil
}

func (w *validatingWriter) Write(p []byte) (n int, err error) {
	l := len(p)
	if len(w.buf) < l {
		w.buf = make([]byte, l)
	}
	n, err = io.ReadFull(w.ref, w.buf[:l])
	if err != nil {
		return 0, errors.E("validating write", errors.K.Invalid, err,
			"sub", "read ref stream",
			"off", w.off+int64(n))
	}

	for idx, b := range p {
		if b != w.buf[idx] {
			return 0, errors.E("validating write", errors.K.Invalid, err,
				"reason", "bytes differ",
				"off", w.off+int64(idx),
				"expected", w.buf[idx],
				"actual", b)
		}
	}
	w.off += int64(l)
	return l, nil
}
