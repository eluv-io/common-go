package byteutil_test

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/metrics"
	"github.com/qluvio/content-fabric/test"
	"github.com/qluvio/content-fabric/util/byteutil"
)

var bufSize = 64 * 1024
var data []byte
var r io.Reader

func init() {
	data = byteutil.RandomBytes(100 * 1024 * 1024) // 100MB
	r = bytes.NewBuffer(data)
}

type testCtx struct {
	NetBuffers metrics.QPartsNetBuffers
}

func TestPool(t *testing.T) {
	bufSize := 8
	refCount := byte(4)
	zeroBuf := make([]byte, bufSize)
	var openCount, closeCount float64

	c, cleanup := test.CreateDebugAppConfig()
	defer cleanup()
	m := test.ModuleFromAppConfig(c)
	inj := test.TestInjector(m)
	ctx := &testCtx{}
	err := inj.Populate(ctx)
	require.NoError(t, err)

	p := byteutil.NewPool(bufSize)
	p.SetMetrics(ctx.NetBuffers)

	buf := p.New()
	// New (zero-ed) buffer of size 8 with refCount 1 should be created
	require.Equal(t, bufSize+1, cap(buf))
	require.Equal(t, bufSize, len(buf))
	require.Equal(t, zeroBuf, buf)
	require.Equal(t, byte(1), buf[:bufSize+1][bufSize])
	openCount++

	buf = p.New(refCount)
	// New (zero-ed) buffer of size 8 with refCount 4 should be created
	require.Equal(t, bufSize+1, cap(buf))
	require.Equal(t, bufSize, len(buf))
	require.Equal(t, zeroBuf, buf)
	require.Equal(t, refCount, buf[:bufSize+1][bufSize])
	openCount++

	buf = p.Get()
	// New (zero-ed) buffer of size 8 with refCount 1 should be created
	require.Equal(t, bufSize+1, cap(buf))
	require.Equal(t, bufSize, len(buf))
	require.Equal(t, zeroBuf, buf)
	require.Equal(t, byte(1), buf[:bufSize+1][bufSize])
	openCount++

	buf = p.Get(refCount)
	// New (zero-ed) buffer of size 8 with refCount 1 should be created
	require.Equal(t, bufSize+1, cap(buf))
	require.Equal(t, bufSize, len(buf))
	require.Equal(t, zeroBuf, buf)
	require.Equal(t, refCount, buf[:bufSize+1][bufSize])
	openCount++

	// Populate existing buffer with 1s, for identification purposes
	for i := range buf {
		buf[i] = 1
	}

	// Repeatedly release existing buffer to reduce refCount to 1 from 4
	for n := byte(0); n < refCount-1; n++ {
		p.Put(buf)
		// RefCount of existing buffer should reduce by 1; buffer should not be re-added to pool yet
		for i := 0; i < 100; i++ {
			buf2 := p.Get()
			require.Equal(t, make([]byte, bufSize), buf2)
			openCount++
			time.Sleep(time.Millisecond)
		}
		require.Equal(t, refCount-n-1, buf[:bufSize+1][bufSize])
	}

	p.Put(buf)
	// Existing buffer should be re-added to pool
	// Attempt to retrieve existing buffer from pool; buffer may not necessarily be the first buffer from Get()
	var buf2 []byte
	for i := 0; i < 100; i++ {
		buf2 = p.Get(0)
		if buf2[0] == 1 {
			// Existing buffer was re-added to pool and successfully retrieved with new refCount 0
			break
		}
		openCount++
		time.Sleep(time.Millisecond)
	}
	require.Equal(t, buf2, buf)
	require.Equal(t, byte(0), buf[:bufSize+1][bufSize])
	closeCount++

	p.Put(buf2)
	// Existing buffer with refCount 0 should not be re-added to pool
	// Attempt to retrieve existing buffer from pool; buffer should not be found
	for i := 0; i < 100; i++ {
		buf3 := p.Get()
		require.Equal(t, make([]byte, bufSize), buf3)
		openCount++
		time.Sleep(time.Millisecond)
	}

	require.Equal(t, openCount, ctx.NetBuffers.Open().Get())
	require.Equal(t, closeCount, ctx.NetBuffers.Close().Get())
}

// go test -tags "avpipe LLVM byollvm" -count=1 -v -bench=Benchmark* -run=Benchmark ./util/byteutil
// goos: darwin
// goarch: amd64
// pkg: github.com/qluvio/content-fabric/util/byteutil
// BenchmarkPool-8       	  421527	      2413 ns/op	      33 B/op	       1 allocs/op
// BenchmarkSyncPool-8   	 1000000	      1971 ns/op	      32 B/op	       1 allocs/op
// BenchmarkNoPool-8     	  156228	      7671 ns/op	   65536 B/op	       1 allocs/op
// PASS
// ok  	github.com/qluvio/content-fabric/util/byteutil	5.404s

func BenchmarkPool(b *testing.B) {
	p := byteutil.NewPool(bufSize)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := p.Get()
			err := task(b, buf)
			if err != nil {
				return
			}
			p.Put(buf)
		}
	})
}

func BenchmarkSyncPool(b *testing.B) {
	p := &sync.Pool{New: func() interface{} {
		return make([]byte, bufSize)
	}}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := p.Get().([]byte)
			err := task(b, buf)
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
	if err == io.EOF {
		r = bytes.NewBuffer(data)
	} else if err != nil {
		b.Error("Unexpected error reading data", err)
		return err
	}
	return nil
}
