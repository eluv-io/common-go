package rtmp

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/bytecounter"
	"github.com/bluenviron/gortmplib/pkg/handshake"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/google/uuid"

	"github.com/eluv-io/errors-go"
)

// Protocol constants of the publish handshake, as RTMP 1.0 and gortmplib's client use them.
const (
	windowAckSize   = 2500000
	chunkSize       = 65536
	commandCsID     = 3
	streamCsID      = 4
	publishStreamID = 0x1000000
	encodingAMF0    = 0

	cmdConnect       = 1
	cmdReleaseStream = 2
	cmdFCPublish     = 3
	cmdCreateStream  = 4
	cmdPublish       = 5
)

// errNeedAuth is the internal signal that the server demands Adobe authentication and the handshake has to be redone
// on a fresh connection with the next authentication step.
var errNeedAuth = errors.Str("rtmp: authentication step required")

// session is an established RTMP publishing connection at the message level: the handshake and the connect/publish
// exchange are done, and media messages can be written. All writes go through write, which serializes them: the
// read loop answers the server's pings and acknowledgements on the same message writer the media goes out on.
type session struct {
	nconn        net.Conn
	bc           *bytecounter.ReadWriter
	r            *message.Reader
	w            *message.Writer
	cw           *chunkWriter
	writeTimeout time.Duration

	wmu sync.Mutex
}

// newSession wraps an established connection in the message and chunk writers. The handshake is the caller's job;
// tests and benchmarks use this to build a session over a pipe or a discarding connection.
func newSession(nconn net.Conn, writeTimeout time.Duration) *session {
	s := &session{nconn: nconn, writeTimeout: writeTimeout}
	s.bc = bytecounter.NewReadWriter(nconn)
	s.w = message.NewWriter(s.bc, s.bc.Writer, false)
	s.r = message.NewReader(s.bc, s.bc.Reader, func(count uint32) error {
		return s.write(&message.Acknowledge{Value: count})
	})
	// Both writers buffer their output and flush it before releasing wmu, so whole messages reach the byte counter
	// in lock order and the counter sees everything either of them sends.
	s.cw = newChunkWriter(s.bc.Writer)
	return s
}

// dialSession connects to u and completes the publish handshake, redoing it with the next authentication step when
// the server asks for one. ctx bounds the whole exchange.
func dialSession(ctx context.Context, u *url.URL, tlsCfg *tls.Config, writeTimeout time.Duration) (*session, error) {
	var salt, challenge string
	for authState := 0; ; authState++ {
		s, err := connectSession(ctx, u, tlsCfg, writeTimeout, authState, salt, challenge)
		if err == nil {
			return s, nil
		}
		var need *authRequest
		if errors.As(err, &need) && authState < 2 {
			salt, challenge = need.salt, need.challenge
			continue
		}
		return nil, err
	}
}

// authRequest carries the server's authentication parameters between two attempts of dialSession.
type authRequest struct {
	salt, challenge string
}

func (a *authRequest) Error() string { return errNeedAuth.Error() }

func connectSession(
	ctx context.Context,
	u *url.URL,
	tlsCfg *tls.Config,
	writeTimeout time.Duration,
	authState int,
	salt, challenge string,
) (*session, error) {
	e := errors.Template("rtmp.dial", errors.K.IO, "host", u.Host)

	nconn, err := (&net.Dialer{}).DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, e(err, "reason", "tcp connect failed")
	}
	if u.Scheme == "rtmps" {
		cfg := tlsCfg
		if cfg == nil {
			cfg = &tls.Config{}
		} else {
			cfg = cfg.Clone()
		}
		if cfg.ServerName == "" {
			cfg.ServerName = u.Hostname()
		}
		tconn := tls.Client(nconn, cfg)
		if err = tconn.HandshakeContext(ctx); err != nil {
			_ = nconn.Close()
			return nil, e(err, "reason", "tls handshake failed")
		}
		nconn = tconn
	}

	// The RTMP handshake and the connect/publish exchange run under the context's deadline; a server that accepts
	// the TCP connection but never answers must not hang the caller. Reads never time out afterwards: a publishing
	// connection receives nothing for long stretches, and the write timeout is what catches a dead peer.
	if deadline, ok := ctx.Deadline(); ok {
		_ = nconn.SetDeadline(deadline)
	}
	s := newSession(nconn, writeTimeout)
	if _, _, err = handshake.DoClient(s.bc, false, false); err != nil {
		_ = nconn.Close()
		return nil, e(err, "reason", "rtmp handshake failed")
	}

	if err = s.publish(u, authState, salt, challenge); err != nil {
		_ = nconn.Close()
		var need *authRequest
		if errors.As(err, &need) {
			return nil, err
		}
		return nil, e(err, "reason", "rtmp publish handshake failed")
	}
	_ = nconn.SetDeadline(time.Time{})
	return s, nil
}

// publish runs the connect, createStream and publish commands.
func (s *session) publish(u *url.URL, authState int, salt, challenge string) error {
	if err := s.write(&message.SetWindowAckSize{Value: windowAckSize}); err != nil {
		return err
	}
	if err := s.write(&message.SetPeerBandwidth{Value: windowAckSize, Type: 2}); err != nil {
		return err
	}
	if err := s.write(&message.SetChunkSize{Value: chunkSize}); err != nil {
		return err
	}

	tcURL, app, streamKey := splitURL(u)
	switch authState {
	case 1:
		q := "?authmod=adobe&user=" + u.User.Username()
		app += q
		tcURL += q
	case 2:
		user := u.User.Username()
		pass, _ := u.User.Password()
		clientChallenge := strings.ReplaceAll(uuid.New().String(), "-", "")
		q := fmt.Sprintf("?authmod=adobe&user=%s&challenge=%s&response=%s",
			user, clientChallenge, authResponse(user, pass, salt, challenge, clientChallenge))
		app += q
		tcURL += q
	}

	err := s.write(&message.CommandAMF0{
		ChunkStreamID: commandCsID,
		Name:          "connect",
		CommandID:     cmdConnect,
		Arguments: []any{amf0.Object{
			{Key: "app", Value: app},
			{Key: "flashVer", Value: "FMLE/3.0 (compatible; eluvio)"},
			{Key: "tcUrl", Value: tcURL},
			{Key: "type", Value: "nonprivate"},
			{Key: "objectEncoding", Value: float64(encodingAMF0)},
		}},
	})
	if err != nil {
		return err
	}
	res, err := s.readCommandResult(cmdConnect)
	if err != nil {
		return err
	}
	if err = s.checkConnectResult(res, u); err != nil {
		return err
	}

	for _, cmd := range []struct {
		name string
		id   int
	}{{"releaseStream", cmdReleaseStream}, {"FCPublish", cmdFCPublish}} {
		err = s.write(&message.CommandAMF0{
			ChunkStreamID: commandCsID,
			Name:          cmd.name,
			CommandID:     cmd.id,
			Arguments:     []any{nil, streamKey},
		})
		if err != nil {
			return err
		}
	}
	err = s.write(&message.CommandAMF0{
		ChunkStreamID: commandCsID,
		Name:          "createStream",
		CommandID:     cmdCreateStream,
		Arguments:     []any{nil},
	})
	if err != nil {
		return err
	}
	res, err = s.readCommandResult(cmdCreateStream)
	if err != nil {
		return err
	}
	if res.Name != "_result" || !resultIsStreamID(res) {
		return errors.E("rtmp.createStream", errors.K.IO, "reason", "unexpected result", "result", describeCommand(res))
	}

	err = s.write(&message.CommandAMF0{
		ChunkStreamID:   streamCsID,
		MessageStreamID: publishStreamID,
		Name:            "publish",
		CommandID:       cmdPublish,
		Arguments:       []any{nil, streamKey, "live"},
	})
	if err != nil {
		return err
	}
	res, err = s.waitOnStatus(cmdPublish)
	if err != nil {
		return err
	}
	if res.Name != "onStatus" || !statusIsOK(res) {
		return errors.E("rtmp.publish", errors.K.Permission, "reason", "publish refused", "status", describeCommand(res))
	}
	return nil
}

// checkConnectResult interprets the server's answer to connect: success, an Adobe authentication demand (returned
// as *authRequest so the caller retries with the next step), or a refusal.
func (s *session) checkConnectResult(res *message.CommandAMF0, u *url.URL) error {
	e := errors.Template("rtmp.connect", errors.K.Permission)
	switch res.Name {
	case "_result":
		return nil
	case "_error":
		if len(res.Arguments) < 2 {
			return e("reason", "connect refused", "result", describeCommand(res))
		}
		obj, ok := objectOrArray(res.Arguments[1])
		if !ok {
			return e("reason", "connect refused", "result", describeCommand(res))
		}
		desc, _ := obj.GetString("description")
		switch {
		case desc == "code=403 need auth; authmod=adobe":
			if u.User == nil {
				return e("reason", "server requires authentication - set username and password")
			}
			return &authRequest{}
		case strings.HasPrefix(desc, "authmod=adobe ?"):
			vals := queryDecode(desc[len("authmod=adobe ?"):])
			if vals["reason"] == "needauth" && vals["salt"] != "" && vals["challenge"] != "" {
				return &authRequest{salt: vals["salt"], challenge: vals["challenge"]}
			}
			return e("reason", "authentication failed", "description", desc)
		default:
			return e("reason", "connect refused", "description", desc)
		}
	default:
		return e("reason", "unexpected connect result", "result", describeCommand(res))
	}
}

// write sends one message through gortmplib's writer, bounded by the write timeout: the handshake commands, the
// sequence headers and the replies the read loop owes the server.
//
// Media messages go through writeVideo and writeAudio instead, and once one of those has written on a chunk stream
// this path must not touch it again: the two writers keep their own idea of what the receiver was told about each
// chunk stream, and a message from the wrong one would be decoded against the other's state.
func (s *session) write(msg message.Message) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	switch msg.(type) {
	case *message.Video, *message.Audio:
		if s.cw.started() {
			return errors.NoTrace("rtmp.write", errors.K.Invalid,
				"reason", "media message on the message writer after the chunk writer took over the stream")
		}
	}
	if s.writeTimeout > 0 {
		_ = s.nconn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	return s.w.Write(msg)
}

// writeVideo sends one H.264 access unit, and writeAudio one AAC frame, on the media chunk streams. They hold the
// same lock as write, so a control message the read loop answers lands between two media messages, never inside one.
func (s *session) writeVideo(nalus [][]byte, ts, cts uint32, keyframe bool) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.writeTimeout > 0 {
		_ = s.nconn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	return s.cw.writeVideo(nalus, ts, cts, keyframe)
}

func (s *session) writeAudio(au []byte, ts uint32) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	if s.writeTimeout > 0 {
		_ = s.nconn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	return s.cw.writeAudio(au, ts)
}

// read returns the next message from the server, after answering the control messages that need no caller: a ping
// request gets its response, an acknowledgement updates the writer's view of what the server has received.
func (s *session) read() (message.Message, error) {
	msg, err := s.r.Read()
	if err != nil {
		return nil, err
	}
	switch m := msg.(type) {
	case *message.Acknowledge:
		s.wmu.Lock()
		s.w.SetAcknowledgeValue(m.Value)
		s.wmu.Unlock()
	case *message.UserControlPingRequest:
		if err = s.write(&message.UserControlPingResponse{ServerTime: m.ServerTime}); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

// readCommandResult reads until the result or error of command commandID arrives.
func (s *session) readCommandResult(commandID int) (*message.CommandAMF0, error) {
	for {
		msg, err := s.read()
		if err != nil {
			return nil, err
		}
		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID || (cmd.CommandID == 0 && (cmd.Name == "_result" || cmd.Name == "_error")) {
				return cmd, nil
			}
		}
	}
}

// waitOnStatus reads until the onStatus for command commandID arrives.
func (s *session) waitOnStatus(commandID int) (*message.CommandAMF0, error) {
	for {
		msg, err := s.read()
		if err != nil {
			return nil, err
		}
		if cmd, ok := msg.(*message.CommandAMF0); ok {
			if cmd.CommandID == commandID || (cmd.CommandID == 0 && cmd.Name == "onStatus") {
				return cmd, nil
			}
		}
	}
}

func (s *session) close() error {
	return s.nconn.Close()
}

// splitURL returns the tcUrl, application and stream key of a publish URL, whose fragment is the stream key.
func splitURL(u *url.URL) (tcURL, app, streamKey string) {
	nu := *u
	streamKey, nu.Fragment = nu.Fragment, ""
	nu.RawFragment = ""
	nu.User = nil
	tcURL = nu.String()
	app = strings.TrimPrefix(nu.RequestURI(), "/")
	return tcURL, app, streamKey
}

// authResponse computes the Adobe authentication response, as FMLE and nginx-rtmp do.
func authResponse(user, pass, salt, challenge, clientChallenge string) string {
	h := md5.New()
	h.Write([]byte(user))
	h.Write([]byte(salt))
	h.Write([]byte(pass))
	str := base64.StdEncoding.EncodeToString(h.Sum(nil))

	h = md5.New()
	h.Write([]byte(str))
	h.Write([]byte(challenge))
	h.Write([]byte(clientChallenge))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// queryDecode splits "k=v&k2=v2" without URL-decoding, since the values in RTMP auth descriptions are not encoded.
func queryDecode(enc string) map[string]string {
	vals := make(map[string]string)
	for _, kv := range strings.Split(enc, "&") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			vals[k] = v
		}
	}
	return vals
}

func objectOrArray(in any) (amf0.Object, bool) {
	switch v := in.(type) {
	case amf0.Object:
		return v, true
	case amf0.ECMAArray:
		return amf0.Object(v), true
	}
	return nil, false
}

// statusIsOK reports whether an onStatus command carries level "status" rather than "error".
func statusIsOK(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}
	obj, ok := objectOrArray(res.Arguments[1])
	if !ok {
		return false
	}
	level, _ := obj.GetString("level")
	return level == "status"
}

// resultIsStreamID reports whether a createStream result carries the stream id 1 gortmplib and nginx answer with.
func resultIsStreamID(res *message.CommandAMF0) bool {
	if len(res.Arguments) < 2 {
		return false
	}
	v, ok := res.Arguments[1].(float64)
	return ok && v == 1
}

// describeCommand renders a command for an error message: its name and the code and description of its status
// object, if any.
func describeCommand(cmd *message.CommandAMF0) string {
	desc := cmd.Name
	if len(cmd.Arguments) >= 2 {
		if obj, ok := objectOrArray(cmd.Arguments[1]); ok {
			if code, ok := obj.GetString("code"); ok {
				desc += " " + code
			}
			if d, ok := obj.GetString("description"); ok {
				desc += ": " + d
			}
		}
	}
	return desc
}
