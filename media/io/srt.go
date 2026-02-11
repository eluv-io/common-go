package io

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"

	"github.com/eluv-io/errors-go"
)

func srtOpen(urlStr string) (connect func() (srt.Conn, error), modeListen bool, err error) {
	e := errors.Template("srtProto.Open", errors.K.IO, "url", urlStr)

	srtConfig := srt.DefaultConfig()
	hostPort, err := srtConfig.UnmarshalURL(srtSanitizeUrl(urlStr))
	if err != nil {
		return nil, false, e(err)
	}

	// force `message` transmission method: https://github.com/Haivision/srt/blob/master/docs/API/API.md#transmission-method-message
	// ensures message boundaries of the sender are preserved
	srtConfig.MessageAPI = true

	if false {
		srtConfig.Logger = srt.NewLogger(strings.Split("connection|control|data|dial|handshake|listen|packet", "|"))
		go func() {
			for m := range srtConfig.Logger.Listen() {
				log.Info(m.Topic, "socket_id", m.SocketId, "file", m.File, "line", m.Line, "msg", m.Message)
			}
		}()
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, false, e(err)
	}

	// PENDING: check mode query param for "listen"/"listener"?
	if !strings.Contains(urlStr, "listen") {
		return func() (srt.Conn, error) {
			log.Debug("srt connect", "url", urlStr)

			// connect mode: connect to SRT server and pull the stream
			conn, err := srt.Dial("srt", hostPort, srtConfig)
			if err != nil {
				return nil, e(err)
			}
			return conn, nil
		}, false, nil
	}

	val := u.Query().Get("stats_period")
	statsPeriod, err := strconv.Atoi(val)
	if err != nil {
		log.Warn("srt init - invalid stats_period", "stats_period", val)
	}

	return func() (conn srt.Conn, err error) {
		// listen mode: act as SRT server and accept incoming connections
		listener, err := srt.Listen("srt", hostPort, srtConfig)
		if err != nil {
			return nil, e(err)
		}

		defer func() {
			if err != nil {
				listener.Close()
			}
		}()

		log.Debug("srt listen - waiting for connection", "url", urlStr)

		req, err := listener.Accept2()
		if err != nil {
			log.Debug("failed to accept connection", "remote", e(err))
			return nil, e(err)
		}

		log.Debug("new connection", "remote", req.RemoteAddr(), "srt_version", req.Version, "stream_id", req.StreamId())

		if srtConfig.Passphrase != "" {
			err = req.SetPassphrase(srtConfig.Passphrase)
			if err != nil {
				req.Reject(srt.REJX_UNAUTHORIZED)
				return nil, e(err, "reason", "invalid passphrase")
			}
		}

		// accept the connection
		conn, err = req.Accept()
		if err != nil {
			return nil, e(err)
		}

		remote := req.RemoteAddr()
		version := req.Version()
		streamId := req.StreamId()
		done := make(chan bool, 1)
		if statsPeriod > 0 {
			go func() {
				stats := &srt.Statistics{}
				report := func() {
					conn.Stats(stats)
					res, _ := json.Marshal(stats)
					log.Debug("srt stats", "remote", remote, "srt_version", version, "stream_id", streamId, "stats", string(res))
				}
				ticker := time.NewTicker(time.Second * time.Duration(statsPeriod))
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

		log.Debug("new connection accepted", "remote", remote, "srt_version", version, "stream_id", streamId)
		return &wrappedConn{
			Conn:     conn,
			listener: listener,
			done:     done,
		}, nil
	}, true, nil
}

// srtSanitizeUrl strips `+rtp` from the `srt+rtp://` URL prefix if present. gosrt only supports `srt://`.
func srtSanitizeUrl(str string) string {
	return strings.Replace(str, "srt+rtp://", "srt://", 1)
}

type wrappedConn struct {
	srt.Conn
	listener srt.Listener
	done     chan bool
	once     sync.Once
}

func (w *wrappedConn) Close() error {
	w.once.Do(func() {
		close(w.done)
	})
	w.listener.Close()
	return w.Conn.Close()
}
