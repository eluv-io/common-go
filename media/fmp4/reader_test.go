package fmp4

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/internal/raceflag"
	"github.com/eluv-io/errors-go"
)

// ---------------------------------------------------------------------------------------------------------------------
// segment builder
//
// The fixtures on disk all carry one sample per fragment, which is what avpipe writes. The cases below need fragments
// the encoders in use do not produce (multi-sample truns, several truns, base_data_offset, a second track), so they
// are built here from raw boxes rather than with mp4ff, which would make the oracle and the input share an encoder.

// tSample is one sample of a synthesized trun.
type tSample struct {
	data  []byte
	dur   uint32
	flags uint32
	cto   int32
}

// tTrun is one synthesized track fragment run.
type tTrun struct {
	version          byte
	samples          []tSample
	noDataOffset     bool   // omit the data offset; the run continues where the previous one ended
	firstSampleFlags uint32 // written when non-zero
	perSampleFlags   bool   // write a flags field per sample instead
	noDuration       bool   // omit the per-sample durations, leaving them to the defaults
	noSize           bool   // omit the per-sample sizes, leaving them to the defaults
	noCto            bool   // omit the composition time offsets
}

// tTraf is one synthesized track fragment.
type tTraf struct {
	trackID        uint32
	baseDecodeTime uint64
	tfdtV0         bool
	truns          []tTrun

	defaultBaseIsMoof bool
	baseDataOffset    uint64 // written when non-zero, which also clears default-base-is-moof
	defaultDuration   uint32 // written when non-zero
	defaultSize       uint32 // written when non-zero
	defaultFlags      uint32 // written when non-zero
	omitTfhd          bool
}

// tFrag is one synthesized fragment: a moof and its mdat.
type tFrag struct {
	trafs      []tTraf
	mdatPad    int  // bytes appended to the mdat after the sample data
	mdatLarge  bool // use a 16 byte mdat header with a large size
	extraBoxes []byte
}

func be32(b []byte, v uint32) []byte {
	var t [4]byte
	binary.BigEndian.PutUint32(t[:], v)
	return append(b, t[:]...)
}

func be64(b []byte, v uint64) []byte {
	var t [8]byte
	binary.BigEndian.PutUint64(t[:], v)
	return append(b, t[:]...)
}

// box wraps parts in a box of the given type.
func box(typ string, parts ...[]byte) []byte {
	n := 8
	for _, p := range parts {
		n += len(p)
	}
	out := be32(make([]byte, 0, n), uint32(n))
	out = append(out, typ...)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func (t tTraf) tfhd() []byte {
	if t.omitTfhd {
		return nil
	}
	var flags uint32
	var tail []byte
	if t.baseDataOffset != 0 {
		flags |= mp4.TfhdBaseDataOffsetPresentFlag
		tail = be64(tail, t.baseDataOffset)
	} else if t.defaultBaseIsMoof {
		flags |= mp4.TfhdDefaultBaseIsMoofFlag
	}
	if t.defaultDuration != 0 {
		flags |= mp4.TfhdDefaultSampleDurationPresentFlag
		tail = be32(tail, t.defaultDuration)
	}
	if t.defaultSize != 0 {
		flags |= mp4.TfhdDefaultSampleSizePresentFlag
		tail = be32(tail, t.defaultSize)
	}
	if t.defaultFlags != 0 {
		flags |= mp4.TfhdDefaultSampleFlagsPresentFlag
		tail = be32(tail, t.defaultFlags)
	}
	body := be32(nil, flags) // version 0
	body = be32(body, t.trackID)
	return box("tfhd", body, tail)
}

func (t tTraf) tfdt() []byte {
	if t.tfdtV0 {
		return box("tfdt", be32(be32(nil, 0), uint32(t.baseDecodeTime)))
	}
	return box("tfdt", be64(be32(nil, 1<<24), t.baseDecodeTime))
}

// trunBox builds one trun. dataOffset is the offset of its first sample from the base data offset.
func (r tTrun) trunBox(dataOffset int32) []byte {
	var flags uint32
	if !r.noDataOffset {
		flags |= mp4.TrunDataOffsetPresentFlag
	}
	if r.firstSampleFlags != 0 {
		flags |= mp4.TrunFirstSampleFlagsPresentFlag
	}
	if !r.noDuration {
		flags |= mp4.TrunSampleDurationPresentFlag
	}
	if !r.noSize {
		flags |= mp4.TrunSampleSizePresentFlag
	}
	if r.perSampleFlags {
		flags |= mp4.TrunSampleFlagsPresentFlag
	}
	if !r.noCto {
		flags |= mp4.TrunSampleCompositionTimeOffsetPresentFlag
	}

	body := be32(nil, uint32(r.version)<<24|flags)
	body = be32(body, uint32(len(r.samples)))
	if !r.noDataOffset {
		body = be32(body, uint32(dataOffset))
	}
	if r.firstSampleFlags != 0 {
		body = be32(body, r.firstSampleFlags)
	}
	for _, s := range r.samples {
		if !r.noDuration {
			body = be32(body, s.dur)
		}
		if !r.noSize {
			body = be32(body, uint32(len(s.data)))
		}
		if r.perSampleFlags {
			body = be32(body, s.flags)
		}
		if !r.noCto {
			body = be32(body, uint32(s.cto))
		}
	}
	return box("trun", body)
}

// build assembles the fragment. Data offsets are patched once the moof size is known.
func (f tFrag) build() []byte {
	moof := f.moof(0)
	mdatHdr := 8
	if f.mdatLarge {
		mdatHdr = 16
	}
	// with default-base-is-moof the base is the moof start, so the first sample sits at moof size + mdat header
	moof = f.moof(int32(len(moof) + mdatHdr))

	var payload []byte
	for _, traf := range f.trafs {
		for _, run := range traf.truns {
			for _, s := range run.samples {
				payload = append(payload, s.data...)
			}
		}
	}
	payload = append(payload, make([]byte, f.mdatPad)...)

	out := append([]byte{}, moof...)
	if f.mdatLarge {
		out = be32(out, 1)
		out = append(out, "mdat"...)
		out = be64(out, uint64(16+len(payload)))
	} else {
		out = be32(out, uint32(8+len(payload)))
		out = append(out, "mdat"...)
	}
	return append(out, payload...)
}

func (f tFrag) moof(firstDataOffset int32) []byte {
	parts := [][]byte{box("mfhd", be32(be32(nil, 0), 1))}
	off := firstDataOffset
	for _, traf := range f.trafs {
		children := [][]byte{traf.tfhd(), traf.tfdt()}
		for _, run := range traf.truns {
			children = append(children, run.trunBox(off))
			for _, s := range run.samples {
				off += int32(len(s.data))
			}
		}
		parts = append(parts, box("traf", children...))
	}
	return box("moof", parts...)
}

// segment assembles a whole segment: styp, sidx and the fragments.
func segment(frags ...tFrag) []byte {
	out := box("styp", []byte("cmfs"), be32(nil, 0), []byte("cmfs"))
	for _, f := range frags {
		out = append(out, f.extraBoxes...)
		out = append(out, f.build()...)
	}
	return out
}

// videoTrack is the track the synthesized fragments belong to, with a length size of 4 and no parameter sets, so any
// AVCC payload parses.
func videoTrack() *TrackInfo {
	return &TrackInfo{TrackID: 1, Kind: KindVideo, Codec: CodecH264, Timescale: 90000, NALULengthSize: 4}
}

// opaqueTrack is a track of neither kind, so samples are taken verbatim without an AVCC split.
func opaqueTrack(id uint32) *TrackInfo {
	return &TrackInfo{TrackID: id, Timescale: 90000}
}

// nalu builds one AVCC NAL unit of the given type and size.
func nalu(typ byte, n int) []byte {
	body := append([]byte{typ}, bytes.Repeat([]byte{byte(n)}, n-1)...)
	return append(be32(nil, uint32(len(body))), body...)
}

// readSegment parses seg and returns copies of the samples.
func readSegment(t *testing.T, seg []byte, track *TrackInfo) ([]*Sample, *FragmentPool, error) {
	t.Helper()
	pool := NewFragmentPool(FragmentPoolConfig{})
	var got []*Sample
	err := ReadFragments(bytes.NewReader(seg), track, pool, func(s *Sample) error {
		got = append(got, s.Clone())
		s.Release()
		return nil
	})
	return got, pool, err
}

// ---------------------------------------------------------------------------------------------------------------------

func TestFragmentReader_MultiSampleTrun(t *testing.T) {
	samples := make([]tSample, 30)
	for i := range samples {
		samples[i] = tSample{data: nalu(1, 10+i), dur: 3000, cto: int32(i % 3 * 1000)}
	}
	samples[0].data = nalu(NaluIDR, 40)
	seg := segment(tFrag{trafs: []tTraf{{
		trackID: 1, baseDecodeTime: 9000, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: samples, firstSampleFlags: mp4.SyncSampleFlags}},
	}}})

	got, pool, err := readSegment(t, seg, videoTrack())
	require.NoError(t, err)
	require.Len(t, got, 30)
	requirePoolDrained(t, pool)

	dts := int64(9000)
	for i, s := range got {
		require.Equal(t, dts, s.DTS, "sample %d", i)
		require.Equal(t, dts+int64(i%3*1000), s.PTS)
		require.EqualValues(t, 3000, s.Duration)
		require.Equal(t, samples[i].data[4:], s.Data[4:], "payload of sample %d", i)
		dts += 3000
	}
	require.True(t, got[0].Sync, "the first sample carries the sync flags")
}

func TestFragmentReader_SeveralTruns(t *testing.T) {
	a := []tSample{{data: nalu(NaluIDR, 20), dur: 3000}, {data: nalu(1, 10), dur: 3000}}
	b := []tSample{{data: nalu(1, 15), dur: 3000}}
	seg := segment(tFrag{trafs: []tTraf{{
		trackID: 1, baseDecodeTime: 0, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: a}, {version: 1, samples: b, noDataOffset: true}},
	}}})

	got, pool, err := readSegment(t, seg, videoTrack())
	require.NoError(t, err)
	require.Len(t, got, 3)
	requirePoolDrained(t, pool)
	require.Equal(t, []int64{0, 3000, 6000}, []int64{got[0].DTS, got[1].DTS, got[2].DTS},
		"a trun without a data offset continues after the previous one")
	require.Equal(t, b[0].data[4:], got[2].Data[4:])
}

func TestFragmentReader_TrunVariants(t *testing.T) {
	t.Run("negative composition offset", func(t *testing.T) {
		seg := segment(tFrag{trafs: []tTraf{{
			trackID: 1, baseDecodeTime: 90000, defaultBaseIsMoof: true,
			truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(NaluIDR, 20), dur: 3000, cto: -1500}}}},
		}}})
		got, _, err := readSegment(t, seg, videoTrack())
		require.NoError(t, err)
		require.Equal(t, int64(90000), got[0].DTS)
		require.Equal(t, int64(88500), got[0].PTS, "a version 1 trun carries signed offsets")
	})

	t.Run("tfdt version 0", func(t *testing.T) {
		seg := segment(tFrag{trafs: []tTraf{{
			trackID: 1, baseDecodeTime: 12345, tfdtV0: true, defaultBaseIsMoof: true,
			truns: []tTrun{{samples: []tSample{{data: nalu(NaluIDR, 20), dur: 3000}}, noCto: true}},
		}}})
		got, _, err := readSegment(t, seg, videoTrack())
		require.NoError(t, err)
		require.Equal(t, int64(12345), got[0].DTS)
		require.Equal(t, got[0].DTS, got[0].PTS)
	})

	t.Run("defaults from tfhd", func(t *testing.T) {
		data := nalu(NaluIDR, 30)
		seg := segment(tFrag{trafs: []tTraf{{
			trackID: 1, defaultBaseIsMoof: true, defaultDuration: 1500, defaultSize: uint32(len(data)),
			defaultFlags: mp4.SyncSampleFlags,
			truns: []tTrun{{samples: []tSample{{data: data}, {data: data}},
				noDuration: true, noSize: true, noCto: true}},
		}}})
		got, _, err := readSegment(t, seg, videoTrack())
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.EqualValues(t, 1500, got[0].Duration)
		require.Equal(t, int64(1500), got[1].DTS)
		require.True(t, got[0].Sync, "the tfhd default flags mark both samples as sync")
		require.True(t, got[1].Sync)
	})

	t.Run("base data offset", func(t *testing.T) {
		// the base is absolute: the payload starts after the styp, the moof and the mdat header
		styp := len(box("styp", []byte("cmfs"), be32(nil, 0), []byte("cmfs")))
		f := tFrag{trafs: []tTraf{{
			trackID: 1, baseDataOffset: 1, // non-zero so the tfhd is built with the field, for the size measurement
			truns: []tTrun{{version: 1, noDataOffset: true,
				samples: []tSample{{data: nalu(NaluIDR, 20), dur: 3000}}}},
		}}}
		f.trafs[0].baseDataOffset = uint64(styp + len(f.moof(0)) + 8)

		got, pool, err := readSegment(t, segment(f), videoTrack())
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, 24, len(got[0].Data))
		requirePoolDrained(t, pool)
	})
}

func TestFragmentReader_TrackSelection(t *testing.T) {
	other := tTraf{trackID: 7, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: bytes.Repeat([]byte{0xaa}, 32), dur: 1024}}}}}
	wanted := tTraf{trackID: 2, baseDecodeTime: 2048, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: bytes.Repeat([]byte{0xbb}, 16), dur: 1024}}}}}

	got, pool, err := readSegment(t, segment(tFrag{trafs: []tTraf{other, wanted}}), opaqueTrack(2))
	require.NoError(t, err)
	require.Len(t, got, 1, "only the samples of the requested track are delivered")
	require.Equal(t, bytes.Repeat([]byte{0xbb}, 16), got[0].Data)
	require.Equal(t, int64(2048), got[0].DTS)
	requirePoolDrained(t, pool)

	t.Run("fragment without the track is skipped", func(t *testing.T) {
		got, pool, err := readSegment(t, segment(tFrag{trafs: []tTraf{other}}, tFrag{trafs: []tTraf{wanted}}),
			opaqueTrack(2))
		require.NoError(t, err)
		require.Len(t, got, 1)
		requirePoolDrained(t, pool)
	})
}

func TestFragmentReader_MdatVariants(t *testing.T) {
	frag := func(pad int, large bool) []byte {
		return segment(tFrag{
			mdatPad: pad, mdatLarge: large,
			trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
				truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(NaluIDR, 24), dur: 3000}}}}}},
		})
	}

	t.Run("mdat larger than the sample data", func(t *testing.T) {
		got, pool, err := readSegment(t, frag(512, false), videoTrack())
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Len(t, got[0].Data, 28, "the padding is skipped, not delivered")
		requirePoolDrained(t, pool)
	})

	t.Run("large size header", func(t *testing.T) {
		got, _, err := readSegment(t, frag(0, true), videoTrack())
		require.NoError(t, err)
		require.Len(t, got, 1)
	})
}

func TestFragmentReader_SkipsUnknownBoxes(t *testing.T) {
	extra := append(box("prft", bytes.Repeat([]byte{0}, 20)), box("emsg", bytes.Repeat([]byte{0}, 40))...)
	seg := segment(tFrag{
		extraBoxes: extra,
		trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
			truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(NaluIDR, 20), dur: 3000}}}}}},
	})
	seg = append(seg, box("free", bytes.Repeat([]byte{0}, 100))...)

	got, pool, err := readSegment(t, seg, videoTrack())
	require.NoError(t, err)
	require.Len(t, got, 1)
	requirePoolDrained(t, pool)
}

func TestFragmentReader_Rejects(t *testing.T) {
	good := tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(NaluIDR, 20), dur: 3000}}}}}}}

	for _, tc := range []struct {
		name    string
		seg     []byte
		wantErr string
	}{
		{"empty segment", box("styp", []byte("cmfs")), "no fragments"},
		{"mdat without moof", box("mdat", bytes.Repeat([]byte{0}, 16)), "mdat without a preceding moof"},
		{"init segment", append(box("ftyp", []byte("isom")), box("moov")...), "unexpected init segment"},
		{"box smaller than its header", append(be32(nil, 4), []byte("free")...), "box smaller than its header"},
		{"box to the end of the file", append(be32(nil, 0), []byte("free")...), "box extending to the end"},
		{"traf without tfhd", segment(tFrag{trafs: []tTraf{{trackID: 1, omitTfhd: true,
			truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(1, 8), dur: 1}}}}}}}), "traf without tfhd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, pool, err := readSegment(t, tc.seg, videoTrack())
			require.ErrorContains(t, err, tc.wantErr)
			requirePoolDrained(t, pool)
		})
	}

	t.Run("sample data beyond the mdat", func(t *testing.T) {
		seg := segment(good)
		// shrink the mdat header so the sample no longer fits
		idx := bytes.Index(seg, []byte("mdat"))
		require.Positive(t, idx)
		binary.BigEndian.PutUint32(seg[idx-4:], 12)
		_, pool, err := readSegment(t, seg, videoTrack())
		require.ErrorContains(t, err, "beyond the mdat")
		requirePoolDrained(t, pool)
	})

	t.Run("moof over the size limit", func(t *testing.T) {
		pool := NewFragmentPool(FragmentPoolConfig{})
		fr := NewFragmentReader(pool, WithLimits(Limits{MaxMoofSize: 32}))
		err := fr.ReadFragments(bytes.NewReader(segment(good)), videoTrack(),
			func(s *Sample) error { s.Release(); return nil })
		require.ErrorContains(t, err, "moof exceeds the size limit")
		requirePoolDrained(t, pool)
	})

	t.Run("mdat over the size limit", func(t *testing.T) {
		pool := NewFragmentPool(FragmentPoolConfig{})
		fr := NewFragmentReader(pool, WithLimits(Limits{MaxMdatSize: 8}))
		err := fr.ReadFragments(bytes.NewReader(segment(good)), videoTrack(),
			func(s *Sample) error { s.Release(); return nil })
		require.ErrorContains(t, err, "mdat exceeds the size limit")
		requirePoolDrained(t, pool)
	})

	t.Run("moof without mdat", func(t *testing.T) {
		seg := segment(good)
		idx := bytes.Index(seg, []byte("mdat"))
		copy(seg[idx:], "moof")
		_, _, err := readSegment(t, seg, videoTrack())
		require.Error(t, err)
	})
}

// TestFragmentReader_ReaderErrorIsPreserved covers the classification the rung publisher depends on: an abort or a
// closed reader must arrive with its cause intact, a short read must not.
func TestFragmentReader_ReaderErrorIsPreserved(t *testing.T) {
	sentinel := errors.Str("page aborted")
	seg := segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(NaluIDR, 2048), dur: 3000}}}}}}})

	pool := NewFragmentPool(FragmentPoolConfig{})
	r := &failAfterReader{data: seg, after: len(seg) - 1024, err: sentinel}
	err := ReadFragments(r, videoTrack(), pool, func(s *Sample) error { s.Release(); return nil })
	require.True(t, errors.Is(err, sentinel), err)
	requirePoolDrained(t, pool)

	t.Run("short read is a truncated fragment", func(t *testing.T) {
		pool := NewFragmentPool(FragmentPoolConfig{})
		err := ReadFragments(bytes.NewReader(seg[:len(seg)-16]), videoTrack(), pool,
			func(s *Sample) error { s.Release(); return nil })
		require.ErrorContains(t, err, "truncated fragment")
		requirePoolDrained(t, pool)
	})
}

// failAfterReader serves data and then fails with err.
type failAfterReader struct {
	data  []byte
	pos   int
	after int
	err   error
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if r.pos >= r.after {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:r.after])
	r.pos += n
	return n, nil
}

// TestFragmentReader_CallbackErrorReleasesTheRest pins that the samples the callback never saw are released too.
func TestFragmentReader_CallbackErrorReleasesTheRest(t *testing.T) {
	samples := make([]tSample, 10)
	for i := range samples {
		samples[i] = tSample{data: nalu(1, 16), dur: 3000}
	}
	samples[0].data = nalu(NaluIDR, 16)
	seg := segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: samples}}}}})

	boom := errors.Str("stop")
	pool := NewFragmentPool(FragmentPoolConfig{})
	seen := 0
	err := ReadFragments(bytes.NewReader(seg), videoTrack(), pool, func(s *Sample) error {
		seen++
		s.Release()
		if seen == 4 {
			return boom
		}
		return nil
	})
	require.True(t, errors.Is(err, boom))
	require.Equal(t, 4, seen)
	requirePoolDrained(t, pool)
}

// TestFragmentReader_RetainedSampleKeepsTheFragment pins the ownership rule: a consumer that keeps a sample keeps its
// fragment out of the pool until it releases it.
func TestFragmentReader_RetainedSampleKeepsTheFragment(t *testing.T) {
	samples := []tSample{{data: nalu(NaluIDR, 40), dur: 3000}, {data: nalu(1, 20), dur: 3000}}
	seg := segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: samples}}}}})

	pool := NewFragmentPool(FragmentPoolConfig{})
	var kept []*Sample
	require.NoError(t, ReadFragments(bytes.NewReader(seg), videoTrack(), pool, func(s *Sample) error {
		kept = append(kept, s)
		return nil
	}))
	require.Len(t, kept, 2)
	require.EqualValues(t, 0, pool.Stats().Returned, "the fragment stays out while its samples are held")
	require.True(t, kept[0].Pooled())

	// both samples view the same fragment buffer
	require.Equal(t, &kept[0].Data[0], &kept[0].Data[0])
	require.Equal(t, 40, len(kept[0].Data)-4)

	kept[0].Release()
	require.EqualValues(t, 0, pool.Stats().Returned, "one sample still holds it")
	kept[1].Release()
	require.EqualValues(t, 1, pool.Stats().Returned)
}

// TestFragmentReader_SharedSampleTakesAReference covers the fan-out rule of the dispatcher.
func TestFragmentReader_SharedSampleTakesAReference(t *testing.T) {
	seg := segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: nalu(NaluIDR, 40), dur: 3000}}}}}}})

	pool := NewFragmentPool(FragmentPoolConfig{})
	var s *Sample
	require.NoError(t, ReadFragments(bytes.NewReader(seg), videoTrack(), pool, func(got *Sample) error {
		s = got
		got.ReferenceN(2) // three consumers in total
		return nil
	}))
	s.Release()
	s.ReleaseN(2)
	require.EqualValues(t, 1, pool.Stats().Returned)
}

func TestSample_NilFragIsNoop(t *testing.T) {
	s := &Sample{Data: []byte{1, 2, 3}}
	require.False(t, s.Pooled())
	s.Reference()
	s.ReferenceN(3)
	s.Release()
	s.ReleaseN(2)

	var nilSample *Sample
	nilSample.Release() // must not panic
}

func TestSample_Clone(t *testing.T) {
	src := &Sample{Track: videoTrack(), DTS: 10, PTS: 20, Duration: 30, Sync: true,
		Data: []byte{1, 2, 3}, NALUs: [][]byte{{1, 2}}, ParamSetChange: true}
	c := src.Clone()
	require.Equal(t, *src, *c)
	require.NotSame(t, &src.Data[0], &c.Data[0], "the payload is copied")
	require.NotSame(t, &src.NALUs[0][0], &c.NALUs[0][0])
	require.False(t, c.Pooled())
}

// TestFragmentPool_DropsOversizedBuffers pins the memory bound: a fragment whose buffer grew past the limit does not
// keep it while pooled.
func TestFragmentPool_DropsOversizedBuffers(t *testing.T) {
	pool := NewFragmentPool(FragmentPoolConfig{MaxRetainedBytes: 1024})
	ref := BorrowFragment(pool, videoTrack())
	ref.T.Grow(4096)
	ref.Release()

	ref2 := BorrowFragment(pool, videoTrack())
	defer ref2.Release()
	require.LessOrEqual(t, cap(ref2.T.Payload()), 1024)
}

// TestFragmentReader_ZeroAlloc is the regression test for the reason this parser exists: reading a segment must not
// allocate once the reader's scratch buffers and the pool are warm. Before it, one 119 frame segment cost about 5100
// allocations, three quarters of them inside the box decoder of the general purpose mp4 library.
func TestFragmentReader_ZeroAlloc(t *testing.T) {
	if raceflag.Enabled {
		// sync.Pool drops about a quarter of its puts under the race detector, so a pooled path allocates there
		t.Skip("allocation counts do not hold under the race detector")
	}
	v := parseFixtureInit(t, "vinit-stream0.m4s").Track(KindVideo)
	seg := fixture(t, "vchunk-stream0-00001.m4s")
	fr := NewFragmentReader(NewFragmentPool(FragmentPoolConfig{}))
	rd := bytes.NewReader(nil)
	release := func(s *Sample) error { s.Release(); return nil }

	allocs := testing.AllocsPerRun(20, func() {
		rd.Reset(seg)
		if err := fr.ReadFragments(rd, v, release); err != nil {
			t.Fatal(err)
		}
	})
	require.Zero(t, allocs, "reading a segment must not allocate")
}

// ---------------------------------------------------------------------------------------------------------------------
// differential test against mp4ff

// mp4ffSamples parses a segment with mp4ff, the reference implementation this parser replaced.
func mp4ffSamples(t *testing.T, seg []byte, track *TrackInfo) []mp4.FullSample {
	t.Helper()
	f, err := mp4.DecodeFile(bytes.NewReader(seg))
	require.NoError(t, err)
	var out []mp4.FullSample
	for _, s := range f.Segments {
		for _, frag := range s.Fragments {
			full, err := frag.GetFullSamples(track.Trex)
			require.NoError(t, err)
			out = append(out, full...)
		}
	}
	return out
}

func TestFragmentReader_MatchesMp4ff(t *testing.T) {
	multi := make([]tSample, 12)
	for i := range multi {
		multi[i] = tSample{data: nalu(1, 12+i), dur: 3000, cto: int32((i%4 - 1) * 1000)}
	}

	for _, tc := range []struct {
		name  string
		seg   []byte
		track *TrackInfo
	}{
		{"video fixture", fixtureBytes(t, "vchunk-stream0-00001.m4s"), fixtureTrack(t, "vinit-stream0.m4s", KindVideo)},
		{"audio fixture", fixtureBytes(t, "achunk-00001.m4s"), fixtureTrack(t, "ainit.m4s", KindAudio)},
		{"audio fixture 2", fixtureBytes(t, "achunk-00002.m4s"), fixtureTrack(t, "ainit.m4s", KindAudio)},
		// an HEVC segment: the parser is codec agnostic, only the AVCC split is not, so it is read as an opaque track
		{"hevc fixture", fixtureBytes(t, "hevc-chunk.m4s"), opaqueTrack(1)},
		{"multi sample trun", segment(tFrag{trafs: []tTraf{{trackID: 1, baseDecodeTime: 6000,
			defaultBaseIsMoof: true, truns: []tTrun{{version: 1, samples: multi}}}}}), opaqueTrack(1)},
		// Every trun carries its own data offset: mp4ff resolves a trun without one from the moof start rather than
		// from the end of the previous run (ISO/IEC 14496-12 8.8.8.2), so that case is covered by the parser's own
		// tests instead of against this oracle.
		{"several truns", segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true, truns: []tTrun{
			{version: 1, samples: multi[:4]},
			{version: 1, samples: multi[4:8]},
		}}}}), opaqueTrack(1)},
		{"tfhd defaults", segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
			defaultDuration: 1024, defaultSize: 16, defaultFlags: mp4.SyncSampleFlags,
			truns: []tTrun{{samples: []tSample{{data: bytes.Repeat([]byte{7}, 16)},
				{data: bytes.Repeat([]byte{8}, 16)}}, noDuration: true, noSize: true, noCto: true}}}}}),
			opaqueTrack(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := mp4ffSamples(t, tc.seg, tc.track)
			got, pool, err := readSegment(t, tc.seg, tc.track)
			require.NoError(t, err)
			requirePoolDrained(t, pool)
			require.Len(t, got, len(want))
			for i := range want {
				require.Equal(t, int64(want[i].DecodeTime)-tc.track.EditMediaTime, got[i].DTS, "sample %d dts", i)
				require.Equal(t, want[i].PresentationTime()-tc.track.EditMediaTime, got[i].PTS, "sample %d pts", i)
				require.Equal(t, want[i].Dur, got[i].Duration, "sample %d duration", i)
				// only a video sample takes its sync flag from the container; every other kind is always a random
				// access point, which is what a receiver of AAC or an opaque codec expects
				wantSync := tc.track.Kind != KindVideo || want[i].IsSync() || isIDR(got[i])
				require.Equal(t, wantSync, got[i].Sync, "sample %d sync", i)
				require.Equal(t, want[i].Data, got[i].Data, "sample %d payload", i)
			}
		})
	}
}

func isIDR(s *Sample) bool {
	for _, n := range s.NALUs {
		if NaluType(n) == NaluIDR {
			return true
		}
	}
	return false
}

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	return fixture(t, name)
}

func fixtureTrack(t *testing.T, initName string, kind Kind) *TrackInfo {
	t.Helper()
	return parseFixtureInit(t, initName).Track(kind)
}

// FuzzFragmentReader checks that no input makes the parser panic, read out of bounds or leak a fragment.
//
// It deliberately does not compare against mp4ff: the two readers disagree about which bytes are a box as soon as the
// children of a container do not tile it, for instance when a tfhd declares a size that swallows the tfdt behind it.
// Both readings are defensible for input that is corrupt either way, so the comparison belongs on well-formed input,
// where TestFragmentReader_MatchesMp4ff does it.
func FuzzFragmentReader(f *testing.F) {
	f.Add(segment(tFrag{trafs: []tTraf{{trackID: 1, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: bytes.Repeat([]byte{1}, 16), dur: 3000}}}}}}}))
	f.Add(segment(tFrag{trafs: []tTraf{{trackID: 1, tfdtV0: true, defaultBaseIsMoof: true, defaultSize: 8,
		truns: []tTrun{{samples: []tSample{{data: bytes.Repeat([]byte{2}, 8)}}, noSize: true, noCto: true}}}}}))
	f.Add(segment(tFrag{trafs: []tTraf{{trackID: 9, defaultBaseIsMoof: true,
		truns: []tTrun{{version: 1, samples: []tSample{{data: bytes.Repeat([]byte{3}, 16), dur: 3000}}}}}}}))
	f.Add(box("styp", []byte("cmfs")))

	f.Fuzz(func(t *testing.T, seg []byte) {
		// two track ids, so both the "traf found" and the "fragment skipped" paths are exercised
		for _, trackID := range []uint32{1, 0x30303030} {
			pool := NewFragmentPool(FragmentPoolConfig{})
			samples, payload := 0, 0
			err := ReadFragments(bytes.NewReader(seg), opaqueTrack(trackID), pool, func(s *Sample) error {
				samples++
				payload += len(s.Data)
				s.Release()
				return nil
			})
			if st := pool.Stats(); st.Borrowed != st.Returned {
				t.Fatalf("leaked %d fragments (track %d)", st.Borrowed-st.Returned, trackID)
			}
			if err == nil && payload > len(seg) {
				t.Fatalf("delivered %d payload bytes from a %d byte segment", payload, len(seg))
			}
		}
	})
}
