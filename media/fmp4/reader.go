package fmp4

import (
	"io"
	"math"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/eluv-io/errors-go"
)

// Limits bounds what a FragmentReader buffers, so a corrupt or hostile segment cannot make it allocate without end.
type Limits struct {
	MaxMoofSize int // the largest moof box accepted; 0 selects DefaultMaxMoofSize
	MaxMdatSize int // the largest mdat box accepted; 0 selects DefaultMaxMdatSize
}

const (
	DefaultMaxMoofSize = 1 << 20
	DefaultMaxMdatSize = 64 << 20

	boxHeaderSize = 8
	skipChunkSize = 4096 // the scratch buffer is at least this large, to skip boxes of no interest in one read
)

// Box types compared as arrays, so no string is built per box.
var (
	typFtyp = [4]byte{'f', 't', 'y', 'p'}
	typMoov = [4]byte{'m', 'o', 'o', 'v'}
	typMoof = [4]byte{'m', 'o', 'o', 'f'}
	typMdat = [4]byte{'m', 'd', 'a', 't'}
	typTraf = [4]byte{'t', 'r', 'a', 'f'}
	typTfhd = [4]byte{'t', 'f', 'h', 'd'}
	typTfdt = [4]byte{'t', 'f', 'd', 't'}
	typTrun = [4]byte{'t', 'r', 'u', 'n'}
)

// errTruncated is the reason a segment ends inside a box.
var errTruncated = errors.NoTrace("fmp4.FragmentReader", errors.K.Invalid, "reason", "truncated fragment")

// defaultPool serves callers of the package-level ReadFragments that pass no pool.
var defaultPool = NewFragmentPool(FragmentPoolConfig{})

// FragmentReader is a streaming parser for fMP4 media segments that hands out samples as views into pooled
// fragments. It reads a segment box by box, buffers only the moof of the current fragment in a scratch buffer it keeps
// across segments, and reads each mdat straight into a fragment borrowed from its pool, so in steady state a segment
// is parsed without heap allocation and its media bytes are copied once. One reader serves one goroutine.
//
// The parser reuses mp4ff's flag constants and sample flag helpers and is shaped like its DecodeXxxSR decoders, so
// the code can move into the mp4ff fork as a zero-copy fragment API later; only the init segment goes through mp4ff
// itself (ParseInit).
//
// It reads the CMAF profile of fragmented MP4 rather than every representation ISO/IEC 14496-12 allows. The one
// restriction a valid file can run into: a second or later traf that sets neither base-data-offset-present nor
// default-base-is-moof, whose data the spec places after the preceding track fragment's, is refused instead of
// resolved. Everything else a general file may do, including several truns, explicit base data offsets, negative
// composition offsets and both trun versions, is read.
type FragmentReader struct {
	pool   *FragmentPool
	limits Limits

	hdr [16]byte // box header scratch
	// moof holds the current moof's body. The trun sample tables are read out of it while the samples are built, so
	// it must not be reused before then. That is why skipping has a buffer of its own.
	moof    []byte
	skipBuf []byte
	runs    []trunRun // the trun boxes of the current fragment's track
	pos     uint64    // offset of the next byte to read, from the start of the segment
}

// ReaderOption configures a FragmentReader.
type ReaderOption func(*FragmentReader)

// WithLimits sets the box size limits.
func WithLimits(l Limits) ReaderOption {
	return func(fr *FragmentReader) { fr.limits = l }
}

// NewFragmentReader creates a reader borrowing fragments from pool (nil selects a process-wide pool).
func NewFragmentReader(pool *FragmentPool, opts ...ReaderOption) *FragmentReader {
	if pool == nil {
		pool = defaultPool
	}
	fr := &FragmentReader{pool: pool}
	for _, opt := range opts {
		opt(fr)
	}
	if fr.limits.MaxMoofSize <= 0 {
		fr.limits.MaxMoofSize = DefaultMaxMoofSize
	}
	if fr.limits.MaxMdatSize <= 0 {
		fr.limits.MaxMdatSize = DefaultMaxMdatSize
	}
	return fr
}

// boxHeader is a decoded box header, the equivalent of mp4ff's BoxHeader without the type string.
type boxHeader struct {
	typ    [4]byte
	size   uint64 // the whole box including the header
	hdrLen int    // 8, or 16 with a large size
}

func (h boxHeader) bodyLen() uint64 {
	return h.size - uint64(h.hdrLen)
}

// tfhdBox mirrors mp4.TfhdBox.
type tfhdBox struct {
	flags                  uint32
	trackID                uint32
	baseDataOffset         uint64
	sampleDescriptionIndex uint32
	defaultSampleDuration  uint32
	defaultSampleSize      uint32
	defaultSampleFlags     uint32
}

// trunRun mirrors mp4.TrunBox, with the sample table left in place in the moof scratch and read row by row.
type trunRun struct {
	version          byte
	flags            uint32
	sampleCount      uint32
	dataOffset       int32
	firstSampleFlags uint32
	rows             []byte // the sample table: sampleCount rows of rowSize bytes
	rowSize          int
	relStart         uint64 // offset of the run's first sample within the mdat payload, set by resolveRuns
}

func (t *trunRun) has(flag uint32) bool {
	return t.flags&flag != 0
}

// fragMeta is what the moof of one fragment says about the track being read.
type fragMeta struct {
	found          bool // the moof carries a traf for the track
	trafIndex      int  // position of that traf among the moof's trafs
	tfhd           tfhdBox
	baseDecodeTime uint64
}

// ReadFragments reads one media segment from r and calls fn once per sample of track, in decode order. See the
// package-level ReadFragments for the contract.
func (fr *FragmentReader) ReadFragments(r io.Reader, track *TrackInfo, fn SampleFunc) error {
	fr.pos = 0
	for {
		hdr, err := fr.readBoxHeader(r)
		if err == io.EOF {
			// A segment with no fragment at all is not an error. A live transcode that is abandoned mid-part serves
			// exactly that, and the caller has to move on to the next segment rather than end the period.
			return nil
		}
		if err != nil {
			return err
		}
		switch hdr.typ {
		case typMoof:
			if err = fr.readFragment(r, hdr, track, fn); err != nil {
				return err
			}
		case typFtyp, typMoov:
			return errors.E("fmp4.ReadFragments", errors.K.Invalid, "reason", "unexpected init segment in media segment",
				"track_id", track.TrackID)
		case typMdat:
			return errors.E("fmp4.ReadFragments", errors.K.Invalid, "reason", "mdat without a preceding moof",
				"track_id", track.TrackID)
		default:
			if err = fr.skip(r, hdr.bodyLen()); err != nil {
				return err
			}
		}
	}
}

// readFragment parses one moof, whose header was just read, and the mdat that follows it, and delivers the samples.
func (fr *FragmentReader) readFragment(r io.Reader, moofHdr boxHeader, track *TrackInfo, fn SampleFunc) error {
	moofStart := fr.pos - uint64(moofHdr.hdrLen)
	if moofHdr.bodyLen() > uint64(fr.limits.MaxMoofSize) {
		return errors.E("fmp4.readFragment", errors.K.Invalid, "reason", "moof exceeds the size limit",
			"size", moofHdr.size, "limit", fr.limits.MaxMoofSize)
	}
	body := fr.growMoof(int(moofHdr.bodyLen()))
	if err := fr.readFull(r, body); err != nil {
		return err
	}
	meta, err := fr.parseMoof(body, track.TrackID)
	if err != nil {
		return err
	}

	// the mdat follows the moof, possibly after boxes such as prft or emsg
	var mdatHdr boxHeader
	for {
		mdatHdr, err = fr.readBoxHeader(r)
		if err == io.EOF {
			return errTruncated
		}
		if err != nil {
			return err
		}
		if mdatHdr.typ == typMdat {
			break
		}
		if mdatHdr.typ == typMoof {
			return errors.E("fmp4.readFragment", errors.K.Invalid, "reason", "moof without mdat")
		}
		if err = fr.skip(r, mdatHdr.bodyLen()); err != nil {
			return err
		}
	}
	mdatSize := mdatHdr.bodyLen()
	if mdatSize > uint64(fr.limits.MaxMdatSize) {
		return errors.E("fmp4.readFragment", errors.K.Invalid, "reason", "mdat exceeds the size limit",
			"size", mdatHdr.size, "limit", fr.limits.MaxMdatSize)
	}
	if !meta.found {
		// the fragment carries no samples of this track (e.g. an audio-only moof in a muxed stream)
		return fr.skip(r, mdatSize)
	}

	need, err := fr.resolveRuns(&meta, moofStart, fr.pos, mdatSize, track.Trex)
	if err != nil {
		return err
	}

	ref := BorrowFragment(fr.pool, track)
	frag := ref.T
	if err = fr.readFull(r, frag.Grow(int(need))); err != nil {
		ref.Release()
		return err
	}
	if err = fr.skip(r, mdatSize-need); err != nil {
		ref.Release()
		return err
	}
	if err = fr.fillSamples(frag, &meta, track); err != nil {
		ref.Release()
		return err
	}

	n := len(frag.Samples)
	ref.ReferenceN(n)
	for i := 0; i < n; i++ {
		if err = fn(&frag.Samples[i]); err != nil {
			ref.ReleaseN(n - 1 - i)
			ref.Release()
			return err
		}
	}
	ref.Release()
	return nil
}

// readBoxHeader reads the next box header. It returns io.EOF when the segment ends exactly at a box boundary.
func (fr *FragmentReader) readBoxHeader(r io.Reader) (boxHeader, error) {
	n, err := io.ReadFull(r, fr.hdr[:boxHeaderSize])
	if err != nil {
		if err == io.EOF && n == 0 {
			return boxHeader{}, io.EOF
		}
		return boxHeader{}, fr.readErr(err)
	}
	fr.pos += boxHeaderSize
	hdr := boxHeader{
		size:   uint64(fr.hdr[0])<<24 | uint64(fr.hdr[1])<<16 | uint64(fr.hdr[2])<<8 | uint64(fr.hdr[3]),
		hdrLen: boxHeaderSize,
	}
	copy(hdr.typ[:], fr.hdr[4:8])
	switch hdr.size {
	case 0:
		return hdr, errors.E("fmp4.readBoxHeader", errors.K.Invalid, "reason", "box extending to the end of the file",
			"type", string(hdr.typ[:]))
	case 1:
		// a large size is only ever needed by the mdat, as mp4ff's DecodeHeaderSR assumes as well
		if hdr.typ != typMdat {
			return hdr, errors.E("fmp4.readBoxHeader", errors.K.Invalid, "reason", "large size on a box other than mdat",
				"type", string(hdr.typ[:]))
		}
		if err = fr.readFull(r, fr.hdr[8:16]); err != nil {
			return hdr, err
		}
		hdr.size = uint64(fr.hdr[8])<<56 | uint64(fr.hdr[9])<<48 | uint64(fr.hdr[10])<<40 | uint64(fr.hdr[11])<<32 |
			uint64(fr.hdr[12])<<24 | uint64(fr.hdr[13])<<16 | uint64(fr.hdr[14])<<8 | uint64(fr.hdr[15])
		hdr.hdrLen = 16
	}
	if hdr.size < uint64(hdr.hdrLen) {
		return hdr, errors.E("fmp4.readBoxHeader", errors.K.Invalid, "reason", "box smaller than its header",
			"type", string(hdr.typ[:]), "size", hdr.size)
	}
	return hdr, nil
}

// readFull fills buf from r, advancing the position. A short read inside a box is a truncated fragment; any other
// reader error is passed on with its cause intact, so callers can tell a cancelled or aborted read apart.
func (fr *FragmentReader) readFull(r io.Reader, buf []byte) error {
	n, err := io.ReadFull(r, buf)
	fr.pos += uint64(n)
	if err != nil {
		return fr.readErr(err)
	}
	return nil
}

func (fr *FragmentReader) readErr(err error) error {
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return errTruncated
	}
	return errors.E("fmp4.FragmentReader", errors.K.IO, err)
}

// skip consumes n bytes of r.
func (fr *FragmentReader) skip(r io.Reader, n uint64) error {
	if n == 0 {
		return nil
	}
	if cap(fr.skipBuf) < skipChunkSize {
		fr.skipBuf = make([]byte, skipChunkSize)
	}
	for n > 0 {
		chunk := fr.skipBuf[:skipChunkSize]
		if n < uint64(len(chunk)) {
			chunk = chunk[:n]
		}
		if err := fr.readFull(r, chunk); err != nil {
			return err
		}
		n -= uint64(len(chunk))
	}
	return nil
}

// growMoof returns the moof scratch buffer with length n, reallocating only when it has to.
func (fr *FragmentReader) growMoof(n int) []byte {
	if cap(fr.moof) < n {
		fr.moof = make([]byte, n)
	}
	fr.moof = fr.moof[:n]
	return fr.moof
}

// parseMoof walks the moof body and collects the traf of trackID: its tfhd, tfdt and truns (into fr.runs).
func (fr *FragmentReader) parseMoof(body []byte, trackID uint32) (fragMeta, error) {
	var meta fragMeta
	fr.runs = fr.runs[:0]
	cur := newBoxCursor(body)
	trafs := 0
	for cur.NrRemainingBytes() > 0 {
		child, sub, err := childBox(&cur)
		if err != nil {
			return meta, err
		}
		if child != typTraf {
			continue
		}
		before := len(fr.runs)
		tfhd, tfdt, err := fr.parseTraf(sub)
		if err != nil {
			return meta, err
		}
		if tfhd.trackID != trackID || meta.found {
			fr.runs = fr.runs[:before]
		} else {
			meta.found = true
			meta.trafIndex = trafs
			meta.tfhd = tfhd
			meta.baseDecodeTime = tfdt
		}
		trafs++
	}
	return meta, cur.AccError()
}

// childBox reads the header of the next child box and returns its type and a cursor over its body.
func childBox(cur *boxCursor) ([4]byte, boxCursor, error) {
	var typ [4]byte
	size := cur.ReadUint32()
	typ[0], typ[1], typ[2], typ[3] = cur.ReadUint8(), cur.ReadUint8(), cur.ReadUint8(), cur.ReadUint8()
	if err := cur.AccError(); err != nil {
		return typ, boxCursor{}, err
	}
	if size < boxHeaderSize {
		return typ, boxCursor{}, errors.E("fmp4.parseMoof", errors.K.Invalid, "reason", "child box smaller than its header",
			"type", string(typ[:]), "size", size)
	}
	sub := cur.Sub(int(size) - boxHeaderSize)
	return typ, sub, cur.AccError()
}

// parseTraf decodes the tfhd, tfdt and trun boxes of one traf, appending the truns to fr.runs.
func (fr *FragmentReader) parseTraf(cur boxCursor) (tfhd tfhdBox, baseDecodeTime uint64, err error) {
	hasTfhd := false
	for cur.NrRemainingBytes() > 0 {
		var child [4]byte
		var sub boxCursor
		child, sub, err = childBox(&cur)
		if err != nil {
			return tfhd, 0, err
		}
		switch child {
		case typTfhd:
			tfhd, err = decodeTfhd(&sub)
			hasTfhd = true
		case typTfdt:
			baseDecodeTime, err = decodeTfdt(&sub)
		case typTrun:
			var run trunRun
			run, err = decodeTrun(&sub)
			if err == nil {
				fr.runs = append(fr.runs, run)
			}
		}
		if err != nil {
			return tfhd, 0, err
		}
	}
	if !hasTfhd {
		return tfhd, 0, errors.E("fmp4.parseTraf", errors.K.Invalid, "reason", "traf without tfhd")
	}
	return tfhd, baseDecodeTime, cur.AccError()
}

// decodeTfhd mirrors mp4.DecodeTfhdSR.
func decodeTfhd(sr *boxCursor) (tfhdBox, error) {
	versionAndFlags := sr.ReadUint32()
	t := tfhdBox{flags: versionAndFlags & 0x00ffffff}
	t.trackID = sr.ReadUint32()
	if t.flags&mp4.TfhdBaseDataOffsetPresentFlag != 0 {
		t.baseDataOffset = sr.ReadUint64()
	}
	if t.flags&mp4.TfhdSampleDescriptionIndexPresentFlag != 0 {
		t.sampleDescriptionIndex = sr.ReadUint32()
	}
	if t.flags&mp4.TfhdDefaultSampleDurationPresentFlag != 0 {
		t.defaultSampleDuration = sr.ReadUint32()
	}
	if t.flags&mp4.TfhdDefaultSampleSizePresentFlag != 0 {
		t.defaultSampleSize = sr.ReadUint32()
	}
	if t.flags&mp4.TfhdDefaultSampleFlagsPresentFlag != 0 {
		t.defaultSampleFlags = sr.ReadUint32()
	}
	return t, sr.AccError()
}

// decodeTfdt mirrors mp4.DecodeTfdtSR.
func decodeTfdt(sr *boxCursor) (uint64, error) {
	versionAndFlags := sr.ReadUint32()
	if versionAndFlags>>24 == 0 {
		return uint64(sr.ReadUint32()), sr.AccError()
	}
	return sr.ReadUint64(), sr.AccError()
}

// decodeTrun mirrors mp4.DecodeTrunSR up to the sample table, which it leaves in place to be read row by row.
func decodeTrun(sr *boxCursor) (trunRun, error) {
	versionAndFlags := sr.ReadUint32()
	t := trunRun{
		version:     byte(versionAndFlags >> 24),
		flags:       versionAndFlags & 0x00ffffff,
		sampleCount: sr.ReadUint32(),
	}
	if t.has(mp4.TrunDataOffsetPresentFlag) {
		t.dataOffset = sr.ReadInt32()
	}
	if t.has(mp4.TrunFirstSampleFlagsPresentFlag) {
		t.firstSampleFlags = sr.ReadUint32()
	}
	if err := sr.AccError(); err != nil {
		return t, err
	}
	for _, f := range [...]uint32{mp4.TrunSampleDurationPresentFlag, mp4.TrunSampleSizePresentFlag,
		mp4.TrunSampleFlagsPresentFlag, mp4.TrunSampleCompositionTimeOffsetPresentFlag} {
		if t.has(f) {
			t.rowSize += 4
		}
	}
	if t.rowSize == 0 && t.sampleCount > 1024 {
		return t, errors.E("fmp4.decodeTrun", errors.K.Invalid,
			"reason", "trun: sample count is big but no sample data present", "sample_count", t.sampleCount)
	}
	if uint64(sr.NrRemainingBytes()) != uint64(t.sampleCount)*uint64(t.rowSize) {
		return t, errors.E("fmp4.decodeTrun", errors.K.Invalid, "reason", "trun: size does not match its sample count",
			"sample_count", t.sampleCount, "row_size", t.rowSize, "remaining", sr.NrRemainingBytes())
	}
	t.rows = sr.b[sr.pos:]
	return t, nil
}

// sampleDefaults are the per-sample values a trun row may omit, resolved from the tfhd and then the trex.
type sampleDefaults struct {
	duration, size, flags uint32
	hasSize               bool
}

func resolveDefaults(tfhd *tfhdBox, trex *mp4.TrexBox) sampleDefaults {
	var d sampleDefaults
	switch {
	case tfhd.flags&mp4.TfhdDefaultSampleDurationPresentFlag != 0:
		d.duration = tfhd.defaultSampleDuration
	case trex != nil:
		d.duration = trex.DefaultSampleDuration
	}
	switch {
	case tfhd.flags&mp4.TfhdDefaultSampleSizePresentFlag != 0:
		d.size, d.hasSize = tfhd.defaultSampleSize, true
	case trex != nil:
		d.size, d.hasSize = trex.DefaultSampleSize, true
	}
	switch {
	case tfhd.flags&mp4.TfhdDefaultSampleFlagsPresentFlag != 0:
		d.flags = tfhd.defaultSampleFlags
	case trex != nil:
		d.flags = trex.DefaultSampleFlags
	}
	return d
}

// resolveRuns locates every run's samples within the mdat payload and returns how many payload bytes they span, so
// the mdat can be read into a buffer of exactly that size before any sample is built.
func (fr *FragmentReader) resolveRuns(meta *fragMeta, moofStart, mdatPayloadStart, mdatSize uint64,
	trex *mp4.TrexBox) (need uint64, err error) {

	// The template is built inside the closure, so the success path of this per-fragment function does not pay for
	// boxing its fields into a slice.
	e := func(f ...interface{}) *errors.Error {
		return errors.Template("fmp4.resolveRuns", errors.K.Invalid)(f...)
	}

	defaults := resolveDefaults(&meta.tfhd, trex)
	base := moofStart
	switch {
	case meta.tfhd.flags&mp4.TfhdBaseDataOffsetPresentFlag != 0:
		base = meta.tfhd.baseDataOffset
	case meta.tfhd.flags&mp4.TfhdDefaultBaseIsMoofFlag == 0 && meta.trafIndex > 0:
		// ISO/IEC 14496-12 puts this traf's data at the end of the preceding track fragment's, which would mean
		// resolving every run of every traf before it rather than only the requested track's. No encoder in the fabric
		// writes this: CMAF requires default-base-is-moof, and everything that reaches this reader is CMAF. It is
		// refused rather than guessed at, so a file that does use it fails loudly instead of decoding to noise.
		return 0, e("reason", "traf without default-base-is-moof after another traf")
	}

	// Every bound below is checked by subtraction against mdatSize rather than by adding sizes up and comparing. The
	// offsets come from the file: a base_data_offset near the top of the range, or a sample count times a default
	// size, overflows uint64 on the way and would otherwise pass a comparison that adds first.
	end := base // where the previous run's data ended, the start of a run without a data offset
	for i := range fr.runs {
		run := &fr.runs[i]
		start := end
		if run.has(mp4.TrunDataOffsetPresentFlag) {
			if start, err = addOffset(base, run.dataOffset); err != nil {
				return 0, e(err, "base", base, "data_offset", run.dataOffset)
			}
		}
		if start < mdatPayloadStart {
			return 0, e("reason", "sample data before the mdat payload", "start", start, "mdat", mdatPayloadStart)
		}
		run.relStart = start - mdatPayloadStart
		if run.relStart > mdatSize {
			return 0, e("reason", "sample data beyond the mdat", "start", run.relStart, "mdat_size", mdatSize)
		}
		avail := mdatSize - run.relStart

		total := uint64(0)
		if run.has(mp4.TrunSampleSizePresentFlag) {
			rows := newBoxCursor(run.rows)
			sizeOff := 0
			if run.has(mp4.TrunSampleDurationPresentFlag) {
				sizeOff = 4
			}
			for j := uint32(0); j < run.sampleCount; j++ {
				rows.SkipBytes(sizeOff)
				size := uint64(rows.ReadUint32())
				if size > avail-total {
					return 0, e("reason", "sample data beyond the mdat", "start", run.relStart, "mdat_size", mdatSize)
				}
				total += size
				rows.SkipBytes(run.rowSize - sizeOff - 4)
			}
			if err = rows.AccError(); err != nil {
				return 0, err
			}
		} else {
			if !defaults.hasSize {
				return 0, e("reason", "sample sizes neither in the trun nor in tfhd or trex defaults")
			}
			if defaults.size > 0 && uint64(run.sampleCount) > avail/uint64(defaults.size) {
				return 0, e("reason", "sample data beyond the mdat", "start", run.relStart, "samples", run.sampleCount,
					"sample_size", defaults.size, "mdat_size", mdatSize)
			}
			total = uint64(run.sampleCount) * uint64(defaults.size)
		}
		end = start + total
		if run.relStart+total > need {
			need = run.relStart + total
		}
	}
	return need, nil
}

// addOffset applies a trun's signed data offset to the base offset, refusing a sum that leaves the file's range.
func addOffset(base uint64, offset int32) (uint64, error) {
	if offset >= 0 {
		if uint64(offset) > math.MaxUint64-base {
			return 0, errors.NoTrace("addOffset", errors.K.Invalid, "reason", "data offset overflows the base offset")
		}
		return base + uint64(offset), nil
	}
	neg := uint64(-int64(offset))
	if neg > base {
		return 0, errors.NoTrace("addOffset", errors.K.Invalid, "reason", "data offset precedes the file start")
	}
	return base - neg, nil
}

// fillSamples builds the fragment's samples from the trun rows, with the payload already read.
func (fr *FragmentReader) fillSamples(frag *Fragment, meta *fragMeta, track *TrackInfo) error {
	defaults := resolveDefaults(&meta.tfhd, track.Trex)
	payload := frag.Payload()
	decodeTime := meta.baseDecodeTime
	for i := range fr.runs {
		run := &fr.runs[i]
		rows := newBoxCursor(run.rows)
		off := run.relStart
		for j := uint32(0); j < run.sampleCount; j++ {
			dur, size, flags := defaults.duration, defaults.size, defaults.flags
			var cto int32
			if run.has(mp4.TrunSampleDurationPresentFlag) {
				dur = rows.ReadUint32()
			}
			if run.has(mp4.TrunSampleSizePresentFlag) {
				size = rows.ReadUint32()
			}
			if run.has(mp4.TrunSampleFlagsPresentFlag) {
				flags = rows.ReadUint32()
			} else if j == 0 && run.has(mp4.TrunFirstSampleFlagsPresentFlag) {
				flags = run.firstSampleFlags
			}
			if run.has(mp4.TrunSampleCompositionTimeOffsetPresentFlag) {
				// read as signed for both trun versions, as mp4ff does; a version 0 offset never uses the top bit
				cto = rows.ReadInt32()
			}
			if err := rows.AccError(); err != nil {
				return err
			}
			data := payload[off : off+uint64(size)]
			off += uint64(size)
			dts := int64(decodeTime) - track.EditMediaTime
			pts := dts + int64(cto)
			decodeTime += uint64(dur)
			mp4Sample := mp4.Sample{Flags: flags}
			if _, err := frag.AppendSample(data, dts, pts, dur, mp4Sample.IsSync()); err != nil {
				return err
			}
		}
	}
	return nil
}
