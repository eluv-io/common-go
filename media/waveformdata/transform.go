package waveformdata

import (
	"github.com/eluv-io/errors-go"
)

// Downsample returns a waveform with factor times fewer buckets, each holding the minimum of the minima and the
// maximum of the maxima of the source buckets it covers. Min/max compose exactly, so this is the same result as
// generating at the coarser resolution directly. A trailing group shorter than factor becomes a bucket of its own.
func (w *Waveform) Downsample(factor int) (*Waveform, error) {
	if factor <= 0 {
		return nil, errors.E("waveformdata.Downsample", errors.K.Invalid, "reason", "factor must be positive",
			"factor", factor)
	}
	if factor == 1 {
		return w.Clone(), nil
	}

	vpb := w.valuesPerBucket()
	srcLen := w.Length()
	dstLen := (srcLen + factor - 1) / factor
	res := w.header()
	res.SamplesPerPixel = w.SamplesPerPixel * factor
	res.Data = make([]int16, dstLen*vpb)

	for dst := 0; dst < dstLen; dst++ {
		first := dst * factor
		last := min(first+factor, srcLen)
		out := res.Data[dst*vpb : (dst+1)*vpb]
		copy(out, w.Data[first*vpb:(first+1)*vpb])
		for src := first + 1; src < last; src++ {
			in := w.Data[src*vpb : (src+1)*vpb]
			mergeBucket(out, in)
		}
	}
	return res, nil
}

// ToMono returns a single-channel waveform whose buckets hold the minimum and maximum across all channels.
func (w *Waveform) ToMono() *Waveform {
	if w.Channels == 1 {
		return w.Clone()
	}
	vpb := w.valuesPerBucket()
	n := w.Length()
	res := w.header()
	res.Channels = 1
	res.Data = make([]int16, n*2)
	for i := 0; i < n; i++ {
		in := w.Data[i*vpb : (i+1)*vpb]
		lo, hi := in[0], in[1]
		for ch := 1; ch < w.Channels; ch++ {
			lo = min(lo, in[2*ch])
			hi = max(hi, in[2*ch+1])
		}
		res.Data[2*i] = lo
		res.Data[2*i+1] = hi
	}
	return res
}

// To8Bit returns the waveform reduced to 8-bit values by dropping the low byte of each value.
func (w *Waveform) To8Bit() *Waveform {
	res := w.Clone()
	if w.Bits == Bits8 {
		return res
	}
	res.Bits = Bits8
	for i, v := range res.Data {
		res.Data[i] = v >> 8
	}
	return res
}

// Slice returns the buckets in [start, end), clamped to the available range.
func (w *Waveform) Slice(start, end int) *Waveform {
	n := w.Length()
	start = max(start, 0)
	end = min(end, n)
	res := w.header()
	if start >= end {
		res.Data = []int16{}
		return res
	}
	vpb := w.valuesPerBucket()
	res.Data = append([]int16(nil), w.Data[start*vpb:end*vpb]...)
	return res
}

// Concat appends the buckets of consecutive waveforms with identical header fields.
func Concat(parts ...*Waveform) (*Waveform, error) {
	e := errors.Template("waveformdata.Concat", errors.K.Invalid)
	if len(parts) == 0 {
		return nil, e("reason", "no parts")
	}
	res := parts[0].header()
	total := 0
	for i, p := range parts {
		if p.Channels != res.Channels || p.SampleRate != res.SampleRate ||
			p.SamplesPerPixel != res.SamplesPerPixel || p.Bits != res.Bits {
			return nil, e("reason", "header mismatch", "part", i)
		}
		total += len(p.Data)
	}
	res.Data = make([]int16, 0, total)
	for _, p := range parts {
		res.Data = append(res.Data, p.Data...)
	}
	return res, nil
}

// mergeBucket folds the min/max pairs of in into out.
func mergeBucket(out, in []int16) {
	for i := 0; i < len(out); i += 2 {
		out[i] = min(out[i], in[i])
		out[i+1] = max(out[i+1], in[i+1])
	}
}
