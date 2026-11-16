// Package waveformdata implements the BBC audiowaveform "waveform-data" format: for each bucket of SamplesPerPixel
// consecutive audio samples, the minimum and maximum sample value of each channel. The binary form (.dat, versions 1
// and 2) and the JSON form are both supported, as are the transformations a viewer needs: downsampling to a coarser
// zoom level, merging channels, reducing to 8 bits, slicing a time range and concatenating consecutive pieces.
//
// The format is consumed by peaks.js and waveform-data.js. See https://github.com/bbc/audiowaveform for the
// specification.
package waveformdata

import (
	"time"

	"github.com/eluv-io/errors-go"
)

const (
	// Version is the file format version written by Encode. Decode accepts versions 1 and 2.
	Version = 2

	// Bits16 and Bits8 are the supported sample value bit depths.
	Bits16 = 16
	Bits8  = 8
)

// Waveform is a decoded waveform-data set: the audio stream cut into buckets of SamplesPerPixel consecutive samples,
// with the minimum and maximum sample value of every channel kept per bucket.
//
// Data holds Length()*Channels*2 values: for every bucket, and within the bucket for every channel, the minimum
// followed by the maximum. Data[(bucket*Channels + channel)*2] is the minimum and the next value the maximum. Values
// are always stored as int16, in the range of the declared bit depth: [-32768, 32767] for 16 bits and [-128, 127]
// for 8 bits.
type Waveform struct {
	Version         int
	Channels        int
	SampleRate      int // samples per second
	SamplesPerPixel int // samples per bucket, the "pixel" of the format's name being one bucket drawn one pixel wide
	Bits            int
	Data            []int16
}

// New returns an empty 16-bit waveform with the given parameters.
func New(channels, sampleRate, samplesPerPixel int) *Waveform {
	return &Waveform{
		Version:         Version,
		Channels:        channels,
		SampleRate:      sampleRate,
		SamplesPerPixel: samplesPerPixel,
		Bits:            Bits16,
	}
}

// Length returns the number of buckets.
func (w *Waveform) Length() int {
	if w.Channels <= 0 {
		return 0
	}
	return len(w.Data) / w.valuesPerBucket()
}

// Duration returns the audio duration covered by the buckets.
func (w *Waveform) Duration() time.Duration {
	if w.SampleRate <= 0 {
		return 0
	}
	samples := int64(w.Length()) * int64(w.SamplesPerPixel)
	return time.Duration(samples * int64(time.Second) / int64(w.SampleRate))
}

// BucketDuration returns the duration of one bucket.
func (w *Waveform) BucketDuration() time.Duration {
	if w.SampleRate <= 0 {
		return 0
	}
	return time.Duration(int64(w.SamplesPerPixel) * int64(time.Second) / int64(w.SampleRate))
}

// Validate checks the header fields and the data length.
func (w *Waveform) Validate() error {
	e := errors.Template("waveformdata.Validate", errors.K.Invalid)
	switch {
	case w.Version != 1 && w.Version != 2:
		return e("reason", "unsupported version", "version", w.Version)
	case w.Channels <= 0:
		return e("reason", "invalid channel count", "channels", w.Channels)
	case w.SampleRate <= 0:
		return e("reason", "invalid sample rate", "sample_rate", w.SampleRate)
	case w.SamplesPerPixel <= 0:
		return e("reason", "invalid samples per pixel", "samples_per_pixel", w.SamplesPerPixel)
	case w.Bits != Bits8 && w.Bits != Bits16:
		return e("reason", "unsupported bit depth", "bits", w.Bits)
	case len(w.Data)%w.valuesPerBucket() != 0:
		return e("reason", "data length is not a multiple of channels*2", "len", len(w.Data), "channels", w.Channels)
	}
	return nil
}

// Clone returns a deep copy.
func (w *Waveform) Clone() *Waveform {
	c := *w
	c.Data = append([]int16(nil), w.Data...)
	return &c
}

// header returns a copy of the header fields with empty data, for transformations that produce a new waveform.
func (w *Waveform) header() *Waveform {
	c := *w
	c.Data = nil
	return &c
}

func (w *Waveform) valuesPerBucket() int {
	return w.Channels * 2
}
