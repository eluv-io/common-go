package rtmp

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/internal/raceflag"
	"github.com/eluv-io/common-go/media/fmp4"
)

// apts returns audio frame j's time on the video's source axis, the two fixtures starting together. The audio fixture
// is shorter than the video one, so frame j repeats it as often as needed, back to back.
func (h *fillHarness) apts(j int) time.Duration {
	n := len(h.audio)
	ts := h.audio[0].Track.Timescale
	span := fmp4.TicksToDuration(h.audio[n-1].DTS+int64(h.audio[n-1].Duration)-h.audio[0].DTS, ts)
	return h.dts(0) + time.Duration(j/n)*span + fmp4.TicksToDuration(h.audio[j%n].DTS-h.audio[0].DTS, ts)
}

// audioAt returns the index of the first audio frame at or after t on the source axis.
func (h *fillHarness) audioAt(t time.Duration) int {
	j := 0
	for h.apts(j) < t {
		j++
	}
	return j
}

// release is one sample handed to the connection: video sample video, or audio frame audio when video is -1. It is
// released at its time on the source axis plus lag, with its timestamps moved by shift.
type release struct {
	video, audio int
	lag, shift   time.Duration
}

func (h *fillHarness) at(r release) time.Duration {
	if r.video >= 0 {
		return h.dts(r.video) + r.lag
	}
	return h.apts(r.audio) + r.lag
}

func (h *fillHarness) videoRun(from, to int, lag, shift time.Duration) []release {
	var out []release
	for i := from; i < to; i++ {
		out = append(out, release{video: i, audio: -1, lag: lag, shift: shift})
	}
	return out
}

func (h *fillHarness) audioRun(from, to int, lag, shift time.Duration) []release {
	var out []release
	for j := from; j < to; j++ {
		out = append(out, release{video: -1, audio: j, lag: lag, shift: shift})
	}
	return out
}

// play writes the releases in the order they are due, moving the clock to each, and calls Fill after every one, as
// the loop that waits for samples does on its ticker.
func (h *fillHarness) play(runs ...[]release) {
	h.t.Helper()
	var all []release
	for _, run := range runs {
		all = append(all, run...)
	}
	sort.SliceStable(all, func(a, b int) bool { return h.at(all[a]) < h.at(all[b]) })
	for _, r := range all {
		h.now = h.t0.Add(h.at(r) - h.dts(0))
		if r.video >= 0 {
			s := h.video[r.video]
			require.NoError(h.t, h.c.WriteVideo(s.NALUs, h.dts(r.video)+r.shift, h.pts(r.video)+r.shift, s.Sync))
		} else {
			require.NoError(h.t, h.c.WriteAudio(h.audio[r.audio%len(h.audio)].Data, h.apts(r.audio)+r.shift))
		}
		require.NoError(h.t, h.c.Fill())
	}
}

// sentMessage is one media message as the server receives it.
type sentMessage struct {
	video bool
	ts    time.Duration
	body  []byte // the raw AAC frame of an audio message
}

func (h *fillHarness) sentMessages() []sentMessage {
	h.t.Helper()
	var out []sentMessage
	for _, msg := range decodeMessages(h.t, h.out.buf.Bytes()) {
		switch msg.Type {
		case uint8(message.TypeVideo):
			out = append(out, sentMessage{video: true, ts: msg.Timestamp})
		case uint8(message.TypeAudio):
			out = append(out, sentMessage{ts: msg.Timestamp, body: audioOf(h.t, msg)})
		}
	}
	return out
}

func (h *fillHarness) sentAudio() []sentMessage {
	var out []sentMessage
	for _, m := range h.sentMessages() {
		if !m.video {
			out = append(out, m)
		}
	}
	return out
}

func isSilent(m sentMessage) bool {
	return bytes.Equal(m.body, silentAAC[2])
}

// requireMillis compares a timestamp as sent, in whole milliseconds, with an exact one.
func requireMillis(t *testing.T, want, got time.Duration, msgAndArgs ...any) {
	t.Helper()
	require.InDelta(t, millis(want), millis(got), 1, msgAndArgs...)
}

// TestConn_FillAudioWithSilence starves video and audio together and brings the source back where it stopped, as a
// stalled read does: silence continues the audio, goes on while the video waits for an IDR, and gives way to the
// source's audio once the IDR has moved the timeline, which keeps the two tracks as far apart as the source has them.
func TestConn_FillAudioWithSilence(t *testing.T) {
	h := newFillHarness(t, Config{})
	require.NotNil(t, h.c.audioFill, "the fixture audio can be filled")
	fd := h.c.audioFill.frameDur
	require.Equal(t, 1024*time.Second/48000, fd)

	const real = 30 // one second
	aReal := h.audioAt(h.dts(real))
	h.play(h.videoRun(0, real, 0, 0), h.audioRun(0, aReal, 0, 0))
	nm, na := len(h.sentMessages()), len(h.sentAudio())
	require.Equal(t, aReal, na)
	require.Zero(t, h.c.Stats().AudioFills, "no silence while the audio arrives")

	h.now = h.now.Add(2 * time.Second)
	require.NoError(t, h.c.Fill())
	silence := h.sentAudio()[na:]
	require.Greater(t, len(silence), int(time.Second/fd), "at least a second of silence")
	lastReal := h.apts(aReal-1) - h.dts(0) // as sent: the first video keyframe is time zero
	for k, m := range silence {
		require.True(t, isSilent(m), "silent frame %d", k)
		requireMillis(t, lastReal+time.Duration(k+1)*fd, m.ts, "silent frame %d continues the audio", k)
	}
	filled := h.sentMessages()[nm:]
	require.Greater(t, len(filled), len(silence), "video is filled too")
	for i := 1; i < len(filled); i++ {
		require.GreaterOrEqual(t, filled[i].ts, filled[i-1].ts, "filled frames go out in timestamp order (%d)", i)
	}

	// The source returns where it stopped, two seconds behind. Its audio is dropped while the video waits for an IDR,
	// and the silence goes on meanwhile.
	const idr = 59
	lag := h.now.Sub(h.t0) - (h.dts(real) - h.dts(0))
	end := h.audioAt(h.dts(idr + 30))
	h.play(h.videoRun(real, idr+30, lag, 0), h.audioRun(aReal, end, lag, 0))
	cs := h.c.Stats()
	require.EqualValues(t, 1, cs.AudioFills)
	require.EqualValues(t, 1, cs.Rebases)
	require.Positive(t, cs.DroppedAudioWhileFilling)
	require.Zero(t, cs.ClampedTimestamps)
	require.EqualValues(t, na+int(cs.FilledAudioFrames)+(end-aReal)-int(cs.DroppedAudioWhileFilling),
		len(h.sentAudio()), "every audio frame is sent, dropped or silence")

	audio := h.sentAudio()
	firstResumed := -1
	for i := na; i < len(audio); i++ {
		if i > 0 {
			require.GreaterOrEqual(t, audio[i].ts, audio[i-1].ts, "audio timestamps never go back (%d)", i)
		}
		if firstResumed < 0 && !isSilent(audio[i]) {
			firstResumed = i
		}
	}
	require.Positive(t, firstResumed, "the source's audio resumes")
	lastSilent := audio[firstResumed-1]
	require.True(t, isSilent(lastSilent))
	var idrTS time.Duration // as sent: the last keyframe is the IDR the video resumed at
	for _, v := range h.sent() {
		if v.keyframe {
			idrTS = v.ts
		}
	}
	require.GreaterOrEqual(t, lastSilent.ts+fd, idrTS, "the silence went on while the video waited for the IDR")
	require.GreaterOrEqual(t, audio[firstResumed].ts+time.Millisecond, lastSilent.ts+fd/2,
		"the source's audio overlaps the silence by at most half a frame")
	vfd := h.dts(1) - h.dts(0)
	require.Less(t, audio[firstResumed].ts, lastSilent.ts+2*fd+vfd,
		"the source's audio follows the silence within the frames that video and audio are apart")
	for _, m := range audio[firstResumed:] {
		require.False(t, isSilent(m), "no silence once the source has taken over")
	}

	// After the shift both tracks keep the distance the source has between them.
	last := h.sentAudio()[len(h.sentAudio())-1]
	var lastVideo time.Duration
	for _, m := range h.sentMessages() {
		if m.video {
			lastVideo = m.ts
		}
	}
	requireMillis(t, h.apts(end-1)-h.dts(idr+29), last.ts-lastVideo, "audio stays in sync with video")
}

// TestConn_FillAudioResumes starves the audio alone and brings it back behind the silence, where a stalled audio read
// resumes, or level with it, as a live source that lost frames does.
func TestConn_FillAudioResumes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		behind bool
	}{
		{"behind", true},
		{"level", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFillHarness(t, Config{FillAfter: 300 * time.Millisecond})
			fd := h.c.audioFill.frameDur
			stop := h.audioAt(h.dts(30))
			gap := h.dts(60) - h.dts(30) // the audio comes back a second later
			shift := gap                 // its timestamps moved on with the clock
			if tc.behind {
				shift = 0 // it continues where it stopped
			}
			end := h.audioAt(h.dts(len(h.video)-1) - gap)
			h.play(h.videoRun(0, len(h.video), 0, 0), h.audioRun(0, stop, 0, 0), h.audioRun(stop, end, gap, shift))

			cs := h.c.Stats()
			require.Zero(t, cs.VideoFills)
			require.EqualValues(t, 1, cs.AudioFills)
			require.Zero(t, cs.Rebases)
			require.Zero(t, cs.ClampedTimestamps)
			audio := h.sentAudio()
			silent := 0
			for _, m := range audio {
				if isSilent(m) {
					silent++
				}
			}
			require.EqualValues(t, silent, cs.FilledAudioFrames)
			require.InDelta(t, int(gap/fd), silent, 2, "silence covers the time until the audio returns")
			if tc.behind {
				require.InDelta(t, silent, int(cs.DroppedAudioWhileFilling), 1,
					"the source catches up with the silence, which stopped when it returned")
			} else {
				require.Zero(t, cs.DroppedAudioWhileFilling)
			}

			// silence only between the last frame before the stop and the first one taken over, in timestamp order
			for i := 1; i < len(audio); i++ {
				require.GreaterOrEqual(t, audio[i].ts, audio[i-1].ts, "audio timestamps never go back (%d)", i)
			}
			require.False(t, isSilent(audio[stop-1]))
			for _, m := range audio[stop : stop+silent] {
				require.True(t, isSilent(m))
			}
			resumed := audio[stop+silent]
			require.GreaterOrEqual(t, resumed.ts+time.Millisecond, audio[stop+silent-1].ts+fd/2)
			require.Less(t, resumed.ts, audio[stop+silent-1].ts+2*fd, "the source takes over where the silence ends")

			// the source's own timestamps, so the audio is in sync with the video again
			j := stop + int(cs.DroppedAudioWhileFilling)
			requireMillis(t, h.apts(j)+shift-h.dts(0), resumed.ts)
		})
	}
}

func TestNewAudioFiller(t *testing.T) {
	for _, tc := range []struct {
		name  string
		asc   []byte
		frame []byte
		dur   time.Duration
	}{
		{"AAC-LC 48 kHz stereo", []byte{0x11, 0x90}, silentAAC[2], 1024 * time.Second / 48000},
		{"AAC-LC 44.1 kHz mono", []byte{0x12, 0x08}, silentAAC[1], 1024 * time.Second / 44100},
		{"AAC-LC 48 kHz 5.1", []byte{0x11, 0xb0}, nil, 0},
		{"AAC-LC 960-sample frames", []byte{0x11, 0x94}, nil, 0},
		{"HE-AAC", []byte{0x2b, 0x11, 0x88}, nil, 0},
		{"no AudioSpecificConfig", nil, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := newAudioFiller(&fmp4.AudioParams{ASC: tc.asc}, time.Second)
			if tc.frame == nil {
				require.Error(t, err)
				require.Nil(t, f)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.frame, f.frame)
			require.Equal(t, tc.dur, f.frameDur)
		})
	}
}

// TestConn_FillAudioAllocationFree checks that the write path stays allocation-free with the audio filler following
// it, and that filling audio does not allocate either: the silent frame is a constant.
func TestConn_FillAudioAllocationFree(t *testing.T) {
	if raceflag.Enabled {
		t.Skip("allocation counts do not hold under the race detector")
	}
	h := newFillHarness(t, Config{})
	h.c.s = newSession(discardConn{}, 0)
	h.play(h.videoRun(0, 1, 0, 0))
	j := 0
	write := func() {
		h.now = h.t0.Add(h.apts(j) - h.dts(0))
		require.NoError(t, h.c.WriteAudio(h.audio[j%len(h.audio)].Data, h.apts(j)))
		j++
	}
	require.Zero(t, testing.AllocsPerRun(100, write), "writing audio must not allocate")

	h.c.videoFill = nil // P_Skip frames cost hi264 a few allocations; this is about the silence
	h.now = h.now.Add(2 * time.Second)
	require.NoError(t, h.c.Fill())
	require.EqualValues(t, 1, h.c.Stats().AudioFills)
	fill := func() {
		h.now = h.now.Add(h.c.audioFill.frameDur)
		require.NoError(t, h.c.Fill())
	}
	before := h.c.Stats().FilledAudioFrames
	require.Zero(t, testing.AllocsPerRun(100, fill), "filling audio must not allocate")
	require.GreaterOrEqual(t, h.c.Stats().FilledAudioFrames, before+100)
}

// TestConn_FilledAudioDecodes hands the AAC stream a server receives across a fill to ffmpeg, which must decode every
// frame, the silent ones included, without complaint.
func TestConn_FilledAudioDecodes(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found")
	}
	h := newFillHarness(t, Config{})
	aReal := h.audioAt(h.dts(30))
	h.play(h.videoRun(0, 30, 0, 0), h.audioRun(0, aReal, 0, 0))
	h.now = h.now.Add(2 * time.Second)
	require.NoError(t, h.c.Fill())
	lag := h.now.Sub(h.t0) - (h.dts(30) - h.dts(0))
	h.play(h.videoRun(30, len(h.video), lag, 0), h.audioRun(aReal, h.audioAt(h.dts(len(h.video)-1)), lag, 0))
	audio := h.sentAudio()
	require.Positive(t, h.c.Stats().FilledAudioFrames)

	// ADTS for AAC-LC (profile 1), 48 kHz (index 3), stereo, as the fixture's AudioSpecificConfig says; the rest of it
	// signals that there is no SBR
	require.Equal(t, []byte{0x11, 0x90}, h.params.Audio.ASC[:2])
	var adts []byte
	for _, m := range audio {
		n := 7 + len(m.body)
		adts = append(adts, 0xff, 0xf1, 1<<6|3<<2, 2<<6|byte(n>>11), byte(n>>3), byte(n&7)<<5|0x1f, 0xfc)
		adts = append(adts, m.body...)
	}
	path := filepath.Join(t.TempDir(), "filled.aac")
	require.NoError(t, os.WriteFile(path, adts, 0o644))

	out, err := exec.Command(ffmpeg, "-v", "warning", "-i", path, "-f", "null", "-").CombinedOutput()
	require.NoError(t, err, string(out))
	var warnings []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// the ADTS demuxer's note on a file without a duration, not a decoding problem
		if line != "" && !strings.Contains(line, "Estimating duration from bitrate") {
			warnings = append(warnings, line)
		}
	}
	require.Empty(t, warnings, "ffmpeg decodes without warnings")
	out, err = exec.Command(ffprobe, "-v", "error", "-count_frames", "-select_streams", "a:0",
		"-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", path).Output()
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(len(audio)), strings.TrimSpace(string(out)), "every frame decodes")
}

// TestSilentAACDecodesToSilence decodes each silent frame with ffmpeg at several sample rates: every frame must
// decode, to digital silence.
func TestSilentAACDecodesToSilence(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	const frames = 50
	for channels, frame := range silentAAC {
		if frame == nil {
			continue
		}
		for _, rate := range []struct {
			hz    int
			index byte // ADTS sampling_frequency_index
		}{{48000, 3}, {44100, 4}, {16000, 8}} {
			t.Run(strconv.Itoa(channels)+"ch-"+strconv.Itoa(rate.hz), func(t *testing.T) {
				var adts []byte
				n := 7 + len(frame)
				for range frames {
					adts = append(adts, 0xff, 0xf1, 1<<6|rate.index<<2|byte(channels>>2),
						byte(channels&3)<<6|byte(n>>11), byte(n>>3), byte(n&7)<<5|0x1f, 0xfc)
					adts = append(adts, frame...)
				}
				path := filepath.Join(t.TempDir(), "silence.aac")
				require.NoError(t, os.WriteFile(path, adts, 0o644))
				out, err := exec.Command(ffmpeg, "-hide_banner", "-i", path, "-af", "volumedetect", "-f", "null",
					"-").CombinedOutput()
				require.NoError(t, err, string(out))
				require.Contains(t, string(out), "n_samples: "+strconv.Itoa(frames*aacFrameSamples*channels))
				require.Contains(t, string(out), "max_volume: -91.0 dB", "digital silence in 16 bits")
			})
		}
	}
}
