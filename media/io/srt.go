package io

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/httputil"
	"github.com/eluv-io/errors-go"
)

// srtSettings holds the parsed SRT configuration for a source or sink URL together with the derived connection
// parameters. It knows how to establish connections in either caller (dial) or listener mode.
type srtSettings struct {
	config      srt.Config
	hostPort    string
	statsPeriod duration.Spec
	encrypted   bool
	modeListen  bool
	urlStr      string
}

// srtParse parses the SRT URL into the configuration and connection parameters used to dial or listen.
func srtParse(urlStr string) (srtSettings, error) {
	e := errors.Template("srtProto.Open", errors.K.IO, "url", urlStr)

	srtConfig := srt.DefaultConfig()
	hostPort, err := srtConfig.UnmarshalURL(srtSanitizeUrl(urlStr))
	if err != nil {
		return srtSettings{}, e(err)
	}

	// force `message` transmission method: https://github.com/Haivision/srt/blob/master/docs/API/API.md#transmission-method-message
	// ensures message boundaries of the sender are preserved
	srtConfig.MessageAPI = true

	u, err := url.Parse(urlStr)
	if err != nil {
		return srtSettings{}, e(err)
	}

	// enable SRT internal logging when requested via the "srt_log" query parameter; messages are forwarded to our
	// logger by the wrapped connection (see newWrappedConn).
	if topics := srtLogTopics(u.Query()); len(topics) > 0 {
		srtConfig.Logger = srt.NewLogger(topics)
	}

	return srtSettings{
		config:      srtConfig,
		hostPort:    hostPort,
		statsPeriod: httputil.DurationQuery(u.Query(), "stats_period", duration.Second, 0),
		encrypted:   srtConfig.Passphrase != "", // Passphrase was populated by UnmarshalURL above
		modeListen:  httputil.StringQuery(u.Query(), "mode", "") == "listener",
		urlStr:      urlStr,
	}, nil
}

// dial connects to a remote SRT server (caller mode) and returns the wrapped connection.
func (s srtSettings) dial() (srt.Conn, error) {
	e := errors.Template("srtProto.Open", errors.K.IO, "url", s.urlStr)
	log.Debug("srt connect", "url", s.urlStr)

	conn, err := srt.Dial("srt", s.hostPort, s.config)
	if err != nil {
		return nil, e(err)
	}
	return newWrappedConn(conn, nil, s.hostPort, s.statsPeriod, s.encrypted, s.config.Logger), nil
}

// listen binds an SRT listener to the URL's address (listener mode). The caller owns the returned listener and must
// close it; closing it also interrupts a pending accept (see accept), which is how shutdown unblocks a listener that is
// still waiting for a connection.
func (s srtSettings) listen() (srt.Listener, error) {
	e := errors.Template("srtProto.Open", errors.K.IO, "url", s.urlStr)

	listener, err := srt.Listen("srt", s.hostPort, s.config)
	if err != nil {
		return nil, e(err)
	}
	return listener, nil
}

// accept blocks on the given listener until a peer connects, then negotiates and wraps the connection. It returns
// promptly once the listener is closed (Accept2 then fails), which is how a pending accept is interrupted on shutdown.
// The listener is handed to the wrapped connection so that closing the connection also releases the listener. On error
// the caller is responsible for closing the listener.
func (s srtSettings) accept(listener srt.Listener) (srt.Conn, error) {
	e := errors.Template("srtProto.Open", errors.K.IO, "url", s.urlStr)

	log.Debug("srt listen - waiting for connection", "url", s.urlStr)

	req, err := listener.Accept2()
	if err != nil {
		log.Debug("failed to accept connection", "remote", e(err))
		return nil, e(err)
	}

	log.Debug("new connection",
		"host", s.hostPort,
		"remote", req.RemoteAddr(),
		"srt_version", req.Version(),
		"stream_id", req.StreamId())

	if s.config.Passphrase != "" {
		if err = req.SetPassphrase(s.config.Passphrase); err != nil {
			req.Reject(srt.REJX_UNAUTHORIZED)
			return nil, e(err, "reason", "invalid passphrase")
		}
	}

	conn, err := req.Accept()
	if err != nil {
		return nil, e(err)
	}

	log.Debug("new connection accepted",
		"host", s.hostPort,
		"remote", req.RemoteAddr(),
		"srt_version", req.Version(),
		"stream_id", req.StreamId())
	return newWrappedConn(conn, listener, s.hostPort, s.statsPeriod, s.encrypted, s.config.Logger), nil
}

func srtOpen(urlStr string) (connect func() (srt.Conn, error), modeListen bool, err error) {
	s, err := srtParse(urlStr)
	if err != nil {
		return nil, false, err
	}

	if !s.modeListen {
		// connect mode: connect to SRT server and pull the stream
		return s.dial, false, nil
	}

	// listen mode: each call binds a fresh listener, accepts one connection, and hands the listener to
	// the wrapped connection so it is released when the connection closes. On accept failure the listener
	// is closed here.
	return func() (srt.Conn, error) {
		listener, err := s.listen()
		if err != nil {
			return nil, err
		}
		conn, err := s.accept(listener)
		if err != nil {
			listener.Close()
			return nil, err
		}
		return conn, nil
	}, true, nil
}

// srtSanitizeUrl strips `+rtp` from the `srt+rtp://` URL prefix if present. gosrt only supports `srt://`.
func srtSanitizeUrl(str string) string {
	return strings.Replace(str, "srt+rtp://", "srt://", 1)
}

// srtLogTopics parses the comma-separated list of SRT logging topics from the "srt_log" query parameter (e.g.
// "connection,control,data,dial,handshake,listen,packet"). It returns nil if the parameter is absent or empty, in which
// case SRT internal logging stays disabled.
func srtLogTopics(query url.Values) []string {
	csv := httputil.StringQuery(query, "srt_log", "")
	if csv == "" {
		return nil
	}
	var topics []string
	for _, topic := range strings.Split(csv, ",") {
		if topic = strings.TrimSpace(topic); topic != "" {
			topics = append(topics, topic)
		}
	}
	return topics
}

type wrappedConn struct {
	srt.Conn
	listener  srt.Listener
	encrypted bool
	done      chan bool
	once      sync.Once
}

// ConnStats implements StatsReporter, exposing the connection's local and remote addresses and SRT
// protocol stats.
func (w *wrappedConn) ConnStats(details bool) ConnStats {
	cs := ConnStats{}
	if addr := w.RemoteAddr(); addr != nil {
		cs.RemoteAddr = addr.String()
	}
	if addr := w.LocalAddr(); addr != nil {
		cs.LocalAddr = addr.String()
	}
	srtStats := &SrtConnStats{Version: w.Version(), Encrypted: w.encrypted}
	if details {
		w.Stats(&srtStats.Statistics)
	}
	cs.SRT = srtStats
	return cs
}

func newWrappedConn(
	conn srt.Conn,
	listener srt.Listener,
	hostPort string,
	statsPeriod duration.Spec,
	encrypted bool,
	logger srt.Logger,
) srt.Conn {
	done := make(chan bool, 1)

	// forward SRT internal log messages to our logger until the connection is closed. We do not close the logger's
	// channel ourselves (SRT may still emit messages during teardown, and sending on a closed channel would panic);
	// the goroutine exits on done. It also exits if the channel is ever closed elsewhere, to avoid a tight loop
	// receiving zero values.
	if logger != nil {
		go func() {
			messages := logger.Listen()
			for {
				select {
				case m, ok := <-messages:
					if !ok {
						return
					}
					log.Info(m.Topic, "socket_id", m.SocketId, "file", m.File, "line", m.Line, "msg", m.Message)
				case <-done:
					return
				}
			}
		}()
	}

	if statsPeriod > 0 {
		go func() {
			remote := conn.RemoteAddr().String()
			version := conn.Version()
			streamId := conn.StreamId()
			stats := &srt.Statistics{}
			report := func() {
				conn.Stats(stats)
				res, _ := json.Marshal(stats)
				log.Debug("srt stats",
					"host", hostPort,
					"remote", remote,
					"srt_version", version,
					"stream_id", streamId,
					"stats", string(res))
			}
			ticker := time.NewTicker(statsPeriod.Duration())
			for {
				select {
				case <-ticker.C:
					report()
				case <-done:
					report()
					return
				}
			}
		}()
	}
	return &wrappedConn{
		Conn:      conn,
		listener:  listener,
		encrypted: encrypted,
		done:      done,
	}
}

func (w *wrappedConn) Close() error {
	w.once.Do(func() {
		close(w.done)
	})
	if w.listener != nil {
		w.listener.Close()
	}
	return w.Conn.Close()
}
