package byteutil_test

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/eluv-io/common-go/util/byteutil"
)

var bufSize = 64 * 1024
var data []byte
var r *bytes.Reader

func init() {
	data = byteutil.RandomBytes(100 * 1024 * 1024) // 100MB
	r = bytes.NewReader(data)
}

type testCtx struct {
	created   counter
	retrieved counter
	released  counter
}

func TestPool(t *testing.T) {
	bufSize := 8
	refCount := byte(4)
	zeroBuf := make([]byte, bufSize)
	var createCount, openCount, closeCount int

	ctx := &testCtx{}

	p := byteutil.NewPool(bufSize)
	p.SetMetrics(&ctx.created, &ctx.retrieved, &ctx.released)

	buf := p.New()
	// New (zero-ed) buffer of size 8 with refCount 1 should be created
	require.Equal(t, bufSize+1, cap(*buf))
	require.Equal(t, bufSize, len(*buf))
	require.Equal(t, zeroBuf, *buf)
	require.Equal(t, byte(1), (*buf)[:bufSize+1][bufSize])
	createCount++
	openCount++

	buf = p.NewN(refCount)
	// New (zero-ed) buffer of size 8 with refCount 4 should be created
	require.Equal(t, bufSize+1, cap(*buf))
	require.Equal(t, bufSize, len(*buf))
	require.Equal(t, zeroBuf, *buf)
	require.Equal(t, refCount, (*buf)[:bufSize+1][bufSize])
	createCount++
	openCount++

	buf = p.Get()
	// New (zero-ed) buffer of size 8 with refCount 1 should be created
	require.Equal(t, bufSize+1, cap(*buf))
	require.Equal(t, bufSize, len(*buf))
	require.Equal(t, zeroBuf, *buf)
	require.Equal(t, byte(1), (*buf)[:bufSize+1][bufSize])
	createCount++
	openCount++

	buf = p.GetN(refCount)
	// New (zero-ed) buffer of size 8 with refCount 1 should be created
	require.Equal(t, bufSize+1, cap(*buf))
	require.Equal(t, bufSize, len(*buf))
	require.Equal(t, zeroBuf, *buf)
	require.Equal(t, refCount, (*buf)[:bufSize+1][bufSize])
	createCount++
	openCount++

	// Populate existing buffer with 1s, for identification purposes
	for i := range *buf {
		(*buf)[i] = 1
	}

	mu := &sync.Mutex{}
	p.WithLocker(mu)
	getRefCount := func(buf []byte) byte {
		mu.Lock()
		defer mu.Unlock()
		buf = buf[:p.BufSize+1]
		return buf[p.BufSize]
	}

	// Repeatedly release existing buffer to reduce refCount to 1 from 4
	for n := byte(0); n < refCount-1; n++ {
		r0 := ctx.released.val.Load()
		p.Put(buf)
		// RefCount of existing buffer should reduce by 1; buffer should not be re-added to pool yet
		for i := 0; i < 100; i++ {
			buf2 := p.Get()
			openCount++
			require.Equal(t, make([]byte, bufSize), *buf2)
			createCount++
			time.Sleep(time.Millisecond)
		}
		r1 := ctx.released.val.Load()
		require.Equal(t, 0.0, r1-r0)
		require.Equal(t, refCount-n-1, getRefCount(*buf))
	}

	require.Equal(t, byte(1), getRefCount(*buf))

	r0 := ctx.released.val.Load()
	p.Put(buf)
	time.Sleep(50 * time.Millisecond)
	r1 := ctx.released.val.Load()
	require.Equal(t, 1.0, r1-r0)
	require.Equal(t, byte(0), getRefCount(*buf))
	closeCount++

	// Existing buffer should be re-added to pool
	// Attempt to retrieve existing buffer from pool; buffer may not necessarily be the first buffer from Get()
	// Most of the time the buffer is retrieved in a few iterations, but there's
	// no guarantee and can fail even with as many as 5000 trials. From doc:
	//    Get may choose to ignore the pool and treat it as empty.
	//    Callers should not assume any relation between values passed to Put and
	//    the values returned by Get.
	attempts := 5000
	var buf2 *[]byte
	for i := 0; i < attempts; i++ {
		buf2 = p.GetN(0)
		openCount++
		if (*buf2)[0] == 1 {
			// Existing buffer was re-added to pool and successfully retrieved with new refCount 0
			fmt.Println(fmt.Sprintf("found buffer at %d", i))
			break
		}
		createCount++

		time.Sleep(time.Millisecond)
	}

	// make sure we retrieved our buffer
	if (*buf2)[0] == 1 {
		require.Equal(t, *buf, *buf2, "buffer not found")
		require.Equal(t, byte(0), (*buf)[:bufSize+1][bufSize])
	}

	p.Put(buf2)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, byte(0), getRefCount(*buf2))

	// Existing buffer with refCount 0 should not be re-added to pool
	// Attempt to retrieve existing buffer from pool; buffer should not be found
	for i := 0; i < 100; i++ {
		buf3 := p.Get()
		openCount++
		require.Equal(t, make([]byte, bufSize), *buf3)
		createCount++
		time.Sleep(time.Millisecond)
	}

	require.Equal(t, float64(createCount), ctx.created.val.Load())
	require.Equal(t, float64(openCount), ctx.retrieved.val.Load())
	require.Equal(t, float64(closeCount), ctx.released.val.Load())
}

func TestPut_NilSlice(t *testing.T) {
	p := byteutil.NewPool(64)

	// Putting nil pointer should be safe
	p.Put(nil)

	// Putting pointer to nil slice should not panic
	var nilSlice []byte
	require.NotPanics(t, func() {
		p.Put(&nilSlice)
	})
}

func TestPut_ReslicedBuffer(t *testing.T) {
	bufSize := 64
	p := byteutil.NewPool(bufSize)

	buf := p.Get()
	require.Equal(t, bufSize, len(*buf))

	// Reslice buffer to a smaller length, retaining capacity
	*buf = (*buf)[:10]
	require.Equal(t, 10, len(*buf))
	require.Equal(t, bufSize+1, cap(*buf))

	// Put resliced buffer back into pool
	p.Put(buf)

	// In-place reslicing should restore len(*buf) to bufSize on the caller's slice header
	require.Equal(t, bufSize, len(*buf))
}

func TestPool_ConcurrentPut_DefaultLocker(t *testing.T) {
	bufSize := 64
	p := byteutil.NewPool(bufSize)
	ctx := &testCtx{}
	p.SetMetrics(&ctx.created, &ctx.retrieved, &ctx.released)

	refCount := byte(100)
	buf := p.GetN(refCount)

	var wg sync.WaitGroup
	for i := 0; i < int(refCount); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Put(buf)
		}()
	}
	wg.Wait()

	// Since refCount was 100 and Put was called 100 times, the buffer should be released exactly once
	require.Equal(t, 1.0, ctx.released.val.Load())
	require.Equal(t, byte(0), (*buf)[:bufSize+1][bufSize])
}

// % go test -tags -count=1 -v -bench="Benchmark.*Pool" -run="Benchmark" ./util/byteutil
// goos: darwin
// goarch: arm64
// pkg: github.com/eluv-io/common-go/util/byteutil
// cpu: Apple M4 Max
// BenchmarkPool
// BenchmarkPool-16        	 3599828	       334.8 ns/op	       0 B/op	       0 allocs/op
// BenchmarkPoolN
// BenchmarkPoolN-16       	 3594460	       332.6 ns/op	       0 B/op	       0 allocs/op
// BenchmarkSyncPool
// BenchmarkSyncPool-16    	 3567121	       336.1 ns/op	       0 B/op	       0 allocs/op
// BenchmarkNoPool
// BenchmarkNoPool-16      	  711243	      1626 ns/op	   65536 B/op	       1 allocs/op
// PASS
// ok  	github.com/eluv-io/common-go/util/byteutil	6.723s

func BenchmarkPool(b *testing.B) {
	p := byteutil.NewPool(bufSize)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := p.Get()
			err := task(b, *buf)
			if err != nil {
				return
			}
			p.Put(buf)
		}
	})
}

func BenchmarkPoolN(b *testing.B) {
	p := byteutil.NewPool(bufSize)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := p.GetN(1)
			err := task(b, *buf)
			if err != nil {
				return
			}
			p.Put(buf)
		}
	})
}

func BenchmarkSyncPool(b *testing.B) {
	p := &sync.Pool{New: func() interface{} {
		b := make([]byte, bufSize)
		return &b
	}}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := p.Get().(*[]byte)
			err := task(b, *buf)
			if err != nil {
				return
			}
			p.Put(buf)
		}
	})
}

func BenchmarkNoPool(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := make([]byte, bufSize)
			err := task(b, buf)
			if err != nil {
				return
			}
		}
	})
}

func task(b *testing.B, buf []byte) error {
	_, err := r.Read(buf)
	if err == io.EOF || (err == nil && r.Len() < len(buf)*64) {
		_, err = r.Seek(0, io.SeekStart)
	}
	if err != nil {
		b.Error("Unexpected error reading data", err)
		return err
	}
	return nil
}

type counter struct {
	val atomic.Float64
}

func (c *counter) Add(delta float64) {
	c.val.Add(delta)
}
