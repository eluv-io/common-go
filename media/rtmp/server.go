package rtmp

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"time"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	"github.com/bluenviron/gortmplib/pkg/message"

	"github.com/eluv-io/common-go/media/fmp4"
	"github.com/eluv-io/errors-go"
)

// DefaultHandshakeTimeout bounds the RTMP handshake and the command exchange up to a player's play command.
const DefaultHandshakeTimeout = 10 * time.Second

// statusCsID is the chunk stream the play status messages go out on, as gortmplib's server and nginx-rtmp use it.
const statusCsID = 5

// Status codes a server rejects a play with.
const (
	PlayStreamNotFound = "NetStream.Play.StreamNotFound"
	PlayFailed         = "NetStream.Play.Failed"
)

// ServerConfig configures the server side of a connection a player opened.
type ServerConfig struct {
	HandshakeTimeout time.Duration // 0 = DefaultHandshakeTimeout
	WriteTimeout     time.Duration // 0 = DefaultWriteTimeout
}

func (c *ServerConfig) handshakeTimeout() time.Duration {
	if c.HandshakeTimeout > 0 {
		return c.HandshakeTimeout
	}
	return DefaultHandshakeTimeout
}

func (c *ServerConfig) writeTimeout() time.Duration {
	if c.WriteTimeout > 0 {
		return c.WriteTimeout
	}
	return DefaultWriteTimeout
}

// Player is a connection a player opened, stopped at its play command. The caller decides from what it asked for:
// Reject turns it away, Start announces the tracks and returns the Conn to write the stream to. Allow answers the play
// command without announcing the tracks yet, for a caller that still has to wait for them.
//
// Nothing is read from the player between Accept and Start, so a caller should not hold a Player for long.
type Player struct {
	// App is the application the player connected to, e.g. "live".
	App string
	// TcURL is the URL the player connected with, as it sent it.
	TcURL string
	// StreamName is the stream the player asked to play, as it sent it. It may carry a secret.
	StreamName string
	// Encrypted reports whether the player connected over rtmps, i.e. Accept was handed a TLS connection.
	Encrypted bool

	s       *session
	play    *message.CommandAMF0
	allowed bool
}

// Accept runs the server side of a connection a player opened: the RTMP handshake, connect, createStream and play.
// The handshake timeout and ctx bound the whole exchange. For rtmps, nconn is the tls.Server connection, whose TLS
// handshake runs under the same bound. A client that publishes rather than plays is refused.
//
// On success nconn belongs to the returned Player. On error it is closed.
func Accept(ctx context.Context, nconn net.Conn, cfg ServerConfig) (*Player, error) {
	e := errors.Template("rtmp.Accept", errors.K.IO, "remote", nconn.RemoteAddr().String())

	deadline := time.Now().Add(cfg.handshakeTimeout())
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = nconn.SetDeadline(deadline)
	// a cancelled context unblocks the exchange by moving the deadline into the past
	stop := context.AfterFunc(ctx, func() { _ = nconn.SetDeadline(time.Unix(1, 0)) })

	p, err := acceptPlay(nconn, cfg)
	if !stop() && err == nil {
		err = ctx.Err()
	}
	if err != nil {
		_ = nconn.Close()
		return nil, e(err)
	}
	// Reads never time out afterwards: a player sends nothing for long stretches. The write timeout is what catches
	// one that is gone.
	_ = nconn.SetDeadline(time.Time{})
	return p, nil
}

func acceptPlay(nconn net.Conn, cfg ServerConfig) (*Player, error) {
	s := newSession(nconn, cfg.writeTimeout())
	keyIn, _, err := handshake.DoServer(s.bc, false)
	if err != nil {
		return nil, errors.E("rtmp.handshake", errors.K.IO, err, "reason", "rtmp handshake failed")
	}
	if keyIn != nil {
		return nil, errors.E("rtmp.handshake", errors.K.Invalid, "reason", "encrypted rtmp (rtmpe) is not supported")
	}

	_, encrypted := nconn.(*tls.Conn)
	p := &Player{s: s, Encrypted: encrypted}
	if err = p.acceptConnect(); err != nil {
		return nil, err
	}
	for {
		cmd, err := s.readCommand()
		if err != nil {
			return nil, err
		}
		switch cmd.Name {
		case "createStream":
			err = s.write(&message.CommandAMF0{
				ChunkStreamID: cmd.ChunkStreamID,
				Name:          "_result",
				CommandID:     cmd.CommandID,
				Arguments:     []any{nil, float64(1)},
			})
			if err != nil {
				return nil, err
			}
		case "play":
			name, ok := commandString(cmd, 1)
			if !ok {
				return nil, errors.E("rtmp.play", errors.K.Invalid, "reason", "play command without a stream name")
			}
			p.StreamName, p.play = name, cmd
			return p, nil
		case "publish":
			// The answer is a courtesy so the client reports a reason. The connection is closed either way.
			_ = s.write(statusMessage(cmd, "error", "NetStream.Publish.BadName", "publishing is not accepted"))
			return nil, errors.E("rtmp.publish", errors.K.Permission, "reason", "publishing is not accepted")
		default:
			// FCSubscribe, getStreamLength, releaseStream and the like need no answer from a live server
			log.Debug("rtmp player command ignored", "command", describeCommand(cmd))
		}
	}
}

// acceptConnect reads the connect command and accepts it.
func (p *Player) acceptConnect() error {
	e := errors.Template("rtmp.connect", errors.K.Invalid)
	cmd, err := p.s.readCommand()
	if err != nil {
		return err
	}
	if cmd.Name != "connect" || len(cmd.Arguments) < 1 {
		return e("reason", "expected connect", "command", describeCommand(cmd))
	}
	obj, ok := objectOrArray(cmd.Arguments[0])
	if !ok {
		return e("reason", "connect command without an object")
	}
	p.App, _ = obj.GetString("app")
	if p.TcURL, ok = obj.GetString("tcUrl"); !ok {
		p.TcURL, _ = obj.GetString("tcurl")
	}

	for _, msg := range []message.Message{
		&message.SetWindowAckSize{Value: windowAckSize},
		&message.SetPeerBandwidth{Value: windowAckSize, Type: 2},
		&message.SetChunkSize{Value: chunkSize},
		&message.CommandAMF0{
			ChunkStreamID: cmd.ChunkStreamID,
			Name:          "_result",
			CommandID:     cmd.CommandID,
			Arguments: []any{
				amf0.Object{
					{Key: "fmsVer", Value: "LNX 9,0,124,2"},
					{Key: "capabilities", Value: float64(31)},
				},
				amf0.Object{
					{Key: "level", Value: "status"},
					{Key: "code", Value: "NetConnection.Connect.Success"},
					{Key: "description", Value: "Connection succeeded."},
					{Key: "objectEncoding", Value: float64(encodingAMF0)},
				},
			},
		},
	} {
		if err = p.s.write(msg); err != nil {
			return err
		}
	}
	return nil
}

// Path returns what the player asked to play as one path: the application and the stream name joined with a slash,
// e.g. "live/stream". Clients disagree on where a play URL's path splits into the two. ffmpeg splits at the last
// slash, while gortmplib takes the stream name from the fragment and sends the whole path as the application. Path is
// the same either way.
func (p *Player) Path() string {
	app := strings.Trim(p.App, "/")
	name := strings.TrimPrefix(p.StreamName, "/")
	switch {
	case app == "":
		return name
	case name == "":
		return app
	}
	return app + "/" + name
}

// RemoteAddr returns the player's address.
func (p *Player) RemoteAddr() net.Addr {
	return p.s.nconn.RemoteAddr()
}

// Allow answers the play command with the stream's start: the stream-begin event and the Play.Reset, Play.Start and
// Data.Start statuses. Start calls it if the caller has not.
//
// Unlike gortmplib's server it sends no stream-is-recorded event. nginx-rtmp sends that only for recorded content,
// and players may treat a stream flagged with it as seekable.
func (p *Player) Allow() error {
	if p.allowed {
		return nil
	}
	p.allowed = true
	for _, msg := range []message.Message{
		&message.UserControlStreamBegin{StreamID: 1},
		statusMessage(p.play, "status", "NetStream.Play.Reset", "play reset"),
		statusMessage(p.play, "status", "NetStream.Play.Start", "play start"),
		statusMessage(p.play, "status", "NetStream.Data.Start", "data start"),
	} {
		if err := p.s.write(msg); err != nil {
			return errors.E("rtmp.Allow", errors.K.IO, err, "remote", p.RemoteAddr().String())
		}
	}
	return nil
}

// Reject answers the play command with an error status, such as PlayStreamNotFound, and closes the connection.
func (p *Player) Reject(code, description string) error {
	err := p.s.write(statusMessage(p.play, "error", code, description))
	_ = p.s.close()
	if err != nil {
		return errors.E("rtmp.Reject", errors.K.IO, err, "remote", p.RemoteAddr().String())
	}
	return nil
}

// Close closes the connection without answering the play command. A Player that was started belongs to its Conn
// instead.
func (p *Player) Close() error {
	return p.s.close()
}

// Start announces the tracks in params to the player: onMetaData with meta, then the H.264 and AAC sequence headers.
// params must have a video track. The returned Conn takes over the connection. Its first sample to go out is the next
// video keyframe written, and its read loop ends it with ErrStopped when the player stops the stream.
func (p *Player) Start(params fmp4.CodecParams, meta Metadata) (*Conn, error) {
	e := errors.Template("rtmp.Start", errors.K.IO, "remote", p.RemoteAddr().String())
	if params.Video == nil {
		_ = p.s.close()
		return nil, e(errors.K.Invalid, "reason", "a video track is required")
	}
	if err := p.Allow(); err != nil {
		_ = p.s.close()
		return nil, e(err)
	}

	c := newConn(endpoint{key: "player", name: p.RemoteAddr().String(), meta: meta, play: true}, params, p.s)
	if err := c.writeHeaders(); err != nil {
		_ = p.s.close()
		return nil, e(err, "reason", "failed to announce tracks")
	}
	go c.readLoop()
	log.Info("rtmp player started", "app", p.App, "player", c.stats.RemoteAddr,
		"video", params.Video.Codecs, "audio", audioCodecs(params.Audio))
	return c, nil
}

// statusMessage builds the onStatus answer to command cmd on the play stream.
func statusMessage(cmd *message.CommandAMF0, level, code, description string) *message.CommandAMF0 {
	return &message.CommandAMF0{
		ChunkStreamID:   statusCsID,
		MessageStreamID: publishStreamID,
		Name:            "onStatus",
		CommandID:       cmd.CommandID,
		Arguments: []any{
			nil,
			amf0.Object{
				{Key: "level", Value: level},
				{Key: "code", Value: code},
				{Key: "description", Value: description},
			},
		},
	}
}

// commandString returns argument i of cmd if it is a string.
func commandString(cmd *message.CommandAMF0, i int) (string, bool) {
	if len(cmd.Arguments) <= i {
		return "", false
	}
	s, ok := cmd.Arguments[i].(string)
	return s, ok
}
