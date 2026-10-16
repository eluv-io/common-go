package rtmp

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/internal/raceflag"
	"github.com/eluv-io/common-go/media/fmp4"
)

// captureConn is a net.Conn that keeps everything written to it.
type captureConn struct {
	discardConn
	buf bytes.Buffer
}

func (c *captureConn) Write(p []byte) (int, error) { return c.buf.Write(p) }

// fillHarness drives a Conn over a captureConn with a fake clock that follows the source's decode times, as a pacer
// releasing the samples in real time would.
type fillHarness struct {
	t      *testing.T
	c      *Conn
	out    *captureConn
	params fmp4.CodecParams
	video  []*fmp4.Sample
	audio  []*fmp4.Sample
	now    time.Time
	t0     time.Time
}

func newFillHarness(t *testing.T, cfg Config) *fillHarness {
	params, video, audio := fixtureTracks(t)
	h := &fillHarness{t: t, out: &captureConn{}, params: params, video: video, audio: audio, t0: time.Unix(1000, 0)}
	h.now = h.t0
	h.c = newConn(cfg, params, newSession(h.out, 0))
	h.c.now = func() time.Time { return h.now }
	require.NotNil(t, h.c.videoFill, "the fixture stream can be filled")
	return h
}

// dts and pts return sample i's times on the source axis.
func (h *fillHarness) dts(i int) time.Duration {
	return fmp4.TicksToDuration(h.video[i].DTS, h.video[i].Track.Timescale)
}

func (h *fillHarness) pts(i int) time.Duration {
	return fmp4.TicksToDuration(h.video[i].PTS, h.video[i].Track.Timescale)
}

// write sends sample i with its decode time moved by offset, at the wall-clock time that decode time is due.
func (h *fillHarness) write(i int, offset time.Duration) {
	h.t.Helper()
	h.now = h.t0.Add(h.dts(i) + offset - h.dts(0))
	s := h.video[i]
	require.NoError(h.t, h.c.WriteVideo(s.NALUs, h.dts(i)+offset, h.pts(i)+offset, s.Sync))
}

// sentVideo is one video message as the server receives it.
type sentVideo struct {
	ts       time.Duration
	cts      time.Duration
	keyframe bool
	nalus    [][]byte
}

func (h *fillHarness) sent() []sentVideo {
	h.t.Helper()
	var out []sentVideo
	for _, msg := range decodeMessages(h.t, h.out.buf.Bytes()) {
		if msg.Type != uint8(message.TypeVideo) {
			continue
		}
		au, keyframe, cts := videoOf(h.t, msg)
		nalus, err := avc.GetNalusFromSample(au)
		require.NoError(h.t, err)
		out = append(out, sentVideo{ts: msg.Timestamp, cts: time.Duration(cts) * time.Millisecond,
			keyframe: keyframe, nalus: nalus})
	}
	return out
}

// sliceHeaders parses the slice header of every sent access unit, in order.
func (h *fillHarness) sliceHeaders(sent []sentVideo) []*avc.SliceHeader {
	h.t.Helper()
	sps, err := avc.ParseSPSNALUnit(h.params.Video.SPS[0], true)
	require.NoError(h.t, err)
	spsMap := map[uint32]*avc.SPS{sps.ParameterID: sps}
	pps, err := avc.ParsePPSNALUnit(h.params.Video.PPS[0], spsMap)
	require.NoError(h.t, err)
	ppsMap := map[uint32]*avc.PPS{pps.PicParameterSetID: pps}
	var out []*avc.SliceHeader
	for _, v := range sent {
		var sh *avc.SliceHeader
		for _, nalu := range v.nalus {
			switch avc.GetNaluType(nalu[0]) {
			case avc.NALU_IDR, avc.NALU_NON_IDR:
				sh, err = avc.ParseSliceHeader(nalu, spsMap, ppsMap)
				require.NoError(h.t, err)
			}
		}
		require.NotNil(h.t, sh)
		out = append(out, sh)
	}
	return out
}

func TestConn_FillContinuesStarvedVideo(t *testing.T) {
	h := newFillHarness(t, Config{FillAfter: time.Second})
	const real = 30 // one second of the fixture, ending inside its first GOP
	for i := 0; i < real; i++ {
		h.write(i, 0)
	}
	lastAt := h.now
	fd := h.dts(real-1) - h.dts(real-2) // the frame duration the filler measured last

	// nothing until the video has been missing for FillAfter
	h.now = lastAt.Add(time.Second - time.Millisecond)
	require.NoError(t, h.c.Fill())
	require.Len(t, h.sent(), real)

	h.now = lastAt.Add(time.Second)
	require.NoError(t, h.c.Fill())
	afterFirst := len(h.sent()) - real
	require.Positive(t, afterFirst, "filling starts after FillAfter")
	h.now = lastAt.Add(2 * time.Second)
	require.NoError(t, h.c.Fill())
	sent := h.sent()
	filled := len(sent) - real
	require.InDelta(t, int(time.Second/fd), filled-afterFirst, 1, "one frame per frame duration")

	// The filled frames continue the real ones: they follow the highest presentation time, one frame duration
	// apart, and are due at the pace the source was sent at. Timestamps are as sent, relative to the first frame.
	var maxPTS time.Duration
	for i := 0; i < real; i++ {
		maxPTS = max(maxPTS, h.pts(i)-h.dts(0))
	}
	lastDTS := h.dts(real-1) - h.dts(0)
	for k, v := range sent[real:] {
		want := maxPTS + time.Duration(k+1)*fd
		require.Equal(t, millis(want), millis(v.ts), "filled frame %d", k)
		require.LessOrEqual(t, want-lastDTS, 2*time.Second, "filled frame %d is not ahead of the clock", k)
		require.False(t, v.keyframe)
		require.Zero(t, v.cts)
		require.Len(t, v.nalus, 1)
		require.EqualValues(t, avc.NALU_NON_IDR, avc.GetNaluType(v.nalus[0][0]))
		require.NotZero(t, v.nalus[0][0]&0x60, "a filled frame is a reference picture")
	}

	// frame_num continues from the last reference picture and the picture order count from the highest one
	headers := h.sliceHeaders(sent)
	var lastRefFrameNum, maxPOC uint32
	for i, sh := range headers[:real] {
		if sent[i].nalus[len(sent[i].nalus)-1][0]&0x60 != 0 {
			lastRefFrameNum = sh.FrameNum
		}
		if i == 0 || avc.GetNaluType(sent[i].nalus[len(sent[i].nalus)-1][0]) == avc.NALU_IDR {
			maxPOC = sh.PicOrderCntLsb
		}
		maxPOC = max(maxPOC, sh.PicOrderCntLsb)
	}
	sps, err := avc.ParseSPSNALUnit(h.params.Video.SPS[0], true)
	require.NoError(t, err)
	maxFrameNum := uint32(1) << (sps.Log2MaxFrameNumMinus4 + 4)
	maxPOCLsb := uint32(1) << (sps.Log2MaxPicOrderCntLsbMinus4 + 4)
	for k, sh := range headers[real:] {
		require.EqualValues(t, (lastRefFrameNum+uint32(k)+1)%maxFrameNum, sh.FrameNum, "filled frame %d", k)
		require.EqualValues(t, (maxPOC+2*uint32(k+1))%maxPOCLsb, sh.PicOrderCntLsb, "filled frame %d", k)
	}

	// The source returns where it stopped, now two seconds behind the filled frames: its frames are dropped up to the
	// next IDR, and the stream continues from that IDR right after the last filled frame.
	h.write(real, 0)
	require.Len(t, h.sent(), real+filled, "a frame that is not an IDR is dropped")
	const idr = 59
	h.write(idr, 0)
	h.write(idr+1, 0)
	sent = h.sent()
	require.Len(t, sent, real+filled+2)
	resumedTS := maxPTS + time.Duration(filled+1)*fd // the slot after the last filled frame
	resumed := sent[real+filled]
	require.True(t, resumed.keyframe)
	require.Equal(t, millis(resumedTS), millis(resumed.ts), "the IDR follows the last filled frame")
	require.Equal(t, millis(resumedTS+h.dts(idr+1)-h.dts(idr)), millis(sent[real+filled+1].ts))
	for i := 1; i < len(sent); i++ {
		require.GreaterOrEqual(t, sent[i].ts, sent[i-1].ts, "video timestamps never go back (message %d)", i)
	}

	cs := h.c.Stats()
	require.EqualValues(t, 1, cs.VideoFills)
	require.EqualValues(t, filled, cs.FilledVideoFrames)
	require.EqualValues(t, 1, cs.DroppedVideoWhileFilling)
	require.EqualValues(t, 1, cs.Rebases)
	require.EqualValues(t, real+filled+2, cs.VideoFrames)
	require.Zero(t, cs.ClampedTimestamps)

	// filling again after the next starvation counts a second fill
	h.now = h.now.Add(2 * time.Second)
	require.NoError(t, h.c.Fill())
	require.EqualValues(t, 2, h.c.Stats().VideoFills)
}

// TestConn_FillResumesAheadWithoutRebase covers a source whose timestamps moved on with the wall clock while it was
// starved, as a live source that dropped frames does: its IDR is ahead of the filled frames and keeps its timestamp.
func TestConn_FillResumesAheadWithoutRebase(t *testing.T) {
	h := newFillHarness(t, Config{FillAfter: 500 * time.Millisecond})
	for i := 0; i < 30; i++ {
		h.write(i, 0)
	}
	h.now = h.now.Add(time.Second)
	require.NoError(t, h.c.Fill())
	require.Positive(t, h.c.Stats().FilledVideoFrames)

	const idr = 59
	offset := 3 * time.Second // the source skipped ahead further than the filler got
	h.write(idr, offset)
	sent := h.sent()
	require.Equal(t, millis(h.dts(idr)+offset-h.dts(0)), millis(sent[len(sent)-1].ts))
	require.Zero(t, h.c.Stats().Rebases)
}

func TestConn_FillWaitsForTheStream(t *testing.T) {
	h := newFillHarness(t, Config{})
	h.now = h.now.Add(time.Hour)
	require.NoError(t, h.c.Fill())
	require.Empty(t, h.sent(), "nothing to continue before the first keyframe")

	nonKey := firstNonKeyframe(t, h.video)
	require.NoError(t, h.c.WriteVideo(nonKey.NALUs, 0, 0, false))
	require.NoError(t, h.c.Fill())
	require.Empty(t, h.sent(), "a dropped frame does not start the stream")

	h.write(0, 0)
	h.now = h.now.Add(DefaultFillAfter - time.Millisecond)
	require.NoError(t, h.c.Fill())
	require.Len(t, h.sent(), 1, "the default FillAfter has not passed")
	h.now = h.now.Add(time.Millisecond)
	require.NoError(t, h.c.Fill())
	require.Greater(t, len(h.sent()), 1)
}

// TestConn_FillAfterUnparsableSlice checks that a slice the filler cannot follow stops it from filling until the next
// IDR, rather than continuing a numbering it no longer knows.
func TestConn_FillAfterUnparsableSlice(t *testing.T) {
	h := newFillHarness(t, Config{})
	for i := 0; i < 10; i++ {
		h.write(i, 0)
	}
	// a P slice cut off inside its header
	broken := h.video[10].NALUs[len(h.video[10].NALUs)-1][:2]
	h.now = h.now.Add(h.dts(10) - h.dts(9))
	require.NoError(t, h.c.WriteVideo([][]byte{broken}, h.dts(10), h.pts(10), false))
	h.now = h.now.Add(2 * DefaultFillAfter)
	require.NoError(t, h.c.Fill())
	require.Zero(t, h.c.Stats().FilledVideoFrames)

	h.write(59, 0) // an IDR resynchronizes
	h.now = h.now.Add(2 * DefaultFillAfter)
	require.NoError(t, h.c.Fill())
	require.Positive(t, h.c.Stats().FilledVideoFrames)
}

// TestConn_FillKeepsWritesAllocationFree checks that following the stream for the filler does not cost the write path
// its allocation-free steady state.
func TestConn_FillKeepsWritesAllocationFree(t *testing.T) {
	if raceflag.Enabled {
		t.Skip("allocation counts do not hold under the race detector")
	}
	params, video, _ := fixtureTracks(t)
	c := newConn(Config{}, params, newSession(discardConn{}, 0))
	require.NotNil(t, c.videoFill)
	vt := video[0].Track
	span := fmp4.TicksToDuration(video[len(video)-1].DTS+int64(video[len(video)-1].Duration)-video[0].DTS, vt.Timescale)
	i, offset := 0, time.Duration(0)
	write := func() {
		s := video[i]
		dts := offset + fmp4.TicksToDuration(s.DTS, vt.Timescale)
		pts := offset + fmp4.TicksToDuration(s.PTS, vt.Timescale)
		require.NoError(t, c.WriteVideo(s.NALUs, dts, pts, s.Sync))
		if i++; i == len(video) {
			i, offset = 0, offset+span
		}
	}
	write() // the first keyframe starts the stream
	require.Zero(t, testing.AllocsPerRun(len(video)*2, write), "writing must not allocate")
}

// TestConn_FilledStreamDecodes hands the H.264 stream a server receives across a fill to ffmpeg, which must decode
// every frame without complaint: the filled frames continue the real ones and the IDR after them resets the decoder.
func TestConn_FilledStreamDecodes(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not found")
	}
	h := newFillHarness(t, Config{})
	for i := 0; i < 40; i++ {
		h.write(i, 0)
	}
	h.now = h.now.Add(3 * time.Second)
	require.NoError(t, h.c.Fill())
	for i := 59; i < len(h.video); i++ {
		h.write(i, 0)
	}
	sent := h.sent()

	var annexB []byte
	startCode := []byte{0, 0, 0, 1}
	for _, ps := range [][]byte{h.params.Video.SPS[0], h.params.Video.PPS[0]} {
		annexB = append(append(annexB, startCode...), ps...)
	}
	for _, v := range sent {
		for _, nalu := range v.nalus {
			annexB = append(append(annexB, startCode...), nalu...)
		}
	}
	path := filepath.Join(t.TempDir(), "filled.264")
	require.NoError(t, os.WriteFile(path, annexB, 0o644))

	// A splice ffmpeg cannot follow, such as resuming at a P frame, draws warnings like "co located POCs unavailable".
	out, err := exec.Command(ffmpeg, "-v", "warning", "-i", path, "-f", "null", "-").CombinedOutput()
	require.NoError(t, err, string(out))
	require.Empty(t, strings.TrimSpace(string(out)), "ffmpeg decodes without warnings")
	// ffprobe rather than an ffmpeg output: raw H.264 carries no timestamps, and ffmpeg drops frames whose made-up
	// ones collide on the way out
	out, err = exec.Command(ffprobe, "-v", "error", "-count_frames", "-select_streams", "v:0",
		"-show_entries", "stream=nb_read_frames", "-of", "csv=p=0", path).Output()
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(len(sent)), strings.TrimSpace(string(out)), "every access unit decodes to a frame")
	require.Positive(t, h.c.Stats().FilledVideoFrames)
}
