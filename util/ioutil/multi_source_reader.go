package ioutil

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
)

var bufPools sync.Map

var _ io.ReadCloser = (*MultiSourceReader)(nil)

// MultiSourceReader returns a io.ReadCloser that reads data from multiple identical source readers. Source readers may
// provide data at different variable rates, so MultiSourceReader returns data as it is made available from any source.
// Additional source readers may be added at anytime via Add; Add may be called concurrently with Read or Close (but
// Read and Close must not be called concurrently). If any of the sources return an error (from reading or closing),
// Read will return that error. Each source will be closed immediately once the source is fully read or errors. Close
// will close any sources that have not yet been closed. If more than one error occurs when reading or closing sources,
// only the first error encountered will be returned.
func NewMultiSourceReader(readers []io.ReadCloser, bufSize ...int) *MultiSourceReader {
	r := &MultiSourceReader{}
	r.reads = make(chan *multiSourceRead, 32)
	r.done = make(chan bool)

	r.bufSize = 1024
	if len(bufSize) > 0 && bufSize[0] > 0 {
		r.bufSize = bufSize[0]
	}
	bufPool, _ := bufPools.LoadOrStore(r.bufSize, byteutil.NewPool(r.bufSize+1))
	r.bufPool = bufPool.(*byteutil.Pool)

	for _, reader := range readers {
		r.Add(reader)
	}

	return r
}

type MultiSourceReader struct {
	n       atomic.Uint32
	wg      sync.WaitGroup
	reads   chan *multiSourceRead
	done    chan bool
	read    *multiSourceRead
	off     int64
	err     error
	errors  []error
	closed  bool
	bufSize int
	bufPool *byteutil.Pool
}

type multiSourceRead struct {
	data []byte
	off  int64
	err  error
	buf  []byte
}

func (r *MultiSourceReader) Add(reader io.ReadCloser) {
	// Start reader goroutine to read from reader and push to reads channel
	r.n.Add(1)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		off := int64(0)
		errored := false
		for {
			buf := r.acquireBuf()
			n, err := reader.Read(buf)
			select {
			case _ = <-r.done:
				r.releaseBuf(buf)
				err = io.ErrUnexpectedEOF // Used only to break from loop
			case r.reads <- &multiSourceRead{data: buf[:n], off: off, err: err, buf: buf}:
				off += int64(n)
				if err != nil {
					errored = true
				}
			}
			if err != nil {
				break
			}
		}
		err := reader.Close()
		if err != nil && !errored {
			select {
			case _ = <-r.done:
			case r.reads <- &multiSourceRead{off: -1, err: err}:
				errored = true
			}
		}
	}()
}

func (r *MultiSourceReader) Read(p []byte) (int, error) {
	e := errors.T("multi-source read", errors.K.Invalid.Default())
	processErr := func(err error) bool {
		if err == io.EOF {
			r.err = io.EOF
		} else if err != nil {
			r.errors = append(r.errors, err)
			if len(r.errors) == int(r.n.Load()) {
				r.err = e("errors", r.errors)
			}
		}
		return r.err != nil
	}
	if r.closed {
		return 0, e("reason", "closed")
	} else if r.err != nil {
		return 0, r.err
	}
	n := 0
	waitRead := true
	if r.read != nil {
		// Process previous partial read
		read := r.read
		n = copy(p, read.data)
		r.off += int64(n)
		if n == len(read.data) {
			r.read = nil
			r.releaseBuf(read.buf)
			if processErr(read.err) {
				return n, r.err
			}
		}
		if n == len(p) {
			if r.read != nil {
				r.read.data = r.read.data[n:]
				r.read.off += int64(n)
			}
			return n, nil
		}
		waitRead = false
	}
	// Process reads channel until next valid read found
	for {
		var read *multiSourceRead
		if waitRead {
			read = <-r.reads
		} else {
			select {
			case read = <-r.reads:
			default:
			}
		}
		if read == nil {
			break
		} else if read.off > r.off {
			r.releaseBuf(read.buf)
			return n, e("reason", "missing bytes", "off", r.off, "read_off", read.off)
		} else if len(read.data) == 0 && read.err != nil {
			r.releaseBuf(read.buf)
			if processErr(read.err) {
				return n, r.err
			}
			continue
		} else if r.off >= read.off+int64(len(read.data)) {
			r.releaseBuf(read.buf)
			continue
		}
		waitRead = false
		if r.off > read.off {
			x := r.off - read.off
			read.data = read.data[x:]
			read.off = r.off
		}
		x := copy(p[n:], read.data)
		n += x
		r.off += int64(x)
		if x == len(read.data) {
			read.data = nil
			r.releaseBuf(read.buf)
			if processErr(read.err) {
				return n, r.err
			}
		}
		if n == len(p) {
			if read.data != nil {
				read.data = read.data[x:]
				read.off += int64(x)
				r.read = read
			}
			break
		}
	}
	return n, nil
}

func (r *MultiSourceReader) Close() error {
	// Reader goroutines are responsible for closing their own readers, so signal done and wait for completion
	var err error
	if !r.closed {
		r.closed = true
		close(r.done)
		r.wg.Wait()
		// Attempt to fully drain reads channel
		// Worst case, may miss unreleased buffers and errors from newly-added sources
		for len(r.reads) > 0 {
			read := <-r.reads
			if read.buf != nil {
				r.releaseBuf(read.buf)
			} else if err == nil && read.off < 0 && read.err != nil {
				err = read.err
			}
		}
	}
	return err
}

func (r *MultiSourceReader) acquireBuf() []byte {
	return r.bufPool.Get()[:r.bufSize]
}

func (r *MultiSourceReader) releaseBuf(buf []byte) {
	r.bufPool.Put(buf[:r.bufSize+1])
}
