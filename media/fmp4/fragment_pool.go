package fmp4

import (
	"github.com/eluv-io/common-go/collections/pool"
	"github.com/eluv-io/errors-go"
)

// DefaultMaxRetainedBytes is the mdat buffer capacity a pooled Fragment keeps across uses unless configured
// otherwise. A fragment whose buffer grew beyond it drops the buffer when it returns to the pool.
const DefaultMaxRetainedBytes = 4 << 20

// FragmentRef is a reference-counted Fragment borrowed from a FragmentPool.
type FragmentRef = *pool.Resource[*Fragment]

// FragmentPool is a pool of Fragments. Borrow one with BorrowFragment.
type FragmentPool = pool.Pool[*Fragment]

// FragmentPoolConfig configures a FragmentPool.
type FragmentPoolConfig struct {
	// MaxRetainedBytes is the largest mdat buffer a fragment keeps when it returns to the pool. 0 selects
	// DefaultMaxRetainedBytes.
	MaxRetainedBytes int
}

// NewFragmentPool creates a pool of Fragments.
func NewFragmentPool(cfg FragmentPoolConfig) *FragmentPool {
	if cfg.MaxRetainedBytes <= 0 {
		cfg.MaxRetainedBytes = DefaultMaxRetainedBytes
	}
	return pool.New(fragmentFactory{maxRetained: cfg.MaxRetainedBytes})
}

// BorrowFragment borrows an empty fragment for the given track. The caller owns one reference; see Sample for how
// references travel with the samples the fragment is filled with.
func BorrowFragment(p *FragmentPool, track *TrackInfo) FragmentRef {
	ref := p.Borrow()
	ref.T.Reset(ref, track)
	return ref
}

type fragmentFactory struct {
	maxRetained int
}

func (f fragmentFactory) New() *Fragment { return &Fragment{} }

func (f fragmentFactory) Init(fr *Fragment) { fr.Reset(nil, nil) }

func (f fragmentFactory) Reset(fr *Fragment) {
	fr.Reset(nil, nil)
	if cap(fr.buf) > f.maxRetained {
		fr.buf = nil
	}
}

// Fragment is the payload of one moof/mdat fragment and the samples that view it. Fragments come from a
// FragmentPool and are reference counted: every Sample handed out holds one reference to its fragment, and the
// buffer is recycled when the last one is released.
//
// Memory model: a pooled buffer grows to the largest mdat it ever held, typically a keyframe, and keeps that capacity
// until it exceeds the pool's MaxRetainedBytes. What a pool pins is therefore the number of fragments in flight times
// the keyframe size, not the sum of the sample sizes. With a jitter buffer of a few seconds and one bounded queue per
// output that is tens of megabytes per input at most.
type Fragment struct {
	Samples []Sample // the fragment's samples in decode order; every one views buf

	track *TrackInfo
	ref   FragmentRef // the resource wrapping this fragment, handed to every sample
	buf   []byte      // the mdat payload; only its length changes, it is never reassigned to a subslice
	nalus [][]byte    // arena for the NALU views of all samples; each sample gets a capped subslice of it
}

// Reset empties the fragment for a new fill on behalf of track. ref is the resource wrapping the fragment; the
// samples appended afterwards carry it.
func (f *Fragment) Reset(ref FragmentRef, track *TrackInfo) {
	f.track = track
	f.ref = ref
	clear(f.Samples)
	f.Samples = f.Samples[:0]
	clear(f.nalus)
	f.nalus = f.nalus[:0]
	f.buf = f.buf[:0]
}

// Grow sizes the payload buffer to n bytes, reusing its capacity, and returns it for the caller to fill.
func (f *Fragment) Grow(n int) []byte {
	if cap(f.buf) < n {
		f.buf = make([]byte, n)
	} else {
		f.buf = f.buf[:n]
	}
	return f.buf
}

// Payload returns the current payload buffer, as returned by the last Grow.
func (f *Fragment) Payload() []byte {
	return f.buf
}

// AppendSample adds a sample viewing data, a subslice of the payload buffer, with its timing in the track's
// timescale (edit list already removed) and the sync flag as signalled by the container. For an H.264 track the NAL
// units are split into the fragment's arena, access unit delimiters and filler dropped, and an IDR picture also
// marks the sample as sync. The returned pointer is stable while the fragment is referenced.
func (f *Fragment) AppendSample(data []byte, dts, pts int64, duration uint32, sync bool) (*Sample, error) {
	f.Samples = append(f.Samples, Sample{
		Track:    f.track,
		DTS:      dts,
		PTS:      pts,
		Duration: duration,
		Sync:     sync,
		Data:     data,
		frag:     f.ref,
	})
	s := &f.Samples[len(f.Samples)-1]
	if f.track == nil || f.track.Kind != KindVideo {
		s.Sync = true
		return s, nil
	}
	start := len(f.nalus)
	var err error
	f.nalus, err = appendNALUs(f.nalus, data, f.track.NALULengthSize)
	if err != nil {
		f.nalus = f.nalus[:start]
		f.Samples = f.Samples[:len(f.Samples)-1]
		return nil, errors.E("fmp4.AppendSample", errors.K.Invalid, err, "track_id", f.track.TrackID, "dts", dts)
	}
	kept, idr, paramSetChange := inspectAccessUnit(f.nalus[start:], f.track.SPS, f.track.PPS)
	end := start + len(kept)
	f.nalus = f.nalus[:end]
	s.NALUs = f.nalus[start:end:end]
	s.Sync = sync || idr
	s.ParamSetChange = paramSetChange
	return s, nil
}
