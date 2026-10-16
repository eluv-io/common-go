package rtmp

import (
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"

	"github.com/eluv-io/common-go/media/fmp4"
	"github.com/eluv-io/errors-go"
)

// aacFrameSamples is the number of samples per channel in an AAC-LC frame, unless the AudioSpecificConfig sets
// frameLengthFlag for 960.
const aacFrameSamples = 1024

// silentAAC holds, per channel configuration, an AAC-LC frame (a raw_data_block) that decodes to silence. libfdk_aac
// encodes digital silence to these same bytes at every sample rate (ffmpeg -f lavfi -i anullsrc=cl=stereo -c:a
// libfdk_aac -vbr 1, any frame after the encoder's start-up), and the decoders of ffmpeg, libfdk and Apple decode them
// to digital silence at sample rates from 8 to 96 kHz.
var silentAAC = [...][]byte{
	1: {0x01, 0x40, 0x22, 0x80, 0xa3, 0x07},             // mono: one single channel element
	2: {0x21, 0x11, 0x45, 0x00, 0x14, 0x50, 0x01, 0x47}, // stereo: one channel pair element
}

// audioFiller continues a starved audio track with silent frames. Every AAC frame decodes on its own, so the source
// can take over again at any frame; what needs care is the timing, since the silence moves the track on in the time
// the source would have filled.
type audioFiller struct {
	frame    []byte        // the silent frame for the track's channel configuration
	frameDur time.Duration // aacFrameSamples at the track's sample rate
	after    time.Duration

	have   bool          // a real frame has been written
	lastTS time.Duration // timestamp of the last real frame, as sent
	lastAt time.Time     // when the last real frame was written

	filling bool          // silent frames were written and the source has not taken over again
	yielded bool          // the source returned behind the silence: write no more, so it catches up
	nextTS  time.Duration // timestamp of the next silent frame, where the silence written so far ends
}

// newAudioFiller prepares a filler for the audio track. It fails for a track it has no silent frame for: other object
// types than AAC-LC (HE-AAC included), 960-sample frames, and channel configurations other than mono and stereo.
func newAudioFiller(a *fmp4.AudioParams, after time.Duration) (*audioFiller, error) {
	e := errors.Template("rtmp.newAudioFiller", errors.K.Invalid)
	var asc mpeg4audio.AudioSpecificConfig
	if err := asc.Unmarshal(a.ASC); err != nil {
		return nil, e(err, "reason", "invalid AudioSpecificConfig")
	}
	switch {
	case asc.Type != mpeg4audio.ObjectTypeAACLC || asc.ExtensionType != 0:
		return nil, e("reason", "not AAC-LC", "codecs", a.Codecs)
	case asc.FrameLengthFlag:
		return nil, e("reason", "960-sample frames")
	case asc.SampleRate <= 0:
		return nil, e("reason", "no sample rate")
	case int(asc.ChannelConfig) >= len(silentAAC) || silentAAC[asc.ChannelConfig] == nil:
		return nil, e("reason", "no silent frame for the channel configuration", "channel_config", asc.ChannelConfig)
	}
	return &audioFiller{
		frame:    silentAAC[asc.ChannelConfig],
		frameDur: aacFrameSamples * time.Second / time.Duration(asc.SampleRate),
		after:    after,
	}, nil
}

// observe records a real frame that is being written.
func (f *audioFiller) observe(ts time.Duration, now time.Time) {
	f.have = true
	f.lastTS = ts
	f.lastAt = now
}

// due returns the timestamp of the next silent frame if one is due at now, starting to fill once no real frame has
// been written for the configured time. As for video, a frame is due when as much time has passed since the last real
// frame was written as lies between their timestamps. Writing the frame is up to the caller, who then calls take.
func (f *audioFiller) due(now time.Time) (time.Duration, bool) {
	if f == nil || f.yielded {
		return 0, false
	}
	next := f.nextTS
	if !f.filling {
		if !f.have || now.Sub(f.lastAt) < f.after {
			return 0, false
		}
		next = f.lastTS + f.frameDur
	}
	if f.lastAt.Add(next - f.lastTS).After(now) {
		return 0, false
	}
	return next, true
}

// take records that the silent frame due at ts is being written.
func (f *audioFiller) take(ts time.Duration) {
	f.filling = true
	f.nextTS = ts + f.frameDur
}

// fillAudioLocked takes the silent frame due at ts and returns it.
func (c *Conn) fillAudioLocked(ts time.Duration) []byte {
	f := c.audioFill
	if !f.filling {
		c.stats.AudioFills++
		log.Info("rtmp audio starved, filling with silence", "destination", c.cfg.Redacted(), "after", f.after)
	}
	f.take(ts)
	c.audioClock.ts, c.audioClock.set = ts, true
	c.stats.AudioFrames++
	c.stats.FilledAudioFrames++
	c.stats.LastAudioTS = ts
	return f.frame
}

// audioResumeLocked decides, while silence is being filled in, whether the source's audio frame at ts (rebased) takes
// over again. A frame that starts more than half a frame before where the silence ends is dropped, since the silence
// already covers it; any later one takes over, so the two overlap by at most half a frame. It reports whether to drop
// the frame.
func (c *Conn) audioResumeLocked(ts time.Duration) (drop bool) {
	f := c.audioFill
	if ts < f.nextTS-f.frameDur/2 {
		c.stats.DroppedAudioWhileFilling++
		if !c.videoFill.active() {
			f.yielded = true
		}
		return true
	}
	f.filling, f.yielded = false, false
	log.Info("rtmp audio resumed after filling", "destination", c.cfg.Redacted(),
		"filled_frames", c.stats.FilledAudioFrames)
	return false
}
