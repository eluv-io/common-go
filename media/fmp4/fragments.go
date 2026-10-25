package fmp4

import (
	"io"
)

// Sample is one access unit read from a media segment, with its timing in the track's timescale, relative to the
// period the segment belongs to (the edit list offset already removed).
//
// A sample produced by a FragmentReader views a pooled Fragment and carries one reference to it. Data, NALUs and the
// Sample itself are valid only while that reference is held: whoever receives a sample owns its reference and either
// hands it on unchanged or calls Release when done with it; sharing the sample with several consumers takes one
// Reference per additional consumer, each of whom releases its own. The reference methods are no-ops on a sample
// built by hand with a struct literal, which is not pooled.
type Sample struct {
	Track    *TrackInfo
	DTS      int64  // decode time in ticks of Track.Timescale
	PTS      int64  // presentation time in ticks of Track.Timescale
	Duration uint32 // sample duration in ticks
	Sync     bool   // a random access point: an IDR picture for video, always true for audio
	// Data is the raw sample payload: length-prefixed NAL units for H.264 (AVCC), a raw frame for AAC. It views the
	// fragment's mdat buffer and is valid until the sample's last reference is released.
	Data []byte
	// NALUs holds the H.264 NAL units of the sample without access unit delimiters and filler data. They alias Data.
	// Nil for audio.
	NALUs [][]byte
	// ParamSetChange is set when the sample carries an SPS or PPS that differs from the init segment's.
	ParamSetChange bool

	frag FragmentRef // the pooled fragment this sample views; nil for a sample that is not pooled
}

// SampleFunc receives the samples of a segment in decode order. The sample carries one reference to its fragment
// which the function owns on return, whether it returns an error or not: it releases the reference, or hands the
// sample on to a consumer that will.
type SampleFunc func(s *Sample) error

// Reference adds one reference to the sample's fragment, to hand the sample to one more consumer.
func (s *Sample) Reference() {
	if s != nil && s.frag != nil {
		s.frag.Reference()
	}
}

// ReferenceN adds n references to the sample's fragment.
func (s *Sample) ReferenceN(n int) {
	if s != nil && s.frag != nil {
		s.frag.ReferenceN(n)
	}
}

// Release drops one reference to the sample's fragment. The sample must not be used afterwards.
func (s *Sample) Release() {
	if s != nil && s.frag != nil {
		s.frag.Release()
	}
}

// ReleaseN drops n references to the sample's fragment.
func (s *Sample) ReleaseN(n int) {
	if s != nil && s.frag != nil {
		s.frag.ReleaseN(n)
	}
}

// Pooled reports whether the sample views a pooled fragment, i.e. whether its reference methods do anything.
func (s *Sample) Pooled() bool {
	return s != nil && s.frag != nil
}

// Clone returns a deep copy of the sample that is not pooled and can be retained without a reference, for tests and
// cold paths. Its NAL units are copies too and no longer alias Data.
func (s *Sample) Clone() *Sample {
	c := *s
	c.frag = nil
	c.Data = append([]byte(nil), s.Data...)
	if s.NALUs != nil {
		c.NALUs = make([][]byte, len(s.NALUs))
		for i, n := range s.NALUs {
			c.NALUs[i] = append([]byte(nil), n...)
		}
	}
	return &c
}

// ReadFragments reads one media segment (a sequence of moof/mdat fragments, optionally preceded by styp/sidx boxes)
// from r and calls fn once per sample, in decode order. It returns once r reaches EOF at a box boundary, or with the
// first error from the parser, the reader or fn. A blocking reader is fine: the parser reads sequentially and only
// ever needs the current fragment in memory, so a segment can be consumed while it is still being produced.
//
// Samples are views into fragments borrowed from pool, see Sample for the ownership rule; a nil pool selects a
// process-wide one. This is the convenience form for callers that read a segment now and then; a caller on a hot
// path holds a FragmentReader, which keeps its scratch buffers across segments.
func ReadFragments(r io.Reader, track *TrackInfo, pool *FragmentPool, fn SampleFunc) error {
	return NewFragmentReader(pool).ReadFragments(r, track, fn)
}
