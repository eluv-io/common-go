package waveformdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// stereo returns a 2-channel waveform with n buckets whose values identify bucket and channel: channel 0 holds
// (-i, i) and channel 1 holds (-10*i, 10*i) for bucket i.
func stereo(n int) *Waveform {
	w := New(2, 48000, 2400)
	for i := 0; i < n; i++ {
		v := int16(i)
		w.Data = append(w.Data, -v, v, -10*v, 10*v)
	}
	return w
}

func TestDownsample(t *testing.T) {
	w := stereo(5)

	d, err := w.Downsample(2)
	require.NoError(t, err)
	require.Equal(t, 4800, d.SamplesPerPixel)
	require.Equal(t, 3, d.Length())
	require.Equal(t, []int16{
		-1, 1, -10, 10, // buckets 0,1
		-3, 3, -30, 30, // buckets 2,3
		-4, 4, -40, 40, // bucket 4 alone
	}, d.Data)

	d, err = w.Downsample(10)
	require.NoError(t, err)
	require.Equal(t, 1, d.Length())
	require.Equal(t, []int16{-4, 4, -40, 40}, d.Data)

	same, err := w.Downsample(1)
	require.NoError(t, err)
	require.Equal(t, w, same)

	_, err = w.Downsample(0)
	require.Error(t, err)

	// downsampling is exact: by 6 equals by 2 then by 3
	w = stereo(100)
	a, err := w.Downsample(6)
	require.NoError(t, err)
	b, err := w.Downsample(2)
	require.NoError(t, err)
	b, err = b.Downsample(3)
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestToMono(t *testing.T) {
	m := stereo(3).ToMono()
	require.Equal(t, 1, m.Channels)
	require.Equal(t, []int16{0, 0, -10, 10, -20, 20}, m.Data)
	require.Equal(t, m, m.ToMono())
}

func TestTo8Bit(t *testing.T) {
	w := New(1, 48000, 2400)
	w.Data = []int16{-32768, 32767, -256, 255, -1, 0}
	b := w.To8Bit()
	require.Equal(t, Bits8, b.Bits)
	require.Equal(t, []int16{-128, 127, -1, 0, -1, 0}, b.Data)
	require.Equal(t, Bits16, w.Bits, "source unchanged")
	require.Equal(t, b, b.To8Bit())
}

func TestSlice(t *testing.T) {
	w := stereo(5)
	s := w.Slice(1, 3)
	require.Equal(t, 2, s.Length())
	require.Equal(t, []int16{-1, 1, -10, 10, -2, 2, -20, 20}, s.Data)

	require.Equal(t, 5, w.Slice(-3, 100).Length())
	require.Equal(t, 0, w.Slice(3, 3).Length())
	require.Equal(t, 0, w.Slice(4, 2).Length())
	require.NotNil(t, w.Slice(4, 2).Data)
}

func TestConcat(t *testing.T) {
	w := stereo(5)
	c, err := Concat(w.Slice(0, 2), w.Slice(2, 5))
	require.NoError(t, err)
	require.Equal(t, w, c)

	other := stereo(1)
	other.SampleRate = 44100
	_, err = Concat(w, other)
	require.Error(t, err)

	_, err = Concat()
	require.Error(t, err)
}
