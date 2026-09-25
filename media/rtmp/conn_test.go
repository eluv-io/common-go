package rtmp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/fmp4"
	"github.com/eluv-io/common-go/media/rtmp/rtmptest"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	bts, err := os.ReadFile(filepath.Join("..", "fmp4", "testdata", name))
	require.NoError(t, err)
	return bts
}

// fixtureTracks loads the AVC and AAC fixtures: their tracks, and the samples of one media segment each.
func fixtureTracks(t *testing.T) (params fmp4.CodecParams, video, audio []*fmp4.Sample) {
	t.Helper()
	vinit, err := fmp4.ParseInit(bytes.NewReader(fixture(t, "vinit-stream0.m4s")))
	require.NoError(t, err)
	ainit, err := fmp4.ParseInit(bytes.NewReader(fixture(t, "ainit.m4s")))
	require.NoError(t, err)
	v, a := vinit.Track(fmp4.KindVideo), ainit.Track(fmp4.KindAudio)
	params = fmp4.CodecParams{Video: v.VideoParams(), Audio: a.AudioParams()}

	collect := func(name string, track *fmp4.TrackInfo) []*fmp4.Sample {
		var samples []*fmp4.Sample
		err := fmp4.ReadFragments(bytes.NewReader(fixture(t, name)), track, func(s *fmp4.Sample) error {
			samples = append(samples, s)
			return nil
		})
		require.NoError(t, err)
		require.NotEmpty(t, samples)
		return samples
	}
	return params, collect("vchunk-stream0-00001.m4s", v), collect("achunk-00001.m4s", a)
}

// publishFixtures writes the fixture segments repeated `repeats` times back to back, starting at `start` on the
// caller's time axis, so the receiver sees more than its 2 s track-analysis window. It returns how many video and
// audio frames it wrote.
func publishFixtures(t *testing.T, c *Conn, video, audio []*fmp4.Sample, start time.Duration, repeats int) (int, int) {
	t.Helper()
	vt, at := video[0].Track, audio[0].Track
	vspan := fmp4.TicksToDuration(video[len(video)-1].DTS+int64(video[len(video)-1].Duration)-video[0].DTS, vt.Timescale)
	aspan := fmp4.TicksToDuration(audio[len(audio)-1].DTS+int64(audio[len(audio)-1].Duration)-audio[0].DTS, at.Timescale)
	nv, na := 0, 0
	for i := 0; i < repeats; i++ {
		voff := start + time.Duration(i)*vspan
		for _, s := range video {
			dts := voff + fmp4.TicksToDuration(s.DTS-video[0].DTS, vt.Timescale)
			pts := voff + fmp4.TicksToDuration(s.PTS-video[0].DTS, vt.Timescale)
			require.NoError(t, c.WriteVideo(s.NALUs, dts, pts, s.Sync))
			nv++
		}
	}
	// the audio fixture is shorter than the video one: repeat it until it covers the video span
	for aoff := start; aoff < start+time.Duration(repeats)*vspan; aoff += aspan {
		for _, s := range audio {
			require.NoError(t, c.WriteAudio(s.Data, aoff+fmp4.TicksToDuration(s.DTS-audio[0].DTS, at.Timescale)))
			na++
		}
	}
	return nv, na
}

// firstNonKeyframe returns the first sample of the segment that is not a random access point.
func firstNonKeyframe(t *testing.T, video []*fmp4.Sample) *fmp4.Sample {
	t.Helper()
	for _, s := range video {
		if !s.Sync && !h264.IsRandomAccess(s.NALUs) {
			return s
		}
	}
	t.Fatal("fixture has no non-keyframe")
	return nil
}

func testConfig(rcv *rtmptest.Receiver) Config {
	return Config{URL: rcv.URL(), StreamKey: "key-1", ConnectTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
}

func TestConn_PublishesTracksAndSamples(t *testing.T) {
	rcv, err := rtmptest.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = rcv.Close() }()

	params, video, audio := fixtureTracks(t)
	cfg := testConfig(rcv)
	cfg.Meta.VideoBitrate = 4_000_000
	c, err := Dial(context.Background(), cfg, params)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	// the stream starts far from zero on the input axis; everything sent is rebased to start at zero
	const start = 100 * time.Second
	// a non-keyframe ahead of the first keyframe must be dropped, not sent
	nonKey := firstNonKeyframe(t, video)
	require.NoError(t, c.WriteVideo(nonKey.NALUs, start-time.Second, start-time.Second, false))
	nv, na := publishFixtures(t, c, video, audio, start, 3)

	require.True(t, rcv.WaitFor(func(s rtmptest.Stats) bool {
		return s.VideoFrames == nv && s.AudioFrames == na
	}, 10*time.Second), "wrote %d video and %d audio frames, conn stats %+v, receiver got video=%d audio=%d",
		nv, na, c.Stats(), rcv.Stats().VideoFrames, rcv.Stats().AudioFrames)

	s := rcv.Stats()
	require.Equal(t, 1, s.Connections)
	require.Equal(t, "/live", s.App)
	require.Equal(t, "key-1", s.StreamKey)
	require.Equal(t, params.Video.SPS[0], s.SPS)
	require.Equal(t, params.Video.PPS[0], s.PPS)
	// the AAC sequence header carries the AudioSpecificConfig re-encoded without the fixture's trailing extension
	// bytes (an SBR absence signal), so compare the decoded configuration rather than the raw bytes
	var want, got mpeg4audio.AudioSpecificConfig
	require.NoError(t, want.Unmarshal(params.Audio.ASC))
	require.NoError(t, got.Unmarshal(s.ASC))
	require.Equal(t, want.Type, got.Type)
	require.Equal(t, want.SampleRate, got.SampleRate)
	require.Equal(t, want.ChannelConfig, got.ChannelConfig)
	require.Equal(t, video[0].NALUs, s.FirstNALUs, "NAL units arrive unchanged")
	require.Equal(t, time.Duration(0), s.FirstVideoDTS, "timestamps are rebased to the first keyframe")
	require.Equal(t, time.Duration(0), s.FirstAudioPTS)
	require.True(t, s.VideoMonotonic)
	require.True(t, s.AudioMonotonic)
	require.GreaterOrEqual(t, s.Keyframes, 3)

	require.EqualValues(t, params.Video.Width, s.Metadata["width"])
	require.EqualValues(t, params.Video.Height, s.Metadata["height"])
	require.EqualValues(t, 4000, s.Metadata["videodatarate"])
	require.EqualValues(t, params.Audio.SampleRate, s.Metadata["audiosamplerate"])
	require.EqualValues(t, params.Audio.Channels, s.Metadata["audiochannels"])
	require.Equal(t, DefaultEncoder, s.Metadata["encoder"])
	// the fixture's SPS carries its time base in the VUI timing, which yields a rate in the tens of thousands, so no
	// frame rate is announced unless the configuration names one
	require.NotContains(t, s.Metadata, "framerate")

	cs := c.Stats()
	require.EqualValues(t, 1, cs.DroppedBeforeStart, "the frame before the first keyframe is dropped")
	require.EqualValues(t, nv, cs.VideoFrames)
	require.EqualValues(t, na, cs.AudioFrames)
	require.Zero(t, cs.ClampedTimestamps)
	require.Greater(t, cs.BytesSent, uint64(0))
	require.NotEmpty(t, cs.RemoteAddr)
	require.Nil(t, c.Err())
}

func TestConn_ServerDropEndsTheConnection(t *testing.T) {
	rcv, err := rtmptest.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = rcv.Close() }()

	params, video, _ := fixtureTracks(t)
	c, err := Dial(context.Background(), testConfig(rcv), params)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	require.NoError(t, c.WriteVideo(video[0].NALUs, 0, 0, true))
	require.True(t, rcv.WaitFor(func(s rtmptest.Stats) bool { return s.Active == 1 }, 5*time.Second))

	rcv.Kill()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done must close once the server drops the connection")
	}
	require.Error(t, c.Err())
	require.Error(t, c.WriteVideo(video[0].NALUs, time.Second, time.Second, true), "writes fail once the connection ended")
	require.True(t, rcv.WaitFor(func(s rtmptest.Stats) bool { return s.Disconnects == 1 }, 5*time.Second))
}

func TestConn_Close(t *testing.T) {
	rcv, err := rtmptest.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = rcv.Close() }()

	params, _, _ := fixtureTracks(t)
	c, err := Dial(context.Background(), testConfig(rcv), params)
	require.NoError(t, err)
	require.NoError(t, c.Close())
	<-c.Done()
	require.ErrorIs(t, c.Err(), ErrClosed)
	require.ErrorIs(t, c.WriteAudio([]byte{1}, 0), ErrClosed)
}

func TestConn_TLS(t *testing.T) {
	cert, pool, err := rtmptest.SelfSignedCert("127.0.0.1")
	require.NoError(t, err)
	rcv, err := rtmptest.Listen("127.0.0.1:0", rtmptest.WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}}))
	require.NoError(t, err)
	defer func() { _ = rcv.Close() }()
	require.Contains(t, rcv.URL(), "rtmps://")

	params, video, _ := fixtureTracks(t)
	cfg := testConfig(rcv)
	require.True(t, cfg.Encrypted())
	cfg.TLS = &tls.Config{RootCAs: pool}
	cfg.Meta.FrameRate = 30
	c, err := Dial(context.Background(), cfg, params)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	require.NoError(t, c.WriteVideo(video[0].NALUs, 0, 0, true))
	require.True(t, rcv.WaitFor(func(s rtmptest.Stats) bool { return s.Connections == 1 && s.Metadata != nil },
		5*time.Second))
	require.EqualValues(t, 30, rcv.Stats().Metadata["framerate"], "a configured frame rate is announced")

	// a client that does not trust the server's certificate must fail the handshake
	cfg.TLS = &tls.Config{RootCAs: x509.NewCertPool()}
	_, err = Dial(context.Background(), cfg, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tls handshake failed")
}

func TestConn_Auth(t *testing.T) {
	rcv, err := rtmptest.Listen("127.0.0.1:0", rtmptest.WithAuth("alice", "secret"))
	require.NoError(t, err)
	defer func() { _ = rcv.Close() }()
	params, _, _ := fixtureTracks(t)

	cfg := testConfig(rcv)
	_, err = Dial(context.Background(), cfg, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires authentication")

	cfg.Username, cfg.Password = "alice", "wrong"
	_, err = Dial(context.Background(), cfg, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "authentication failed")

	cfg.Password = "secret"
	c, err := Dial(context.Background(), cfg, params)
	require.NoError(t, err)
	_ = c.Close()
	require.Equal(t, "rtmp://alice:***@"+rcv.Addr().String()+"/live/***", cfg.Redacted())
}

func TestConn_ExtendedTimestampsAndClamping(t *testing.T) {
	rcv, err := rtmptest.Listen("127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = rcv.Close() }()

	params, video, audio := fixtureTracks(t)
	c, err := Dial(context.Background(), testConfig(rcv), params)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	key := video[0]
	require.True(t, key.Sync)
	// the 24-bit chunk timestamp saturates at 0xFFFFFF ms (about 4 h 39 min); beyond it the extended timestamp
	// field carries the value, and the receiver must read it back exactly (to the millisecond)
	const far = 5 * time.Hour
	require.NoError(t, c.WriteVideo(key.NALUs, 0, 0, true))
	require.NoError(t, c.WriteVideo(key.NALUs, far, far, true))
	// a timestamp going backwards on a track is clamped to the previous one, not sent as is
	require.NoError(t, c.WriteVideo(key.NALUs, far-time.Second, far-time.Second, true))
	require.NoError(t, c.WriteAudio(audio[0].Data, 0))
	require.NoError(t, c.WriteAudio(audio[0].Data, far+time.Second))
	require.NoError(t, c.WriteAudio(audio[0].Data, far))
	// the receiver's track analysis needs 2 s of media time before it reports frames, hence the far timestamps
	require.True(t, rcv.WaitFor(func(s rtmptest.Stats) bool { return s.VideoFrames == 3 && s.AudioFrames == 3 },
		10*time.Second), "receiver got %+v", rcv.Stats())

	s := rcv.Stats()
	require.Equal(t, far, s.LastVideoDTS)
	require.Equal(t, far+time.Second, s.LastAudioPTS)
	require.True(t, s.VideoMonotonic)
	require.True(t, s.AudioMonotonic)
	require.EqualValues(t, 2, c.Stats().ClampedTimestamps)
	require.Equal(t, far, c.Stats().LastVideoTS)
}

func TestConfig_PublishURL(t *testing.T) {
	good := Config{URL: "rtmp://a.rtmp.youtube.com/live2", StreamKey: "abcd-efgh"}
	u, err := good.publishURL()
	require.NoError(t, err)
	require.Equal(t, "rtmp://a.rtmp.youtube.com:1935/live2#abcd-efgh", u.String())
	require.NoError(t, good.Validate())

	secure := Config{URL: "rtmps://live-api-s.facebook.com:443/rtmp/", StreamKey: "FB-123", Username: "u", Password: "p"}
	u, err = secure.publishURL()
	require.NoError(t, err)
	require.Equal(t, "rtmps://u:p@live-api-s.facebook.com:443/rtmp#FB-123", u.String())
	require.True(t, secure.Encrypted())
	require.Equal(t, "rtmps://u:***@live-api-s.facebook.com:443/rtmp//***", secure.Redacted())

	defaultTLSPort := Config{URL: "rtmps://x.example.com/app", StreamKey: "k"}
	u, err = defaultTLSPort.publishURL()
	require.NoError(t, err)
	require.Equal(t, "x.example.com:443", u.Host)

	bad := []struct {
		name string
		cfg  Config
		msg  string
	}{
		{"scheme", Config{URL: "udp://h/app", StreamKey: "k"}, "scheme must be rtmp or rtmps"},
		{"no host", Config{URL: "rtmp:///app", StreamKey: "k"}, "no host"},
		{"user info", Config{URL: "rtmp://u:p@h/app", StreamKey: "k"}, "must not carry user info"},
		{"fragment", Config{URL: "rtmp://h/app#key", StreamKey: "k"}, "must not carry a fragment"},
		{"no app", Config{URL: "rtmp://h/", StreamKey: "k"}, "no application path"},
		{"no key", Config{URL: "rtmp://h/app"}, "stream key is required"},
		{"bad key", Config{URL: "rtmp://h/app", StreamKey: "a/b"}, "stream key must not contain"},
		{"half credentials", Config{URL: "rtmp://h/app", StreamKey: "k", Username: "u"}, "set together"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, tc.cfg.Validate(), tc.msg)
		})
	}
}

func TestCheck(t *testing.T) {
	params, _, _ := fixtureTracks(t)
	cfg := Config{URL: "rtmps://live-api-s.facebook.com/rtmp", StreamKey: "k"}
	require.Empty(t, Check(cfg, params), "the fixtures are AAC-LC stereo 48 kHz and H.264 high profile")

	cfg.URL = "rtmp://live-api-s.facebook.com/rtmp"
	warnings := Check(cfg, params)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "RTMPS only")

	videoOnly := fmp4.CodecParams{Video: params.Video}
	warnings = Check(Config{URL: "rtmp://a.rtmp.youtube.com/live2", StreamKey: "k"}, videoOnly)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "no audio track")

	require.Contains(t, Check(cfg, fmp4.CodecParams{})[0], "no video track")
}
