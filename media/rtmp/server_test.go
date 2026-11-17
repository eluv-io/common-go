package rtmp

import (
	"context"
	"crypto/tls"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/fmp4"
	"github.com/eluv-io/common-go/media/rtmp/rtmptest"
)

// acceptResult is what the server side of a test connection got from Accept.
type acceptResult struct {
	p   *Player
	err error
}

// listenOnce accepts one connection on a fresh listener and runs Accept on it. It returns the address and the
// channel the result arrives on.
func listenOnce(t *testing.T, cfg ServerConfig, tlsCfg *tls.Config) (string, <-chan acceptResult) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	res := make(chan acceptResult, 1)
	go func() {
		nconn, err := ln.Accept()
		if err != nil {
			res <- acceptResult{err: err}
			return
		}
		if tlsCfg != nil {
			nconn = tls.Server(nconn, tlsCfg)
		}
		p, err := Accept(context.Background(), nconn, cfg)
		res <- acceptResult{p: p, err: err}
	}()
	return ln.Addr().String(), res
}

func waitAccept(t *testing.T, res <-chan acceptResult) acceptResult {
	t.Helper()
	select {
	case r := <-res:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("Accept did not return")
		return acceptResult{}
	}
}

// startPlayer plays url in the background, since the play does not complete before the server starts the stream.
func startPlayer(url string, tlsCfg *tls.Config) <-chan playResult {
	res := make(chan playResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p, err := rtmptest.Play(ctx, url, tlsCfg)
		res <- playResult{p: p, err: err}
	}()
	return res
}

type playResult struct {
	p   *rtmptest.Player
	err error
}

func waitPlay(t *testing.T, res <-chan playResult) playResult {
	t.Helper()
	select {
	case r := <-res:
		return r
	case <-time.After(15 * time.Second):
		t.Fatal("Play did not return")
		return playResult{}
	}
}

func TestServer_PlaysTracksAndSamples(t *testing.T) {
	params, video, audio := fixtureTracks(t)
	addr, accepted := listenOnce(t, ServerConfig{}, nil)
	played := startPlayer("rtmp://"+addr+"/live/live-out.iq__abc-out1.token", nil)

	ar := waitAccept(t, accepted)
	require.NoError(t, ar.err)
	require.Equal(t, "live", ar.p.App)
	require.Equal(t, "live-out.iq__abc-out1.token", ar.p.StreamName)
	require.Equal(t, "rtmp://"+addr+"/live", ar.p.TcURL)
	require.Equal(t, "live/live-out.iq__abc-out1.token", ar.p.Path())
	require.False(t, ar.p.Encrypted)

	c, err := ar.p.Start(params, Metadata{VideoBitrate: 2_000_000})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	// The player's reader analyses the tracks for a while before Play returns, so the samples have to flow first.
	const start = 50 * time.Second
	nonKey := firstNonKeyframe(t, video)
	require.NoError(t, c.WriteVideo(nonKey.NALUs, start-time.Second, start-time.Second, false))
	nv, na := publishFixtures(t, c, video, audio, start, 3)

	pr := waitPlay(t, played)
	require.NoError(t, pr.err)
	defer pr.p.Close()
	require.True(t, pr.p.WaitFor(func(s rtmptest.Stats) bool {
		return s.VideoFrames == nv && s.AudioFrames == na
	}, 10*time.Second), "wrote %d video and %d audio frames, player got %+v", nv, na, pr.p.Stats())

	s := pr.p.Stats()
	require.Equal(t, params.Video.SPS[0], s.SPS)
	require.Equal(t, params.Video.PPS[0], s.PPS)
	var want, got mpeg4audio.AudioSpecificConfig
	require.NoError(t, want.Unmarshal(params.Audio.ASC))
	require.NoError(t, got.Unmarshal(s.ASC))
	require.Equal(t, want.SampleRate, got.SampleRate)
	require.Equal(t, video[0].NALUs, s.FirstNALUs, "the stream starts at the first keyframe")
	require.Equal(t, time.Duration(0), s.FirstVideoDTS)
	require.True(t, s.VideoMonotonic)
	require.True(t, s.AudioMonotonic)

	cs := c.Stats()
	require.EqualValues(t, 1, cs.DroppedBeforeStart)
	require.Greater(t, cs.BytesSent, uint64(0))
	require.Nil(t, c.Err())
}

func TestPlayer_Path(t *testing.T) {
	for _, tc := range []struct {
		app, name, want string
	}{
		{"live", "stream", "live/stream"},   // ffmpeg: split at the last slash
		{"live/stream", "", "live/stream"},  // gortmplib without a fragment: the whole path is the app
		{"live/", "/stream", "live/stream"}, // stray slashes
		{"", "live/stream", "live/stream"},  // everything in the stream name
		{"live/inst", "stream", "live/inst/stream"},
	} {
		p := &Player{App: tc.app, StreamName: tc.name}
		require.Equal(t, tc.want, p.Path(), "app %q name %q", tc.app, tc.name)
	}
}

func TestServer_PlayerStopEndsTheConnection(t *testing.T) {
	params, video, audio := fixtureTracks(t)
	addr, accepted := listenOnce(t, ServerConfig{}, nil)
	played := startPlayer("rtmp://"+addr+"/live/stream", nil)

	ar := waitAccept(t, accepted)
	require.NoError(t, ar.err)
	c, err := ar.p.Start(params, Metadata{})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	publishFixtures(t, c, video, audio, 0, 3)

	pr := waitPlay(t, played)
	require.NoError(t, pr.err)
	require.NoError(t, pr.p.Stop())
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done must close once the player stops the stream")
	}
	require.ErrorIs(t, c.Err(), ErrStopped)
}

func TestServer_PlayerDropEndsTheConnection(t *testing.T) {
	params, video, audio := fixtureTracks(t)
	addr, accepted := listenOnce(t, ServerConfig{}, nil)
	played := startPlayer("rtmp://"+addr+"/live/stream", nil)

	ar := waitAccept(t, accepted)
	require.NoError(t, ar.err)
	c, err := ar.p.Start(params, Metadata{})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	publishFixtures(t, c, video, audio, 0, 3)

	pr := waitPlay(t, played)
	require.NoError(t, pr.err)
	pr.p.Close()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done must close once the player drops the connection")
	}
	require.Error(t, c.Err())
	require.NotErrorIs(t, c.Err(), ErrStopped)
}

func TestServer_Reject(t *testing.T) {
	addr, accepted := listenOnce(t, ServerConfig{}, nil)
	played := startPlayer("rtmp://"+addr+"/live/unknown", nil)

	ar := waitAccept(t, accepted)
	require.NoError(t, ar.err)
	require.NoError(t, ar.p.Reject(PlayStreamNotFound, "no such output"))

	pr := waitPlay(t, played)
	require.Error(t, pr.err)
	require.Contains(t, pr.err.Error(), PlayStreamNotFound)
}

func TestServer_RefusesPublishers(t *testing.T) {
	params, _, _ := fixtureTracks(t)
	addr, accepted := listenOnce(t, ServerConfig{}, nil)

	_, err := Dial(context.Background(), Config{URL: "rtmp://" + addr + "/live", StreamKey: "key"}, params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "publish refused")

	ar := waitAccept(t, accepted)
	require.Error(t, ar.err)
	require.Contains(t, ar.err.Error(), "publishing is not accepted")
}

func TestServer_HandshakeTimeout(t *testing.T) {
	addr, accepted := listenOnce(t, ServerConfig{HandshakeTimeout: 200 * time.Millisecond}, nil)
	nconn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer func() { _ = nconn.Close() }()

	ar := waitAccept(t, accepted)
	require.Error(t, ar.err, "a client that never completes the handshake must not hold the server")
}

// TestServer_FFprobe plays from the server with ffmpeg's RTMP client, which sends commands gortmplib's does not, such
// as FCSubscribe and a buffer length, and splits the play URL differently.
func TestServer_FFprobe(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	params, video, audio := fixtureTracks(t)
	addr, accepted := listenOnce(t, ServerConfig{WriteTimeout: 2 * time.Second}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, ffprobe, "-v", "error", "-rtmp_live", "live",
		"-show_entries", "stream=codec_name,width,height,sample_rate", "-of", "compact",
		"rtmp://"+addr+"/live/live-out.iq__abc-out1.token")
	out := make(chan []byte, 1)
	go func() {
		bts, _ := probe.CombinedOutput()
		out <- bts
	}()

	ar := waitAccept(t, accepted)
	require.NoError(t, ar.err)
	require.Equal(t, "live/live-out.iq__abc-out1.token", ar.p.Path())
	c, err := ar.p.Start(params, Metadata{})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	// Write until ffprobe has seen enough and hangs up, at about real time so it gets a live-looking stream.
	vt := video[0].Track
	span := fmp4.TicksToDuration(video[len(video)-1].DTS+int64(video[len(video)-1].Duration)-video[0].DTS, vt.Timescale)
	go func() {
		for i := 0; ctx.Err() == nil; i++ {
			off := time.Duration(i) * span
			for _, s := range video {
				dts := off + fmp4.TicksToDuration(s.DTS-video[0].DTS, vt.Timescale)
				pts := off + fmp4.TicksToDuration(s.PTS-video[0].DTS, vt.Timescale)
				if c.WriteVideo(s.NALUs, dts, pts, s.Sync) != nil {
					return
				}
			}
			for _, s := range audio {
				pts := off + fmp4.TicksToDuration(s.DTS-audio[0].DTS, audio[0].Track.Timescale)
				if c.WriteAudio(s.Data, pts) != nil {
					return
				}
			}
			time.Sleep(span / 2)
		}
	}()

	var bts []byte
	select {
	case bts = <-out:
	case <-ctx.Done():
		t.Fatal("ffprobe did not finish")
	}
	require.Contains(t, string(bts), "codec_name=h264", "ffprobe output: %s", bts)
	require.Contains(t, string(bts), "codec_name=aac", "ffprobe output: %s", bts)
}

func TestServer_TLS(t *testing.T) {
	cert, pool, err := rtmptest.SelfSignedCert("127.0.0.1")
	require.NoError(t, err)
	params, video, audio := fixtureTracks(t)
	addr, accepted := listenOnce(t, ServerConfig{}, &tls.Config{Certificates: []tls.Certificate{cert}})
	played := startPlayer("rtmps://"+addr+"/live/stream", &tls.Config{RootCAs: pool})

	ar := waitAccept(t, accepted)
	require.NoError(t, ar.err)
	require.True(t, ar.p.Encrypted)
	c, err := ar.p.Start(params, Metadata{})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	nv, _ := publishFixtures(t, c, video, audio, 0, 3)

	pr := waitPlay(t, played)
	require.NoError(t, pr.err)
	defer pr.p.Close()
	require.True(t, pr.p.WaitFor(func(s rtmptest.Stats) bool { return s.VideoFrames == nv }, 10*time.Second))
}
