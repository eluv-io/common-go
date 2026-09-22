package fmp4

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTicksToDuration(t *testing.T) {
	require.Equal(t, time.Second, TicksToDuration(90000, 90000))
	require.Equal(t, time.Second, TicksToDuration(48000, 48000))
	require.Equal(t, 3*time.Second+time.Second/3, TicksToDuration(160000, 48000))
	require.Equal(t, time.Duration(0), TicksToDuration(1, 0))

	// five hours of 1024-sample AAC frames at 48 kHz land on the exact second boundary a float conversion would
	// drift away from
	ticks := int64(5 * 3600 * 48000)
	require.Equal(t, 5*time.Hour, TicksToDuration(ticks, 48000))
	// and a large 90 kHz value does not overflow the intermediate product
	require.Equal(t, 24*time.Hour, TicksToDuration(24*3600*90000, 90000))
}

func TestTimeline(t *testing.T) {
	video := &TrackInfo{Kind: KindVideo, Timescale: 90000}
	audio := &TrackInfo{Kind: KindAudio, Timescale: 48000}

	t.Run("first epoch keeps period-relative time and aligns both tracks", func(t *testing.T) {
		tl := NewTimeline()
		vd, vp := tl.Map(video, 90000, 93000)
		require.Equal(t, time.Second, vd)
		require.Equal(t, time.Second+time.Second/30, vp)
		ad, ap := tl.Map(audio, 47000, 47000)
		require.Equal(t, TicksToDuration(47000, 48000), ad)
		require.Equal(t, ad, ap)
		require.Equal(t, 0, tl.Epoch())
	})

	t.Run("new epoch continues after the last output and preserves the A/V offset", func(t *testing.T) {
		tl := NewTimeline(WithEpochGap(40 * time.Millisecond))
		tl.Map(video, 10*90000, 10*90000)         // video at 10 s
		tl.Map(audio, 10*48000+480, 10*48000+480) // audio 10 ms after
		tl.NewEpoch()
		require.Equal(t, 1, tl.Epoch())

		// the new period restarts at 2 s for video and 2.01 s for audio
		vd, _ := tl.Map(video, 2*90000, 2*90000)
		ad, _ := tl.Map(audio, 2*48000+480, 2*48000+480)
		require.Equal(t, 10*time.Second+10*time.Millisecond+40*time.Millisecond, vd, "one gap after the last output")
		require.Equal(t, vd+10*time.Millisecond, ad, "audio keeps its 10 ms offset to video")
	})

	t.Run("a backwards jump starts an epoch implicitly", func(t *testing.T) {
		tl := NewTimeline(WithEpochGap(40 * time.Millisecond))
		tl.Map(video, 5*90000, 5*90000)
		vd, _ := tl.Map(video, 90000, 90000) // 1 s: the encoder restarted
		require.Equal(t, 5*time.Second+40*time.Millisecond, vd)
		require.Equal(t, 1, tl.AutoJumps())
	})

	t.Run("a forward jump beyond the threshold starts an epoch, a smaller one does not", func(t *testing.T) {
		tl := NewTimeline(WithEpochGap(40*time.Millisecond), WithJumpThreshold(5*time.Second))
		tl.Map(video, 0, 0)
		vd, _ := tl.Map(video, 3*90000, 3*90000)
		require.Equal(t, 3*time.Second, vd)
		vd, _ = tl.Map(video, 60*90000, 60*90000)
		require.Equal(t, 3*time.Second+40*time.Millisecond, vd)
		require.Equal(t, 1, tl.AutoJumps())
	})

	t.Run("jump detection can be disabled", func(t *testing.T) {
		tl := NewTimeline(WithJumpThreshold(0))
		tl.Map(video, 5*90000, 5*90000)
		vd, _ := tl.Map(video, 90000, 90000)
		require.Equal(t, time.Second, vd)
		require.Equal(t, 0, tl.AutoJumps())
	})
}
