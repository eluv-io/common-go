package rtmp

import (
	"testing"
	"time"

	"github.com/eluv-io/common-go/media/fmp4"
)

// Benchmarks of the media write path over a connection that discards what it is given, so the numbers cover the
// framing rather than the network. Inline error checks rather than require: require runs inside the timed loop.
//
// Before the chunk writer, every access unit was copied three times on the way out (the AVCC re-serialisation, the
// FLV message body and the chunk buffer) and cost about ten allocations; audio two copies and nine allocations.
//
//	goos: linux
//	goarch: amd64
//	pkg: github.com/eluv-io/common-go/media/rtmp
//	cpu: VirtualApple @ 2.50GHz
//	BenchmarkConn_WriteVideo/p-frame-20KB-4     4013534    299.7 ns/op   68331.09 MB/s   0 B/op   0 allocs/op
//	BenchmarkConn_WriteVideo/keyframe-200KB-4    297894   3847 ns/op     53235.27 MB/s   0 B/op   0 allocs/op
//	BenchmarkConn_WriteAudio-4                 22728522     51.88 ns/op   7710.14 MB/s   0 B/op   0 allocs/op
//	BenchmarkChunkWriter_WriteVideo-4           4157175    289.2 ns/op                    0 B/op   0 allocs/op
func BenchmarkConn_WriteVideo(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"p-frame-20KB", 20 << 10},
		{"keyframe-200KB", 200 << 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			c := benchConn(b)
			nalus := [][]byte{nalOf(5, 40), nalOf(1, tc.size)}
			var dts time.Duration
			b.ReportAllocs()
			b.SetBytes(int64(tc.size))
			for b.Loop() {
				if err := c.WriteVideo(nalus, dts, dts, true); err != nil {
					b.Fatal(err)
				}
				dts += 40 * time.Millisecond
			}
		})
	}
}

func BenchmarkConn_WriteAudio(b *testing.B) {
	c := benchConn(b)
	// prime the connection: audio before the first video keyframe is dropped
	if err := c.WriteVideo([][]byte{nalOf(5, 64)}, 0, 0, true); err != nil {
		b.Fatal(err)
	}
	au := nalOf(1, 400)
	var pts time.Duration
	b.ReportAllocs()
	b.SetBytes(int64(len(au)))
	for b.Loop() {
		if err := c.WriteAudio(au, pts); err != nil {
			b.Fatal(err)
		}
		pts += 21 * time.Millisecond
	}
}

// BenchmarkChunkWriter_WriteVideo isolates the framing from the connection's timeline and statistics.
func BenchmarkChunkWriter_WriteVideo(b *testing.B) {
	w := newChunkWriter(discardConn{})
	nalus := [][]byte{nalOf(5, 40), nalOf(1, 20<<10)}
	ts := uint32(0)
	b.ReportAllocs()
	for b.Loop() {
		if err := w.writeVideo(nalus, ts, 0, false); err != nil {
			b.Fatal(err)
		}
		ts += 40
	}
}

// benchConn builds a connection over a discarding socket, with neither the announcement nor the read loop.
func benchConn(b *testing.B) *Conn {
	b.Helper()
	params := fmp4.CodecParams{
		Video: &fmp4.VideoParams{Codecs: "avc1.640028"},
		Audio: &fmp4.AudioParams{Codecs: "mp4a.40.2"},
	}
	return newConn(Config{}, params, newSession(discardConn{}, 0))
}
