package fmp4

import (
	"io"

	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/eluv-io/errors-go"
)

// Sample is one access unit read from a media segment, with its timing in the track's timescale, relative to the
// period the segment belongs to (the edit list offset already removed).
type Sample struct {
	Track    *TrackInfo
	DTS      int64  // decode time in ticks of Track.Timescale
	PTS      int64  // presentation time in ticks of Track.Timescale
	Duration uint32 // sample duration in ticks
	Sync     bool   // a random access point: an IDR picture for video, always true for audio
	// Data is the raw sample payload: length-prefixed NAL units for H.264 (AVCC), a raw frame for AAC. It aliases the
	// segment's mdat buffer, which is allocated per fragment and never reused, so it may be retained.
	Data []byte
	// NALUs holds the H.264 NAL units of the sample without access unit delimiters and filler data. They alias Data.
	// Nil for audio.
	NALUs [][]byte
	// ParamSetChange is set when the sample carries an SPS or PPS that differs from the init segment's.
	ParamSetChange bool
}

// ReadFragments reads one media segment (a sequence of moof/mdat fragments, optionally preceded by styp/sidx boxes)
// from r and calls fn once per sample, in decode order. It returns once r reaches EOF, or with the first error from
// the parser, the reader or fn. A blocking reader is fine: the parser reads sequentially and only ever needs the
// current fragment in memory, so a segment can be consumed while it is still being produced.
func ReadFragments(r io.Reader, track *TrackInfo, fn func(*Sample) error) error {
	e := errors.Template("fmp4.ReadFragments", errors.K.Invalid.Default(), "track_id", track.TrackID)

	cb := func(f *mp4.Fragment, sa mp4.SampleAccessor) error {
		if f.Mdat.IsLazy() {
			buf := make([]byte, f.Mdat.GetLazyDataSize())
			if _, err := sa.ReadMdatData(buf); err != nil {
				return errors.E("readMdat", errors.K.IO, err)
			}
			f.Mdat.SetData(buf)
		}
		// GetFullSamples resolves the per-sample defaults from tfhd and, failing that, from the init segment's trex,
		// which the streaming parser does not have when it is fed a media segment on its own.
		full, err := f.GetFullSamples(track.Trex)
		if err != nil {
			return errors.E("getSamples", errors.K.Invalid, err)
		}
		for i := range full {
			s, err := newSample(track, &full[i])
			if err != nil {
				return err
			}
			if err = fn(s); err != nil {
				return err
			}
		}
		return nil
	}

	sf, err := mp4.InitDecodeStream(r, mp4.WithFragmentCallback(cb), mp4.WithMaxFragments(1))
	if err != nil {
		return e(err, "reason", "failed to start segment decoding")
	}
	// The parser records an init segment under Init when fragments follow it, and leaves a bare moov under Moov when
	// the input ends there; both mean the caller handed us an init segment.
	if sf.Init != nil || sf.Moov != nil {
		return e("reason", "unexpected init segment in media segment")
	}
	if err = sf.ProcessFragments(); err != nil {
		if cause := unwrapCallbackErr(err); cause != nil {
			return cause
		}
		return e(err, "reason", "failed to read fragments")
	}
	return nil
}

// unwrapCallbackErr returns the error our own fragment callback produced, if err wraps one, so a caller's error (or
// a reader's io error) is returned as-is rather than buried under the parser's formatting.
func unwrapCallbackErr(err error) error {
	for cur := err; cur != nil; {
		if _, ok := cur.(*errors.Error); ok {
			return cur
		}
		u, ok := cur.(interface{ Unwrap() error })
		if !ok {
			return nil
		}
		cur = u.Unwrap()
	}
	return nil
}

func newSample(track *TrackInfo, fs *mp4.FullSample) (*Sample, error) {
	s := &Sample{
		Track:    track,
		DTS:      int64(fs.DecodeTime) - track.EditMediaTime,
		PTS:      fs.PresentationTime() - track.EditMediaTime,
		Duration: fs.Dur,
		Data:     fs.Data,
	}
	switch track.Kind {
	case KindVideo:
		nalus, err := SplitAVCC(fs.Data, track.NALULengthSize)
		if err != nil {
			return nil, errors.E("fmp4.newSample", errors.K.Invalid, err, "track_id", track.TrackID, "dts", s.DTS)
		}
		au := inspectAccessUnit(nalus, track.SPS, track.PPS)
		s.NALUs = au.nalus
		s.Sync = fs.IsSync() || au.idr
		s.ParamSetChange = au.paramSetChange
	default:
		s.Sync = true
	}
	return s, nil
}
