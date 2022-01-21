package ioutil

import (
	"io"

	"github.com/eluv-io/errors-go"
)

// NewRepeatingReader returns a reader that generates len bytes in total,
// repeating the content of the provided buf as necessary.
//
// Example:
//   NewRepeatingReader([]byte("1234567890"), 24)
// will generate the byte sequence
//   123456789012345678901234
func NewRepeatingReader(buf []byte, len int64) ReadSeekCloser {
	return &repeatingReader{buf: buf, len: len}
}

type repeatingReader struct {
	buf []byte
	idx int64
	len int64
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	bufLen := int64(len(r.buf))
	var total int64
	for r.idx < r.len && len(p) > 0 {
		n := int64(len(p))
		if n > r.len-r.idx {
			n = r.len - r.idx
		}
		for n > 0 {
			start := r.idx % bufLen
			end := start + n
			if end > bufLen {
				end = bufLen
			}
			copied := int64(copy(p, r.buf[start:end]))
			total += copied
			n -= copied
			r.idx += copied
			p = p[copied:]
		}
	}
	if r.idx < r.len {
		return int(total), nil
	}
	return int(total), io.EOF
}

func (r *repeatingReader) Seek(offset int64, whence int) (int64, error) {
	var idx int64
	switch whence {
	case io.SeekCurrent:
		idx = r.idx + offset
	case io.SeekStart:
		idx = offset
	case io.SeekEnd:
		idx = r.len - offset
	}
	if idx < 0 {
		return r.idx, errors.E("seek", errors.K.Invalid, "reason", "index out of bounds")
	}
	r.idx = idx
	return r.idx, nil
}

func (r *repeatingReader) Close() error {
	r.idx = r.len
	return nil
}
