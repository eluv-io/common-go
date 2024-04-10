package ioutil

import (
	"io"

	"github.com/eluv-io/errors-go"
)

// LimitedWriter is a writer that limits the number of bytes that can be written to it. Any calls to Write will fail if
// the limit would be exceeded if the full byte slice was written (no bytes will be written in that case).
type LimitedWriter struct {
	Writer  io.Writer
	Limit   int
	Written int
}

// NewLimitedWriter creates a LimitedWriter with the given limit.
func NewLimitedWriter(w io.Writer, limit int) io.Writer {
	return &LimitedWriter{
		Writer: w,
		Limit:  limit,
	}
}

func (l *LimitedWriter) Write(bts []byte) (n int, err error) {
	written := l.Written + len(bts)
	if written > l.Limit {
		return 0, errors.E("limitedWriter.write", errors.K.IO, "reason", "limit exceeded", "limit", l.Limit)
	}
	l.Written = written
	return l.Writer.Write(bts)
}
