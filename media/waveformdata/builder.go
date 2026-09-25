package waveformdata

import (
	"github.com/eluv-io/errors-go"
)

// Builder assembles a waveform from batches of buckets delivered in ascending order of absolute bucket index. A
// bucket holds samplesPerPixel consecutive samples of the audio stream, and bucket i covers the samples from
// i*samplesPerPixel up to but excluding (i+1)*samplesPerPixel, counted from the start of the stream.
//
// Buckets are placed on that global grid, so a piece of audio that starts or ends mid-bucket (a mezzanine part or an
// ABR segment, which are cut on codec frames rather than on bucket boundaries) contributes a partial first or last
// bucket. Two partial buckets at the same index merge by min/max into the exact full bucket. Indices nothing was
// added for are filled with silence and counted in GapBuckets.
//
// Bucket values use the layout of Waveform.Data: for every bucket, and within the bucket for every channel, the
// minimum followed by the maximum sample value. A batch of n buckets for c channels is therefore n*c*2 values, with
// values[(bucket*c + channel)*2] the minimum and values[(bucket*c + channel)*2 + 1] the maximum.
type Builder struct {
	wf    *Waveform
	first int64 // absolute index of wf bucket 0, or -1 before the first Add
	gaps  int64
}

// NewBuilder returns an empty 16-bit builder for the given channel count, sample rate in Hz and bucket size in
// samples.
func NewBuilder(channels, sampleRate, samplesPerPixel int) *Builder {
	return &Builder{wf: New(channels, sampleRate, samplesPerPixel), first: -1}
}

// Add places a batch of whole buckets, in the layout described on Builder, starting at absolute bucket index
// firstBucket. Buckets that overlap already added ones merge by min/max. firstBucket may not precede the first
// bucket of the very first Add.
func (b *Builder) Add(firstBucket int64, values []int16) error {
	e := errors.Template("waveformdata.Builder.Add", errors.K.Invalid, "first_bucket", firstBucket)
	vpb := b.wf.valuesPerBucket()
	if len(values)%vpb != 0 {
		return e("reason", "values are not whole buckets", "len", len(values), "channels", b.wf.Channels)
	}
	if b.first < 0 {
		b.first = firstBucket
	}
	if firstBucket < b.first {
		return e("reason", "bucket precedes the builder start", "builder_first", b.first)
	}

	rel := firstBucket - b.first
	have := int64(b.wf.Length())
	if rel > have {
		b.gaps += rel - have
		b.wf.Data = append(b.wf.Data, make([]int16, (rel-have)*int64(vpb))...)
		have = rel
	}

	n := int64(len(values) / vpb)
	overlap := min(have-rel, n)
	for i := int64(0); i < overlap; i++ {
		out := b.wf.Data[(rel+i)*int64(vpb) : (rel+i+1)*int64(vpb)]
		mergeBucket(out, values[i*int64(vpb):(i+1)*int64(vpb)])
	}
	if overlap < n {
		b.wf.Data = append(b.wf.Data, values[overlap*int64(vpb):]...)
	}
	return nil
}

// FirstBucket returns the absolute index of the first bucket, or -1 if nothing was added.
func (b *Builder) FirstBucket() int64 {
	return b.first
}

// Length returns the number of buckets added so far, including gap buckets.
func (b *Builder) Length() int64 {
	return int64(b.wf.Length())
}

// GapBuckets returns the number of buckets filled with silence because nothing was added for them.
func (b *Builder) GapBuckets() int64 {
	return b.gaps
}

// Build returns the assembled waveform. The builder must not be used afterwards.
func (b *Builder) Build() *Waveform {
	if b.wf.Data == nil {
		b.wf.Data = []int16{}
	}
	return b.wf
}
