package ioutil

import (
	"io"
	"sync"
)

// Discard is a Writer on which all Write calls succeed without doing anything.
// Discard is like io.Discard but reports the count of written bytes
type Discard struct {
	BytesCount int64
}

// discard implements ReaderFrom as an optimization so Copy to
// io.Discard can avoid doing unnecessary work.
var _ io.ReaderFrom = (*Discard)(nil)

func (d *Discard) Write(p []byte) (int, error) {
	d.BytesCount += int64(len(p))
	return len(p), nil
}

func (d *Discard) WriteString(s string) (int, error) {
	d.BytesCount += int64(len(s))
	return len(s), nil
}

var blackHolePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 128*1024)
		return &b
	},
}

func (d *Discard) ReadFrom(r io.Reader) (n int64, err error) {
	bufp := blackHolePool.Get().(*[]byte)
	readSize := 0
	for {
		readSize, err = r.Read(*bufp)
		n += int64(readSize)
		d.BytesCount += int64(readSize)
		if err != nil {
			blackHolePool.Put(bufp)
			if err == io.EOF {
				return n, nil
			}
			return
		}
	}
}
