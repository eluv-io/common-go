package ioutil

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
)

var bufPools = make(map[int]*byteutil.Pool)
var bufPoolsMu = sync.RWMutex{}

var _ io.ReadCloser = (*MultiSourceReader)(nil)

// NewMultiSourceReader returns an io.ReadCloser that reads data from multiple identical source readers. Source readers
// may provide data at different variable rates, so MultiSourceReader returns data as it is made available from any
// source. Additional source readers may be added at anytime via Add; Add may be called concurrently with Read or Close
// (but Read and Close must not be called concurrently). If any of the sources return an error (from reading/closing),
// Read will return that error. Each source will be closed immediately once the source is fully read or errors. Close
// will close any sources that have not yet been closed. If more than one error occurs when reading or closing sources,
// only the first error encountered will be returned.
//
// Precondition: a source's Read must only return a bare io.EOF once it has genuinely, completely delivered its
// share of the (shared, identical) stream; any early/abnormal termination (e.g. a dropped connection) must be
// surfaced as a distinguishable, non-io.EOF error instead. Read treats any source's bare io.EOF as proof that the
// whole (shared) stream has ended, and returns immediately without waiting for any other source — so a source
// that violates this precondition will truncate the whole read right there, with no protection against it.
func NewMultiSourceReader(readers []io.ReadCloser, bufferSize ...int) *MultiSourceReader {
	r := &MultiSourceReader{}
	r.readers = make(map[io.ReadCloser]struct{})
	r.reads = make(chan *multiSourceRead, 32)
	r.done = make(chan bool)
	r.errors = &errors.ErrorList{}

	bufSize := 1024
	if len(bufferSize) > 0 && bufferSize[0] > 0 {
		bufSize = bufferSize[0]
	}

	var ok bool
	bufPoolsMu.RLock()
	r.bufPool, ok = bufPools[bufSize]
	bufPoolsMu.RUnlock()
	if !ok {
		bufPoolsMu.Lock()
		r.bufPool, ok = bufPools[bufSize] // Double check in case another reader already made the pool just before
		if !ok {
			r.bufPool = byteutil.NewPool(bufSize)
			bufPools[bufSize] = r.bufPool
		}
		bufPoolsMu.Unlock()
	}

	for _, reader := range readers {
		r.Add(reader)
	}

	return r
}

// MultiSourceReader is an io.ReadCloser that fans in data from multiple identical source readers, each read
// concurrently by its own goroutine (started in Add) into r.reads. Read is the single consumer of that channel: it is
// not safe for concurrent use with itself or with Close, but Add may be called concurrently with either. See
// NewMultiSourceReader for the full contract.
type MultiSourceReader struct {
	// mu guards readers, closed, and add-vs-close/reader-goroutine-completion races.
	mu sync.Mutex
	// readers holds sources not yet closed; used by Close to force-unblock pending Reads.
	readers map[io.ReadCloser]struct{}
	// n is the total number of sources ever added (via Add), including already-completed ones.
	n atomic.Uint32
	// completed is the number of source goroutines that have finished their read loop.
	completed atomic.Uint32
	// wg has one increment per source goroutine; Close waits on this after signaling done.
	wg sync.WaitGroup
	// reads carries chunks read from sources, in the order each source produced them (not globally ordered
	// across sources).
	reads chan *multiSourceRead
	// done is closed by Close to tell source goroutines and a blocked Read to give up.
	done chan bool
	// read is a chunk partially consumed by the previous Read call, carried over to the next one.
	read *multiSourceRead
	// off is the next absolute stream offset Read needs; drives overlap resolution and EOF detection.
	off int64
	// err is the sticky terminal error/EOF once determined; every subsequent Read returns it directly.
	err error
	// errors collects errors from sources; only surfaced once every source has errored.
	errors *errors.ErrorList
	// closed is set to true once Close has been called.
	closed bool
	// bufPool is the pool of read buffers, shared across all MultiSourceReaders using the same buffer size.
	bufPool *byteutil.Pool
}

// multiSourceRead is one chunk read from a source: data starting at absolute stream offset off, and/or the error
// (possibly io.EOF) that ended the source's read loop. buf is the pooled backing buffer for data, released via
// MultiSourceReader.releaseRead once the chunk has been fully consumed or discarded.
type multiSourceRead struct {
	data []byte
	off  int64
	err  error
	buf  *[]byte
}

// Add registers another source reader and starts a dedicated goroutine that reads it to completion, streaming chunks
// into r.reads for Read to consume. It may be called at any time, including concurrently with Read or with other calls
// to Add or Close (but not concurrently with Read, per the type's contract). If the reader is closed already, reader is
// closed immediately and otherwise ignored, matching the "no more sources accepted after Close" contract.
func (r *MultiSourceReader) Add(reader io.ReadCloser) {
	if reader == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = reader.Close()
		return
	}
	r.readers[reader] = struct{}{}
	r.n.Add(1)
	r.wg.Add(1)
	r.mu.Unlock()

	// Start reader goroutine to read from reader and push to reads channel
	go func() {
		off := int64(0) // offset within THIS source, i.e. bytes it has produced so far
		errored := false
		defer func() {
			r.mu.Lock()
			delete(r.readers, reader)
			r.mu.Unlock()
			r.wg.Done()
		}()
		for {
			buf := r.acquireBuf()
			n, err := reader.Read(*buf)
			select {
			case _ = <-r.done:
				r.releaseBuf(buf)
				err = io.ErrUnexpectedEOF // Used only to break from loop
			case r.reads <- r.acquireRead((*buf)[:n], off, err, buf):
				off += int64(n)
				if err != nil && err != io.EOF {
					errored = true
				}
			}
			if err != nil {
				break
			}
		}
		r.completed.Add(1)
		// Close may have already force-closed reader (to unblock the Read call above) without removing it from
		// r.readers, so check-and-delete atomically here to avoid closing it twice: a second Close on a real
		// io.ReadCloser (e.g. a net.Conn) can return a spurious error.
		r.mu.Lock()
		_, active := r.readers[reader]
		if active {
			delete(r.readers, reader)
		}
		r.mu.Unlock()
		if active {
			err := reader.Close()
			if err != nil && !errored {
				select {
				case _ = <-r.done:
				case r.reads <- r.acquireRead(nil, -1, err, nil):
					errored = true
				}
			}
		}
	}()
}

// Read implements io.Reader by copying the next available bytes of the (single, logical) stream into p, sourced from
// whichever source reader produced them. It is the sole consumer of r.reads and must not be called concurrently with
// itself or with Close.
//
// Overall shape of one call:
//  1. First drain any leftover data from a chunk a previous call couldn't fully copy into its p (r.read).
//  2. Then repeatedly pull the next chunk from r.reads, trimming or discarding any part that's already been consumed
//     via an overlapping chunk from another source, copy what fits into p, and loop until p is full or no more data is
//     available right now.
//  3. Return io.EOF as soon as any source cleanly finishes — per the io.EOF precondition documented on
//     NewMultiSourceReader, that's the whole stream's end — or an accumulated error once every source has
//     errored.
func (r *MultiSourceReader) Read(p []byte) (int, error) {
	// e is a closure rather than an errors.T() constructed unconditionally, so Read's hot, no-error path doesn't pay
	// for building one on every call (including a heap allocation due to the varargs on every call)!
	e := func(fields ...any) error {
		return errors.T("multi-source read", errors.K.IO.Default())(fields...)
	}
	// processErr treats a source's bare io.EOF as immediate proof the whole (shared) stream has ended (per the
	// precondition on NewMultiSourceReader), and otherwise records a real error, only turning accumulated errors
	// into r.err once every source has reported one. Returns whether r.err is now set, so callers can
	// short-circuit and return it immediately.
	processErr := func(err error) bool {
		if err == io.EOF {
			r.err = io.EOF
		} else if err != nil {
			r.errors.Append(err)
			if len(r.errors.Errors) == int(r.n.Load()) {
				r.err = e(r.errors)
			}
		}
		return r.err != nil
	}
	if r.closed {
		return 0, e("reason", "closed")
	} else if r.err != nil {
		// r.err is sticky: once EOF or a terminal error has been determined, every subsequent Read repeats it.
		return 0, r.err
	}
	n := 0
	waitRead := true // true while p still has room and no data has been copied into it yet this call
	if r.read != nil {
		// Process previous partial read
		read := r.read
		n = copy(p, read.data)
		r.off += int64(n)
		if n == len(read.data) {
			r.read = nil
			readErr := read.err
			r.releaseRead(read)
			if readErr != nil && processErr(readErr) {
				return n, r.err
			}
		} else {
			// p was smaller than the remaining data; keep the rest for the next Read call.
			r.read.data = r.read.data[n:]
			r.read.off += int64(n)
			return n, nil
		}
		if n == len(p) {
			return n, nil
		}
		waitRead = false
	}

	// Process reads channel until next valid read found
	for {
		var read *multiSourceRead
		if len(r.reads) == 0 && r.completed.Load() >= r.n.Load() {
			// Fallback for the degenerate case (no source was ever added) and as a defensive backstop; in normal
			// operation, io.EOF/errors are handled immediately as each chunk carrying one is processed below, via
			// processErr.
			if r.errors != nil && len(r.errors.Errors) == int(r.n.Load()) {
				r.err = e(r.errors)
			} else if r.err == nil {
				r.err = io.EOF
			}
			return n, r.err
		}
		if waitRead {
			// Nothing copied into p yet: block for more.
			select {
			case read = <-r.reads:
			case <-r.done:
				return n, io.ErrUnexpectedEOF
			}
		} else {
			// p already has some data: don't block further, just take whatever (if anything) is immediately
			// available.
			select {
			case read = <-r.reads:
			default:
			}
		}

		if read == nil {
			break
		} else if read.off > r.off {
			// Unreachable by construction: within a single source, off is exactly that source's own cumulative byte
			// count (contiguous, gap-free), and Add's goroutine sends chunks to r.reads in the exact order it produced
			// them, with Read never dequeuing a source's next chunk until any leftover from its previous one (r.read,
			// above) has been fully drained. So no chunk can ever legitimately start ahead of r.off. If this fires,
			// some internal invariant has been violated — fail loudly rather than silently waiting for a gap that can't
			// close.
			readOff := read.off
			r.releaseRead(read)
			r.err = e("reason", "missing bytes", "off", r.off, "read_off", readOff)
			return n, r.err
		} else if len(read.data) == 0 && read.err != nil {
			// A pure error/EOF marker from a source (no data attached).
			readErr := read.err
			r.releaseRead(read)
			if processErr(readErr) {
				return n, r.err
			}
			continue
		} else if r.off >= read.off+int64(len(read.data)) {
			// Chunk is entirely behind r.off already (redundant data from a slower duplicate source);
			// discard it but still surface any error/EOF it carried.
			readErr := read.err
			r.releaseRead(read)
			if readErr != nil && processErr(readErr) {
				return n, r.err
			}
			continue
		}
		waitRead = false
		if r.off > read.off {
			// Chunk partially overlaps data already consumed from another source; trim to r.off.
			x := r.off - read.off
			read.data = read.data[x:]
			read.off = r.off
		}
		x := copy(p[n:], read.data)
		n += x
		r.off += int64(x)
		if x == len(read.data) {
			readErr := read.err
			r.releaseRead(read)
			if readErr != nil && processErr(readErr) {
				return n, r.err
			}
			if n == len(p) {
				break
			}
		} else if n == len(p) {
			// p is full but the chunk has more; keep the remainder for the next Read call.
			read.data = read.data[x:]
			read.off += int64(x)
			r.read = read
			break
		}
	}
	return n, nil
}

// Close stops and closes every source that hasn't finished/closed itself yet and releases any buffered data. It is
// idempotent (a second call is a no-op) and safe to call concurrently with Add, but not with Read.
func (r *MultiSourceReader) Close() error {
	// Reader goroutines are responsible for closing their own readers, so signal done and wait for completion
	var err error
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.done)
		for reader := range r.readers {
			_ = reader.Close()
			delete(r.readers, reader)
		}
		r.mu.Unlock()

		r.wg.Wait()
		// Attempt to fully drain reads channel
		// Worst case, may miss unreleased buffers and errors from newly-added sources
		for len(r.reads) > 0 {
			read := <-r.reads
			if read.off < 0 && read.err != nil && err == nil {
				err = read.err
			}
			r.releaseRead(read)
		}
	} else {
		r.mu.Unlock()
	}
	return err
}

// acquireBuf gets a buffer from the pool shared by all readers using this instance's buffer size.
func (r *MultiSourceReader) acquireBuf() *[]byte {
	return r.bufPool.Get()
}

// releaseBuf returns buf to the pool, restoring its length first if needed so byteutil.Pool.Put
// doesn't reject it (or reallocate) for having a length other than the pool's configured size.
func (r *MultiSourceReader) releaseBuf(buf *[]byte) {
	if buf != nil {
		if len(*buf) != r.bufPool.BufSize {
			*buf = (*buf)[:r.bufPool.BufSize]
		}
		r.bufPool.Put(buf)
	}
}

// readPool recycles *multiSourceRead structs (across all MultiSourceReader instances) to avoid a heap allocation for
// every chunk read from every source.
var readPool = sync.Pool{
	New: func() any {
		return &multiSourceRead{}
	},
}

// acquireRead gets a *multiSourceRead from readPool and populates it; see releaseRead for the counterpart.
func (r *MultiSourceReader) acquireRead(data []byte, off int64, err error, buf *[]byte) *multiSourceRead {
	mRead := readPool.Get().(*multiSourceRead)
	mRead.data = data
	mRead.off = off
	mRead.err = err
	mRead.buf = buf
	return mRead
}

// releaseRead releases read's backing buffer (if any) back to the buffer pool and returns read itself to readPool.
// read must not be used again after this call.
func (r *MultiSourceReader) releaseRead(read *multiSourceRead) {
	if read != nil {
		if read.buf != nil {
			r.releaseBuf(read.buf)
			read.buf = nil
		}
		read.data = nil
		read.err = nil
		read.off = 0
		readPool.Put(read)
	}
}
