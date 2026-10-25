package fmp4

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Benchmarks of the segment read path. They use inline error checks rather than require.NoError: require runs on
// every iteration inside the timed loop and its reflection would dominate a measurement of a few microseconds.
//
// Baseline before the pooled parser (mp4ff, one mdat buffer per fragment):
//
//	goos: linux
//	goarch: amd64
//	pkg: github.com/eluv-io/common-go/media/fmp4
//	cpu: VirtualApple @ 2.50GHz
//	BenchmarkReadFragments_Video-4   2000   332226 ns/op   1326.27 MB/s   703985 B/op   5135 allocs/op
//
// Baseline with the pooled streaming parser (the video segment holds 119 fragments of one sample each; the 13 B/op
// is the pool's first fill amortized over the run, the allocation count is what matters):
//
//	BenchmarkReadFragments_Video-4         22272    53859 ns/op   8181.01 MB/s   13 B/op   0 allocs/op
//	BenchmarkReadFragments_Audio-4         67519    17654 ns/op   1107.98 MB/s    0 B/op   0 allocs/op
//	BenchmarkFragmentReader_ParseMoof-4  6132811      193.4 ns/op                 0 B/op   0 allocs/op
//	BenchmarkAppendNALUs-4              39642130       30.37 ns/op                0 B/op   0 allocs/op
func BenchmarkReadFragments_Video(b *testing.B) {
	benchmarkReadFragments(b, "vinit-stream0.m4s", "vchunk-stream0-00001.m4s", KindVideo)
}

func BenchmarkReadFragments_Audio(b *testing.B) {
	benchmarkReadFragments(b, "ainit.m4s", "achunk-00001.m4s", KindAudio)
}

func benchmarkReadFragments(b *testing.B, initName, segName string, kind Kind) {
	init, err := ParseInit(bytes.NewReader(mustRead(b, initName)))
	if err != nil {
		b.Fatal(err)
	}
	track := init.Track(kind)
	seg := mustRead(b, segName)
	fr := NewFragmentReader(NewFragmentPool(FragmentPoolConfig{}))
	rd := bytes.NewReader(nil)
	release := func(s *Sample) error { s.Release(); return nil }

	b.ReportAllocs()
	b.SetBytes(int64(len(seg)))
	for b.Loop() {
		rd.Reset(seg)
		if err := fr.ReadFragments(rd, track, release); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFragmentReader_ParseMoof isolates the box parsing from the payload read: one moof of the video fixture,
// already in memory.
func BenchmarkFragmentReader_ParseMoof(b *testing.B) {
	seg := mustRead(b, "vchunk-stream0-00001.m4s")
	moof := firstMoofBody(b, seg)
	fr := NewFragmentReader(nil)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := fr.parseMoof(moof, 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAppendNALUs isolates the AVCC split and the access unit inspection of one access unit.
func BenchmarkAppendNALUs(b *testing.B) {
	sps := []byte{0x67, 0x64, 0x00, 0x1f}
	pps := []byte{0x68, 0xeb}
	sample := avcc([]byte{0x09, 0xf0}, sps, pps, append([]byte{0x65}, bytes.Repeat([]byte{0x42}, 4096)...))
	arena := make([][]byte, 0, 16)
	b.ReportAllocs()
	for b.Loop() {
		var err error
		arena, err = appendNALUs(arena[:0], sample, 4)
		if err != nil {
			b.Fatal(err)
		}
		arena, _, _ = inspectAccessUnit(arena, [][]byte{sps}, [][]byte{pps})
	}
}

// firstMoofBody returns the body of the first moof box of a segment.
func firstMoofBody(b *testing.B, seg []byte) []byte {
	pos := 0
	for pos+8 <= len(seg) {
		size := int(seg[pos])<<24 | int(seg[pos+1])<<16 | int(seg[pos+2])<<8 | int(seg[pos+3])
		if string(seg[pos+4:pos+8]) == "moof" {
			return seg[pos+8 : pos+size]
		}
		if size < 8 {
			break
		}
		pos += size
	}
	b.Fatal("no moof box in segment")
	return nil
}

func mustRead(b *testing.B, name string) []byte {
	bts, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Fatal(err)
	}
	return bts
}
