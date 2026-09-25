package fmp4

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	bts, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return bts
}

func parseFixtureInit(t *testing.T, name string) *Init {
	t.Helper()
	init, err := ParseInit(bytes.NewReader(fixture(t, name)))
	require.NoError(t, err)
	return init
}

func TestParseInit_AVC(t *testing.T) {
	init := parseFixtureInit(t, "vinit-stream0.m4s")
	require.Len(t, init.Tracks, 1)
	v := init.Track(KindVideo)
	require.NotNil(t, v)
	require.Nil(t, init.Track(KindAudio))

	require.Equal(t, CodecH264, v.Codec)
	require.Len(t, v.SPS, 1)
	require.Len(t, v.PPS, 1)
	require.Equal(t, byte(NaluSPS), NaluType(v.SPS[0]))
	require.Equal(t, byte(NaluPPS), NaluType(v.PPS[0]))
	require.Equal(t, 4, v.NALULengthSize)
	require.NotZero(t, v.Timescale)
	require.Greater(t, v.Width, 0)
	require.Greater(t, v.Height, 0)
	require.Contains(t, v.Codecs, "avc1.")
	require.NotNil(t, v.Trex, "init carries the track's default sample values")

	p := v.VideoParams()
	require.NotNil(t, p)
	require.Equal(t, v.SPS, p.SPS)
	require.Nil(t, v.AudioParams())
	t.Logf("video: %s %dx%d timescale=%d", v.Codecs, v.Width, v.Height, v.Timescale)
}

func TestParseInit_AAC(t *testing.T) {
	init := parseFixtureInit(t, "ainit.m4s")
	a := init.Track(KindAudio)
	require.NotNil(t, a)
	require.Equal(t, CodecAAC, a.Codec)
	require.Equal(t, "mp4a.40.2", a.Codecs)
	require.Equal(t, 2, a.ObjectType)
	require.Equal(t, 48000, a.SampleRate)
	require.Equal(t, 2, a.Channels)
	require.NotEmpty(t, a.ASC)
	require.EqualValues(t, 48000, a.Timescale)

	p := a.AudioParams()
	require.NotNil(t, p)
	require.Equal(t, a.ASC, p.ASC)
	require.Nil(t, a.VideoParams())
}

func TestParseInit_RejectsHEVC(t *testing.T) {
	_, err := ParseInit(bytes.NewReader(fixture(t, "hevc-init.m4s")))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrUnsupportedCodec), err.Error())
	require.Contains(t, err.Error(), "hvc1")
}

func TestParseInit_Garbage(t *testing.T) {
	_, err := ParseInit(bytes.NewReader([]byte("not an mp4 file at all")))
	require.Error(t, err)
}

func TestTrackInfo_CodecEqual(t *testing.T) {
	v := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)
	v2 := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)
	a := parseFixtureInit(t, "ainit.m4s").Track(KindAudio)
	require.True(t, v.CodecEqual(v2))
	require.False(t, v.CodecEqual(a))

	changed := *v2
	changed.SPS = [][]byte{append([]byte(nil), v.SPS[0]...)}
	changed.SPS[0][len(changed.SPS[0])-1] ^= 0x01
	require.False(t, v.CodecEqual(&changed))

	require.True(t, CodecParams{Video: v.VideoParams(), Audio: a.AudioParams()}.Equal(
		CodecParams{Video: v2.VideoParams(), Audio: a.AudioParams()}))
	videoOnly := CodecParams{Video: v.VideoParams()}
	require.False(t, videoOnly.Equal(CodecParams{Video: v.VideoParams(), Audio: a.AudioParams()}))
}

func readAll(t *testing.T, r io.Reader, track *TrackInfo) []*Sample {
	t.Helper()
	var samples []*Sample
	require.NoError(t, ReadFragments(r, track, func(s *Sample) error {
		samples = append(samples, s)
		return nil
	}))
	return samples
}

func TestReadFragments_Video(t *testing.T) {
	v := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)
	seg := fixture(t, "vchunk-stream0-00001.m4s")
	samples := readAll(t, bytes.NewReader(seg), v)
	require.NotEmpty(t, samples)

	require.True(t, samples[0].Sync, "a segment starts with a keyframe")
	var payload int
	for i, s := range samples {
		require.Same(t, v, s.Track)
		require.NotEmpty(t, s.NALUs)
		require.NotZero(t, s.Duration, "sample durations resolved from tfhd/trex defaults")
		require.GreaterOrEqual(t, s.PTS, s.DTS)
		if i > 0 {
			require.Equal(t, samples[i-1].DTS+int64(samples[i-1].Duration), s.DTS, "DTS advances by the previous duration")
		}
		for _, n := range s.NALUs {
			typ := NaluType(n)
			require.NotEqual(t, byte(NaluAUD), typ, "access unit delimiters are stripped")
			require.NotEqual(t, byte(NaluFIL), typ, "filler is stripped")
		}
		require.False(t, s.ParamSetChange, "in-band parameter sets match the init segment")
		payload += len(s.Data)
	}
	require.Less(t, payload, len(seg))
	require.Greater(t, payload, len(seg)*9/10, "nearly all of the segment is sample data")
	t.Logf("%d video samples, first dts=%d, duration=%d", len(samples), samples[0].DTS, samples[0].Duration)
}

func TestReadFragments_Audio(t *testing.T) {
	a := parseFixtureInit(t, "ainit.m4s").Track(KindAudio)
	samples := readAll(t, bytes.NewReader(fixture(t, "achunk-00001.m4s")), a)
	require.NotEmpty(t, samples)
	for i, s := range samples {
		require.True(t, s.Sync)
		require.Nil(t, s.NALUs)
		require.EqualValues(t, 1024, s.Duration, "one AAC frame per sample")
		require.Equal(t, s.DTS, s.PTS)
		if i > 0 {
			require.Equal(t, samples[i-1].DTS+1024, s.DTS)
		}
	}
	// the second segment continues the timeline of the first
	next := readAll(t, bytes.NewReader(fixture(t, "achunk-00002.m4s")), a)
	require.NotEmpty(t, next)
	last := samples[len(samples)-1]
	require.Equal(t, last.DTS+int64(last.Duration), next[0].DTS)
}

// trickleReader hands out 1..7 bytes per Read, the way a segment still being produced arrives.
type trickleReader struct {
	data []byte
	n    int
}

func (r *trickleReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	chunk := r.n%7 + 1
	r.n++
	if chunk > len(r.data) {
		chunk = len(r.data)
	}
	if chunk > len(p) {
		chunk = len(p)
	}
	copy(p, r.data[:chunk])
	r.data = r.data[chunk:]
	return chunk, nil
}

func TestReadFragments_TrickledInput(t *testing.T) {
	v := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)
	seg := fixture(t, "vchunk-stream0-00001.m4s")
	want := readAll(t, bytes.NewReader(seg), v)
	got := readAll(t, &trickleReader{data: seg}, v)
	require.Equal(t, len(want), len(got))
	for i := range want {
		require.Equal(t, want[i].DTS, got[i].DTS)
		require.Equal(t, want[i].Data, got[i].Data)
	}
}

// gatedReader blocks every read until released, standing in for a page reader that waits for the transcoder.
type gatedReader struct {
	data  []byte
	gate  chan struct{}
	reads int
}

func (r *gatedReader) Read(p []byte) (int, error) {
	<-r.gate
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	if n > 1000 {
		n = 1000
	}
	r.data = r.data[n:]
	r.reads++
	return n, nil
}

func TestReadFragments_BlockingReader(t *testing.T) {
	v := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)
	r := &gatedReader{data: fixture(t, "vchunk-stream0-00001.m4s"), gate: make(chan struct{})}
	got := make(chan int, 1)
	go func() {
		n := 0
		_ = ReadFragments(r, v, func(*Sample) error { n++; return nil })
		got <- n
	}()
	select {
	case <-got:
		t.Fatal("must block while the reader has not delivered anything")
	default:
	}
	close(r.gate)
	require.Greater(t, <-got, 0)
}

func TestReadFragments_Errors(t *testing.T) {
	v := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)

	t.Run("truncated segment", func(t *testing.T) {
		seg := fixture(t, "vchunk-stream0-00001.m4s")
		err := ReadFragments(bytes.NewReader(seg[:len(seg)/2]), v, func(*Sample) error { return nil })
		require.Error(t, err)
	})

	t.Run("init segment instead of media segment", func(t *testing.T) {
		err := ReadFragments(bytes.NewReader(fixture(t, "vinit-stream0.m4s")), v, func(*Sample) error { return nil })
		require.ErrorContains(t, err, "unexpected init segment")
	})

	t.Run("callback error is returned as is", func(t *testing.T) {
		boom := errors.E("test", errors.K.Cancelled, "reason", "stop")
		err := ReadFragments(bytes.NewReader(fixture(t, "vchunk-stream0-00001.m4s")), v, func(*Sample) error { return boom })
		require.Same(t, boom, err)
	})
}

func BenchmarkReadFragments_Video(b *testing.B) {
	init, err := ParseInit(bytes.NewReader(mustRead(b, "vinit-stream0.m4s")))
	if err != nil {
		b.Fatal(err)
	}
	v := init.Track(KindVideo)
	seg := mustRead(b, "vchunk-stream0-00001.m4s")
	b.ReportAllocs()
	b.SetBytes(int64(len(seg)))
	for b.Loop() {
		if err := ReadFragments(bytes.NewReader(seg), v, func(*Sample) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

func mustRead(b *testing.B, name string) []byte {
	bts, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Fatal(err)
	}
	return bts
}
