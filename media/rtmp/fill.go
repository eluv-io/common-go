package rtmp

import (
	"time"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/mp4ff/avc"

	"github.com/eluv-io/common-go/media/fmp4"
	"github.com/eluv-io/errors-go"
)

// defaultFrameDuration spaces the filler frames when neither the source nor the configuration gives a frame rate:
// before two real frames have been written, the decode time between them is not known yet.
const defaultFrameDuration = time.Second / 30

// videoFiller continues a starved video track with P_Skip frames: frames that repeat the previous picture and cost a
// few dozen bytes each. hi264 builds them from the stream's own SPS and PPS, so the server sees no configuration
// change, and their frame_num and picture order count continue those of the real frames written before them.
//
// The source can only take over again at an IDR, because the inserted frames advanced the numbering past what its
// encoder emits next. Until one arrives, WriteVideo drops the source's frames and Fill keeps filling.
type videoFiller struct {
	ext   *encode.PSkipExtender
	after time.Duration

	synced   bool          // every frame since the stream start or the last IDR parsed, so the numbering is known
	have     bool          // a real frame has been written
	lastDTS  time.Duration // decode time of the last real frame, as sent
	maxPTS   time.Duration // highest presentation time of a real frame, as sent
	lastAt   time.Time     // when the last real frame was written
	frameDur time.Duration // decode time between consecutive real frames

	filling bool          // filler frames were written and the source has not resumed at an IDR yet
	stopped bool          // hi264 could not build a frame for this stream; the connection fills no more
	nextTS  time.Duration // timestamp of the next frame Fill writes
	nalus   [1][]byte     // the NAL unit of the frame being written, so writing it does not allocate a slice
}

// newVideoFiller prepares a filler for the video track. It fails for a stream hi264 cannot continue, such as one with
// pic_order_cnt_type 1 or interlaced coding.
func newVideoFiller(v *fmp4.VideoParams, after time.Duration, frameRate float64) (*videoFiller, error) {
	e := errors.Template("rtmp.newVideoFiller", errors.K.Invalid)
	if v == nil || len(v.SPS) == 0 || len(v.PPS) == 0 {
		return nil, e("reason", "no SPS or PPS")
	}
	spsMap := make(map[uint32]*avc.SPS, len(v.SPS))
	for _, nalu := range v.SPS {
		sps, err := avc.ParseSPSNALUnit(nalu, true)
		if err != nil {
			return nil, e(err, "reason", "invalid SPS")
		}
		spsMap[sps.ParameterID] = sps
	}
	pps, err := avc.ParsePPSNALUnit(v.PPS[0], spsMap)
	if err != nil {
		return nil, e(err, "reason", "invalid PPS")
	}
	ext, err := encode.NewPSkipExtender(spsMap[pps.SeqParameterSetID], pps)
	if err != nil {
		return nil, e(err)
	}
	// the other parameter sets of the track, for slices that refer to them
	for _, nalus := range [][][]byte{v.SPS, v.PPS[1:]} {
		for _, nalu := range nalus {
			if err = ext.ObserveNALU(nalu); err != nil {
				return nil, e(err, "reason", "invalid parameter set")
			}
		}
	}
	f := &videoFiller{ext: ext, after: after, frameDur: defaultFrameDuration}
	if frameRate > 0 {
		f.frameDur = time.Duration(float64(time.Second) / frameRate)
	}
	return f, nil
}

// observe records a real frame that is being written. Its NAL units advance the numbering the filler continues from;
// none of this allocates, so writing video stays allocation-free with the filler on.
func (f *videoFiller) observe(nalus [][]byte, dts, pts time.Duration, idr bool, now time.Time) {
	if idr || !f.have {
		f.synced = true
	}
	for _, nalu := range nalus {
		if err := f.ext.ObserveNALU(nalu); err != nil {
			// a slice that does not parse leaves the numbering unknown until an IDR resets it
			f.synced = false
		}
	}
	if !f.have || pts > f.maxPTS {
		f.maxPTS = pts
	}
	// A gap longer than the filler waits is a jump in the source's timeline, not its frame duration.
	if d := dts - f.lastDTS; f.have && d > 0 && d < f.after {
		f.frameDur = d
	}
	f.have = true
	f.lastDTS = dts
	f.lastAt = now
}

// nextDue returns the timestamp of the next filler frame if one is due at now, starting to fill once no real frame
// has been written for the configured time. A frame is due when as much time has passed since the last real frame
// was written as lies between their timestamps, so the filler continues at the pace the source was sent at. The
// first one follows the highest presentation time rather than the last decode time: with B frames the two differ by
// the reorder delay, and a frame placed between them would be displayed out of order.
func (f *videoFiller) nextDue(now time.Time) (time.Duration, bool) {
	next := f.nextTS
	if f.stopped {
		return 0, false
	}
	if !f.filling {
		if !f.have || !f.synced || now.Sub(f.lastAt) < f.after {
			return 0, false
		}
		next = max(f.maxPTS, f.lastDTS) + f.frameDur
	}
	if f.lastAt.Add(next - f.lastDTS).After(now) {
		return 0, false
	}
	f.filling = true
	f.nextTS = next + f.frameDur
	return next, true
}

// Fill keeps the video going while the source is starved, so the server goes on receiving a live stream instead of
// timing it out. Once no video has been written for Config.FillAfter, each call writes the frames that are due by
// now: P_Skip frames that repeat the last picture, spaced at the source's frame duration and paced as the source was.
// When the source returns, its frames are dropped until an IDR, which the stream continues from; if its timestamps
// fell behind the inserted frames meanwhile, everything written from then on is shifted to follow them.
//
// Audio is not filled: while video is filled, whatever audio the caller writes goes out as it is.
//
// Fill is a write: call it from the goroutine that calls WriteVideo, e.g. on a ticker in the loop that waits for
// samples. It does nothing before the first keyframe has been written, while video arrives, or for a stream the
// filler cannot continue.
func (c *Conn) Fill() error {
	if err := c.Err(); err != nil {
		return err
	}
	for {
		c.mu.Lock()
		f := c.videoFill
		if f == nil || !c.started {
			c.mu.Unlock()
			return nil
		}
		starting := !f.filling
		ts, due := f.nextDue(c.now())
		if !due {
			c.mu.Unlock()
			return nil
		}
		if starting {
			c.stats.VideoFills++
			log.Info("rtmp video starved, filling", "destination", c.cfg.Redacted(), "after", f.after)
		}
		slice, err := f.ext.NextSlice()
		if err != nil {
			// The source switched to parameter sets the filler cannot continue. Stop filling on this connection rather
			// than failing it, since the stream itself is fine. Frames already filled still make the source resume at
			// an IDR.
			f.stopped = true
			f.filling = !starting
			c.mu.Unlock()
			log.Warn("rtmp filler stopped", err, "destination", c.cfg.Redacted())
			return nil
		}
		f.nalus[0] = stripStartCode(slice)
		c.videoClock.ts, c.videoClock.set = ts, true
		c.stats.VideoFrames++
		c.stats.FilledVideoFrames++
		c.stats.LastVideoTS = ts
		c.mu.Unlock()

		if err = c.writeResult(c.s.writeVideo(f.nalus[:], millis(ts), 0, false)); err != nil {
			return err
		}
	}
}

// resumeLocked ends filling for an IDR from the source with decode time dts on the caller's axis. If the source's
// timestamps fell behind the inserted frames, the origin moves so the IDR follows the last of them; audio shares the
// origin and moves with it.
func (c *Conn) resumeLocked(dts time.Duration) {
	f := c.videoFill
	f.filling = false
	if dts-c.base < f.nextTS {
		c.base = dts - f.nextTS
		c.stats.Rebases++
	}
	log.Info("rtmp video resumed after filling", "destination", c.cfg.Redacted(),
		"filled_frames", c.stats.FilledVideoFrames)
}

// isIDR reports whether the access unit holds an IDR slice, the only picture a stream can resume at after filler
// frames.
func isIDR(nalus [][]byte) bool {
	for _, nalu := range nalus {
		if len(nalu) > 0 && avc.GetNaluType(nalu[0]) == avc.NALU_IDR {
			return true
		}
	}
	return false
}

// stripStartCode returns the NAL unit of an Annex-B slice as hi264 encodes it.
func stripStartCode(b []byte) []byte {
	switch {
	case len(b) >= 4 && b[0] == 0 && b[1] == 0 && b[2] == 0 && b[3] == 1:
		return b[4:]
	case len(b) >= 3 && b[0] == 0 && b[1] == 0 && b[2] == 1:
		return b[3:]
	}
	return b
}
