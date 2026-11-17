package rtmp

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"

	"github.com/eluv-io/common-go/media/fmp4"
	mio "github.com/eluv-io/common-go/media/io"
	"github.com/eluv-io/errors-go"
)

// ErrClosed is the error a Conn reports after Close.
var ErrClosed = errors.Str("rtmp: connection closed")

// DefaultEncoder is the encoder name announced in onMetaData when the configuration sets none.
const DefaultEncoder = "eluvio content fabric"

// maxPlausibleFrameRate bounds the frame rate taken from an SPS for onMetaData. Above it the VUI timing describes a
// time base rather than a frame rate and is not announced.
const maxPlausibleFrameRate = 240

// Conn is one RTMP publishing session. It is created connected and announced (Dial) and ends when the server drops
// it, a write fails, or Close is called; Done closes then and Err tells why. Writes are safe from one goroutine at a
// time; the read loop answering the server's control messages runs concurrently and is serialized with them.
//
// Timestamps are rebased per connection: the first video keyframe written defines time zero, and everything written
// before it is dropped, since a decoder can only start at a keyframe and the sequence headers sent at connect must be
// followed by one. Per track the timestamps sent are non-decreasing, which the strictest ingest servers require; a
// sample that would go backwards is clamped to the previous timestamp and counted.
type Conn struct {
	cfg    Config
	params fmp4.CodecParams
	s      *session

	mu         sync.Mutex // guards the timeline state and stats; the session serializes the writes themselves
	started    bool
	base       time.Duration
	videoClock trackClock
	audioClock trackClock
	stats      Stats

	done     chan struct{}
	failOnce sync.Once
	closed   atomic.Bool
	err      atomic.Pointer[error]
}

// Stats is a snapshot of a Conn's counters.
type Stats struct {
	ConnectedAt        time.Time     `json:"connected_at"`
	RemoteAddr         string        `json:"remote_addr"`
	BytesSent          uint64        `json:"bytes_sent"`
	VideoFrames        uint64        `json:"video_frames"`
	Keyframes          uint64        `json:"keyframes"`
	AudioFrames        uint64        `json:"audio_frames"`
	DroppedBeforeStart uint64        `json:"dropped_before_start"` // samples before the first video keyframe
	ClampedTimestamps  uint64        `json:"clamped_timestamps"`   // samples whose timestamp went backwards
	NegativeCTS        uint64        `json:"negative_cts"`         // video samples with PTS < DTS, sent with CTS 0
	LastVideoTS        time.Duration `json:"last_video_ts"`        // as sent, i.e. rebased
	LastAudioTS        time.Duration `json:"last_audio_ts"`
}

// Dial connects to the server named by cfg, completes the publish handshake and announces the tracks in params:
// onMetaData, then the H.264 and AAC sequence headers. params must have a video track; the audio track is optional.
// On return the connection is ready for WriteVideo and WriteAudio.
func Dial(ctx context.Context, cfg Config, params fmp4.CodecParams) (*Conn, error) {
	e := errors.Template("rtmp.Dial", errors.K.IO, "destination", cfg.Redacted())

	u, err := cfg.publishURL()
	if err != nil {
		return nil, err
	}
	if params.Video == nil {
		return nil, e(errors.K.Invalid, "reason", "a video track is required")
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.connectTimeout())
	defer cancel()
	s, err := dialSession(ctx, u, cfg.TLS, cfg.writeTimeout())
	if err != nil {
		return nil, e(err)
	}

	c := newConn(cfg, params, s)

	if err = c.writeHeaders(); err != nil {
		_ = s.close()
		return nil, e(err, "reason", "failed to announce tracks")
	}
	go c.readLoop()
	log.Info("rtmp connected", "destination", cfg.Redacted(), "remote", c.stats.RemoteAddr,
		"video", params.Video.Codecs, "audio", audioCodecs(params.Audio))
	return c, nil
}

// newConn wraps an established session. It neither announces the tracks nor starts the read loop, which is what
// tests and benchmarks that drive the write path over a pipe want.
func newConn(cfg Config, params fmp4.CodecParams, s *session) *Conn {
	c := &Conn{
		cfg:    cfg,
		params: params,
		s:      s,
		done:   make(chan struct{}),
	}
	c.stats.ConnectedAt = time.Now()
	c.stats.RemoteAddr = s.nconn.RemoteAddr().String()
	return c
}

func audioCodecs(a *fmp4.AudioParams) string {
	if a == nil {
		return "none"
	}
	return a.Codecs
}

// writeHeaders sends onMetaData and the codec sequence headers.
func (c *Conn) writeHeaders() error {
	var sps h264.SPS
	if err := sps.Unmarshal(c.params.Video.SPS[0]); err != nil {
		return errors.E("rtmp.headers", errors.K.Invalid, err, "reason", "invalid SPS")
	}
	var asc *mpeg4audio.AudioSpecificConfig
	if c.params.Audio != nil {
		asc = &mpeg4audio.AudioSpecificConfig{}
		if err := asc.Unmarshal(c.params.Audio.ASC); err != nil {
			return errors.E("rtmp.headers", errors.K.Invalid, err, "reason", "invalid AudioSpecificConfig")
		}
	}

	if err := c.s.write(&message.DataAMF0{
		ChunkStreamID:   streamCsID,
		MessageStreamID: publishStreamID,
		Payload:         []any{"@setDataFrame", "onMetaData", c.metadata(&sps, asc)},
	}); err != nil {
		return err
	}

	if err := c.s.write(&message.Video{
		ChunkStreamID:   message.VideoChunkStreamID,
		MessageStreamID: publishStreamID,
		Codec:           message.CodecH264,
		IsKeyFrame:      true,
		Type:            message.VideoTypeConfig,
		AVCConfig:       avcDecoderConfiguration(&sps, c.params.Video.SPS[0], c.params.Video.PPS[0]),
	}); err != nil {
		return err
	}

	if asc != nil {
		if err := c.s.write(&message.Audio{
			ChunkStreamID:   message.AudioChunkStreamID,
			MessageStreamID: publishStreamID,
			Codec:           message.CodecMPEG4Audio,
			Rate:            message.AudioRate44100,
			Depth:           message.AudioDepth16,
			IsStereo:        true,
			AACType:         message.AudioAACTypeConfig,
			AACConfig:       asc,
		}); err != nil {
			return err
		}
	}
	return nil
}

// metadata builds the onMetaData object. The FLV audio header hard-codes 44.1 kHz stereo for AAC by convention, so
// the real sample rate and channel count only reach the server through this object and the AudioSpecificConfig.
func (c *Conn) metadata(sps *h264.SPS, asc *mpeg4audio.AudioSpecificConfig) amf0.Object {
	meta := c.cfg.Meta
	if meta.Width == 0 {
		meta.Width = sps.Width()
	}
	if meta.Height == 0 {
		meta.Height = sps.Height()
	}
	if meta.FrameRate == 0 {
		// The VUI timing of an SPS is not always the frame rate: encoders that only know their time base write it
		// there, and the quotient then comes out as the timescale (90000) rather than the rate. Only announce a
		// value that can be one.
		if fps := sps.FPS(); fps >= 1 && fps <= maxPlausibleFrameRate {
			meta.FrameRate = fps
		}
	}
	if meta.Encoder == "" {
		meta.Encoder = DefaultEncoder
	}
	obj := amf0.Object{
		{Key: "duration", Value: float64(0)},
		{Key: "width", Value: float64(meta.Width)},
		{Key: "height", Value: float64(meta.Height)},
		{Key: "videodatarate", Value: float64(meta.VideoBitrate) / 1000},
	}
	if meta.FrameRate != 0 {
		obj = append(obj, amf0.ObjectEntry{Key: "framerate", Value: meta.FrameRate})
	}
	obj = append(obj, amf0.ObjectEntry{Key: "videocodecid", Value: float64(message.CodecH264)})
	if asc != nil {
		channels := asc.ChannelCount
		if channels == 0 {
			channels = int(asc.ChannelConfig)
		}
		obj = append(obj,
			amf0.ObjectEntry{Key: "audiodatarate", Value: float64(meta.AudioBitrate) / 1000},
			amf0.ObjectEntry{Key: "audiosamplerate", Value: float64(asc.SampleRate)},
			amf0.ObjectEntry{Key: "audiosamplesize", Value: float64(16)},
			amf0.ObjectEntry{Key: "audiochannels", Value: float64(channels)},
			amf0.ObjectEntry{Key: "stereo", Value: channels == 2},
			amf0.ObjectEntry{Key: "audiocodecid", Value: float64(message.CodecMPEG4Audio)},
		)
	}
	obj = append(obj, amf0.ObjectEntry{Key: "encoder", Value: meta.Encoder})
	return obj
}

// avcDecoderConfiguration builds the AVCDecoderConfigurationRecord the H.264 sequence header carries.
func avcDecoderConfiguration(sps *h264.SPS, spsNalu, ppsNalu []byte) *mp4.AVCDecoderConfiguration {
	return &mp4.AVCDecoderConfiguration{
		AnyTypeBox:                 mp4.AnyTypeBox{Type: mp4.BoxTypeAvcC()},
		ConfigurationVersion:       1,
		Profile:                    sps.ProfileIdc,
		ProfileCompatibility:       spsNalu[2],
		Level:                      sps.LevelIdc,
		Reserved:                   0b111111,
		LengthSizeMinusOne:         3,
		Reserved2:                  0b111,
		NumOfSequenceParameterSets: 1,
		SequenceParameterSets:      []mp4.AVCParameterSet{{Length: uint16(len(spsNalu)), NALUnit: spsNalu}},
		NumOfPictureParameterSets:  1,
		PictureParameterSets:       []mp4.AVCParameterSet{{Length: uint16(len(ppsNalu)), NALUnit: ppsNalu}},
	}
}

// readLoop drains the server's messages until the connection ends. Pings and acknowledgements are answered inside
// session.read; an onStatus with level "error" ends the connection with its description, since servers report a
// rejected or stopped publish that way rather than by closing.
func (c *Conn) readLoop() {
	for {
		msg, err := c.s.read()
		if err != nil {
			c.fail(errors.E("rtmp.read", errors.K.IO, err, "destination", c.cfg.Redacted()))
			return
		}
		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.Name == "onStatus" && !statusIsOK(cmd) {
				c.fail(errors.E("rtmp.status", errors.K.IO,
					"reason", "server reported an error", "status", describeCommand(cmd),
					"destination", c.cfg.Redacted()))
				return
			}
			log.Debug("rtmp server command", "command", describeCommand(cmd), "destination", c.cfg.Redacted())
		}
	}
}

// fail ends the connection with err, once. Errors after Close are folded into ErrClosed.
func (c *Conn) fail(err error) {
	c.failOnce.Do(func() {
		if c.closed.Load() {
			err = ErrClosed
		}
		c.err.Store(&err)
		_ = c.s.close()
		close(c.done)
	})
}

// Close ends the connection. Done closes and Err returns ErrClosed.
func (c *Conn) Close() error {
	c.closed.Store(true)
	c.fail(ErrClosed)
	return nil
}

// Done closes when the connection has ended, for whatever reason.
func (c *Conn) Done() <-chan struct{} {
	return c.done
}

// Err returns why the connection ended: nil while it is alive, ErrClosed after Close, the failing write or read
// otherwise.
func (c *Conn) Err() error {
	if p := c.err.Load(); p != nil {
		return *p
	}
	return nil
}

// WriteVideo sends one H.264 access unit. nalus are the NAL units without length prefixes or start codes; keyframe
// marks a random access point. dts and pts are on the caller's time axis; the connection rebases them.
//
// The NAL units are written to the socket before this returns and are not retained, so a caller that owns them may
// reuse or release them afterwards.
func (c *Conn) WriteVideo(nalus [][]byte, dts, pts time.Duration, keyframe bool) error {
	if err := c.Err(); err != nil {
		return err
	}
	if len(nalus) == 0 {
		return nil
	}
	keyframe = keyframe || h264.IsRandomAccess(nalus)

	c.mu.Lock()
	if !c.started {
		if !keyframe {
			c.stats.DroppedBeforeStart++
			c.mu.Unlock()
			return nil
		}
		c.started = true
		c.base = dts
	}
	ts := c.rebaseLocked(&c.videoClock, dts)
	cts := pts - dts
	if cts < 0 {
		c.stats.NegativeCTS++
		cts = 0
	}
	c.stats.VideoFrames++
	if keyframe {
		c.stats.Keyframes++
	}
	c.stats.LastVideoTS = ts
	c.mu.Unlock()

	return c.writeResult(c.s.writeVideo(nalus, millis(ts), millis(cts), keyframe))
}

// millis converts a duration to the milliseconds an RTMP timestamp counts. The field is 32 bits and wraps after
// about 49 days of stream time, which is what every RTMP implementation does with it.
func millis(d time.Duration) uint32 {
	return uint32(d / time.Millisecond)
}

// WriteAudio sends one AAC access unit (a raw frame, no ADTS header). Audio before the first video keyframe is
// dropped like video is, so the two tracks share the same origin.
func (c *Conn) WriteAudio(au []byte, pts time.Duration) error {
	if err := c.Err(); err != nil {
		return err
	}
	if c.params.Audio == nil {
		return errors.E("rtmp.WriteAudio", errors.K.Invalid, "reason", "connection has no audio track")
	}
	if len(au) == 0 {
		return nil
	}

	c.mu.Lock()
	if !c.started || pts < c.base {
		c.stats.DroppedBeforeStart++
		c.mu.Unlock()
		return nil
	}
	ts := c.rebaseLocked(&c.audioClock, pts)
	c.stats.AudioFrames++
	c.stats.LastAudioTS = ts
	c.mu.Unlock()

	return c.writeResult(c.s.writeAudio(au, millis(ts)))
}

// trackClock is what one track's timestamps were rebased to last.
type trackClock struct {
	ts  time.Duration
	set bool
}

// rebaseLocked maps t onto the connection's time axis and keeps the track's timestamps non-decreasing.
func (c *Conn) rebaseLocked(clock *trackClock, t time.Duration) time.Duration {
	ts := t - c.base
	if clock.set && ts < clock.ts {
		c.stats.ClampedTimestamps++
		ts = clock.ts
	}
	clock.ts = ts
	clock.set = true
	return ts
}

// writeResult ends the connection on a write error and returns it.
func (c *Conn) writeResult(err error) error {
	if err == nil {
		return nil
	}
	err = errors.E("rtmp.write", errors.K.IO, err, "destination", c.cfg.Redacted())
	c.fail(err)
	return c.Err()
}

// Stats returns a snapshot of the counters.
func (c *Conn) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats
	s.BytesSent = c.s.bc.Writer.Count()
	return s
}

// ConnStats implements mio.StatsReporter.
func (c *Conn) ConnStats(into *mio.ConnStats, _ bool) {
	if into == nil {
		return
	}
	*into = mio.ConnStats{RemoteAddr: c.stats.RemoteAddr, LocalAddr: c.s.nconn.LocalAddr().String()}
}

// RemoteAddr returns the server's address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.s.nconn.RemoteAddr()
}
