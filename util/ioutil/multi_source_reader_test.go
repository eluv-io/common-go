package ioutil_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
)

func TestMultiSourceReader(t *testing.T) {
	count := 10
	for n := 0; n < count; n++ {
		r := ioutil.NewMultiSourceReader(nil)
		buf := byteutil.RandomBytes(128 * 1024)
		for i := 0; i < 4; i++ {
			r.Add(newTestSourceReader(buf, i == 0))
		}
		b, err := io.ReadAll(r)
		require.NoError(t, err)
		require.Equal(t, buf, b)
		err = r.Close()
		require.NoError(t, err)
		fmt.Printf("%d of %d done\n", n+1, count)
	}

	r := ioutil.NewMultiSourceReader(nil)
	buf := byteutil.RandomBytes(128 * 1024)
	for i := 0; i < 4; i++ {
		r.Add(newTestSourceReader(buf, true))
	}
	b, err := io.ReadAll(r)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed early")
	require.Equal(t, buf[:len(b)], b)
	err = r.Close()
	require.NoError(t, err)
}

func newTestSourceReader(buf []byte, fail bool) io.ReadCloser {
	n := 0
	if fail {
		n = len(buf)/2 + rand.Intn(len(buf)/2) - 1
	}
	return &testSourceReader{r: bytes.NewBuffer(buf), fail: n}
}

type testSourceReader struct {
	mu     sync.Mutex
	r      io.Reader
	off    int
	fail   int
	failed bool
	closed bool
}

func (r *testSourceReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	if r.failed {
		r.mu.Unlock()
		return 0, errors.E("read after fail")
	} else if r.closed {
		r.mu.Unlock()
		return 0, errors.E("read after close")
	}
	r.mu.Unlock()

	time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, errors.E("read after close")
	}
	if r.fail > 0 && r.off >= r.fail {
		r.failed = true
		return 0, errors.E("failed early")
	}
	n, err := r.r.Read(p)
	r.off += n
	return n, err
}

func (r *testSourceReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return nil
}

func TestMultiSourceReader_CloseUnblocksPendingReads(t *testing.T) {
	pr, pw := io.Pipe()
	r := ioutil.NewMultiSourceReader(nil)
	r.Add(pr)

	closeDone := make(chan struct{})
	go func() {
		_ = r.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		// Success!
	case <-time.After(1 * time.Second):
		t.Fatal("r.Close() hung because reader.Read was blocked and reader.Close() was not called")
	}
	_ = pw.Close()
}

// blockingCloseCountingReader blocks in Read until closed, then returns io.ErrClosedPipe. It counts Close calls and
// returns an error on any call after the first, simulating a real io.ReadCloser (e.g. net.Conn, os.File) that is not
// idempotent on double-close.
type blockingCloseCountingReader struct {
	mu      sync.Mutex
	closed  bool
	closes  int
	unblock chan struct{}
}

func newBlockingCloseCountingReader() *blockingCloseCountingReader {
	return &blockingCloseCountingReader{unblock: make(chan struct{})}
}

func (b *blockingCloseCountingReader) Read([]byte) (int, error) {
	<-b.unblock
	return 0, io.ErrClosedPipe
}

func (b *blockingCloseCountingReader) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closes++
	if b.closed {
		return errors.E("already closed")
	}
	b.closed = true
	close(b.unblock)
	return nil
}

func (b *blockingCloseCountingReader) closeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closes
}

func TestMultiSourceReader_CloseDoesNotDoubleCloseSource(t *testing.T) {
	// The underlying race (Close() force-closing a reader that's blocked in Read, while that reader's goroutine also
	// tries to close it) only manifests probabilistically, so repeat.
	for range 100 {
		src := newBlockingCloseCountingReader()
		r := ioutil.NewMultiSourceReader(nil)
		r.Add(src)

		// Give the reader goroutine time to enter the blocking Read call.
		time.Sleep(time.Millisecond)

		err := r.Close()
		require.NoError(t, err)

		// Give the reader goroutine time to finish its own post-Read bookkeeping.
		require.Eventually(t, func() bool {
			return src.closeCount() > 0
		}, time.Second, time.Millisecond)
		time.Sleep(time.Millisecond)

		require.Equal(t, 1, src.closeCount(), "source reader should be closed exactly once")
	}
}

func TestMultiSourceReader_ConcurrentAddAndClose(t *testing.T) {
	for range 50 {
		r := ioutil.NewMultiSourceReader(nil)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			pr, pw := io.Pipe()
			r.Add(pr)
			_ = pw.Close()
		}()

		go func() {
			defer wg.Done()
			_ = r.Close()
		}()

		wg.Wait()
	}
}

func TestMultiSourceReader_AddAfterClose(t *testing.T) {
	r := ioutil.NewMultiSourceReader(nil)
	require.NoError(t, r.Close())

	pr, pw := io.Pipe()
	r.Add(pr) // Should immediately close pr when reader is already closed

	// Validate pr was actually closed by verifying write to pw fails with io.ErrClosedPipe
	_, err := pw.Write([]byte("test"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	_ = pw.Close()
}

type chunkedReader struct {
	chunks [][]byte
	delays []time.Duration
	idx    int
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.idx >= len(c.chunks) {
		return 0, io.EOF
	}
	if c.idx < len(c.delays) && c.delays[c.idx] > 0 {
		time.Sleep(c.delays[c.idx])
	}
	data := c.chunks[c.idx]
	c.idx++
	n := copy(p, data)
	var err error
	if c.idx >= len(c.chunks) {
		err = io.EOF
	}
	return n, err
}

func (c *chunkedReader) Close() error {
	return nil
}

// TestMultiSourceReader_InterleavedArrivals validates overlap resolution (trimming/discarding a chunk that starts at or
// behind r.off, from a redundant source delivering the same range at a different rate) — not out-of-order buffering. A
// chunk can never legitimately arrive strictly ahead of r.off (see the comment on that branch in Read), so there's
// nothing to exercise there. Both sources deliver the same total length, matching the identical-sources contract: since
// any source's io.EOF ends the whole read immediately, a source that finished short (even if only due to test data,
// not a real violation) would truncate this test regardless of arrival order, which isn't what it means to exercise.
func TestMultiSourceReader_InterleavedArrivals(t *testing.T) {
	// Source 1: yields A (0..512, fast), B (512..1024, 100ms delay), C (1024..1536, fast)
	s1 := &chunkedReader{
		chunks: [][]byte{
			bytes.Repeat([]byte("A"), 512),
			bytes.Repeat([]byte("B"), 512),
			bytes.Repeat([]byte("C"), 512),
		},
		delays: []time.Duration{0, 100 * time.Millisecond, 0},
	}

	// Source 2: yields A+B (0..1024, fast), C (1024..1536, fast)
	s2 := &chunkedReader{
		chunks: [][]byte{
			append(bytes.Repeat([]byte("A"), 512), bytes.Repeat([]byte("B"), 512)...),
			bytes.Repeat([]byte("C"), 512),
		},
		delays: []time.Duration{0, 0},
	}

	r := ioutil.NewMultiSourceReader(nil)
	r.Add(s1)
	r.Add(s2)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	expected := append(
		append(bytes.Repeat([]byte("A"), 512), bytes.Repeat([]byte("B"), 512)...),
		bytes.Repeat([]byte("C"), 512)...,
	)
	require.Equal(t, expected, data)
}

// TestMultiSourceReader_EarlyEOFEndsStream demonstrates the accepted behavior of treating any source's bare
// io.EOF as proof the whole (shared) stream has ended, per the precondition documented on NewMultiSourceReader:
// a source that finishes early — here, a genuinely empty one, but the same applies to one that violates the
// precondition — ends the read immediately, even though another source still has real data. Source 1 has no
// delay at all, so its io.EOF is guaranteed to be seen before source 2 (which does have a delay) ever delivers
// anything, making the outcome deterministic.
func TestMultiSourceReader_EarlyEOFEndsStream(t *testing.T) {
	// Source 1: empty stream, returns io.EOF immediately.
	s1 := &chunkedReader{
		chunks: nil,
	}

	// Source 2: a full stream that source 1 preempts from ever being read.
	s2 := &chunkedReader{
		chunks: [][]byte{
			bytes.Repeat([]byte("X"), 512),
			bytes.Repeat([]byte("Y"), 512),
			bytes.Repeat([]byte("Z"), 512),
		},
		delays: []time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond},
	}

	r := ioutil.NewMultiSourceReader(nil)
	r.Add(s1)
	r.Add(s2)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Empty(t, data)
}

// TestMultiSourceReader_PrematureSourceTruncates demonstrates the accepted tradeoff of treating any source's
// bare io.EOF as proof the whole stream has ended: a source that violates the io.EOF precondition documented on
// NewMultiSourceReader — returning it after only part of the stream, the way a dropped connection might — ends
// the read there, even though another source has more real data. sA has no delay at all, so its (premature)
// io.EOF is guaranteed to be seen before sB (whose chunks are all delayed) delivers anything, making the
// truncation point deterministic rather than dependent on arrival order.
func TestMultiSourceReader_PrematureSourceTruncates(t *testing.T) {
	full := append(bytes.Repeat([]byte("A"), 100), bytes.Repeat([]byte("B"), 700)...)

	// sA violates the io.EOF precondition: it stops after 100 bytes and returns io.EOF as if that were the
	// stream's true end.
	sA := &chunkedReader{
		chunks: [][]byte{full[:100]},
	}

	// sB delivers the real, full stream, but more slowly.
	sB := &chunkedReader{
		chunks: [][]byte{
			full[:300],
			full[300:],
		},
		delays: []time.Duration{10 * time.Millisecond, 50 * time.Millisecond},
	}

	r := ioutil.NewMultiSourceReader(nil)
	r.Add(sA)
	r.Add(sB)

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, full[:100], data)
}

// TestMultiSourceReader_LeftoverExactlyFillsBuffer proves that Read's "drain leftover data left over from a
// previous call" path (r.read) actually returns when that leftover exactly fills the caller's next buffer and
// carries no error attached — i.e. that this return is reachable, not dead code.
func TestMultiSourceReader_LeftoverExactlyFillsBuffer(t *testing.T) {
	// Two chunks so the first (20 bytes) carries no error when it becomes the leftover; the second chunk (which
	// carries io.EOF) is irrelevant here and never reached.
	s := &chunkedReader{
		chunks: [][]byte{
			make([]byte, 20),
			make([]byte, 1),
		},
	}

	r := ioutil.NewMultiSourceReader(nil)
	r.Add(s)
	defer func() { _ = r.Close() }()

	// First Read only takes 8 of the chunk's 20 bytes, leaving a 12-byte leftover in r.read with no error attached.
	buf1 := make([]byte, 8)
	n1, err1 := r.Read(buf1)
	require.NoError(t, err1)
	require.Equal(t, 8, n1)

	// Second Read's buffer exactly matches the 12-byte leftover.
	buf2 := make([]byte, 12)
	n2, err2 := r.Read(buf2)
	require.NoError(t, err2)
	require.Equal(t, 12, n2)
}

// infiniteSource is an io.ReadCloser that never runs out of data (or errors), always filling p fully. It exists so
// BenchmarkMultiSourceReader can drive MultiSourceReader.Read directly for b.N iterations without either running
// out of source data (for however large b.N ends up being) or having to reconstruct/re-add sources partway
// through, which would pollute the benchmark with setup costs unrelated to the Read hot path. It's a distinct
// pointer per Add call so it doesn't collide as a map key in MultiSourceReader.readers (unlike an empty struct
// value type, which would compare equal across instances).
type infiniteSource struct{}

func (s *infiniteSource) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i)
	}
	return len(p), nil
}

func (s *infiniteSource) Close() error { return nil }

// BenchmarkMultiSourceReader measures the steady-state cost of MultiSourceReader.Read itself: pool
// acquire/release, offset bookkeeping, and copying data into the caller's buffer. Reader/source construction
// happens once, before b.ResetTimer, so it doesn't count toward the reported ns/op or allocs/op (ResetTimer
// zeroes both the elapsed time and the memory allocation counters). The read buffer p is likewise allocated once
// and reused across all b.N calls, so it isn't attributed to Read's own allocation profile either.
func BenchmarkMultiSourceReader(b *testing.B) {
	r := ioutil.NewMultiSourceReader(nil, 64*1024)
	r.Add(&infiniteSource{})
	r.Add(&infiniteSource{})
	defer func() { _ = r.Close() }()

	p := make([]byte, 32*1024)

	b.SetBytes(int64(len(p)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		n, err := r.Read(p)
		if err != nil {
			b.Fatal(err)
		}
		if n == 0 {
			b.Fatal("Read returned 0 bytes with no error")
		}
	}
}
