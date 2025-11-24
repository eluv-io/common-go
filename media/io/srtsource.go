package io

import (
	"io"
	"net/url"
	"strings"

	srt "github.com/datarhei/gosrt"

	"github.com/eluv-io/errors-go"
)

func NewSrtSource(url *url.URL) PacketSource {
	return &srtSource{url: url, urlStr: url.String()}
}

type srtSource struct {
	url    *url.URL
	urlStr string
}

func (s *srtSource) URL() *url.URL {
	return s.url
}

func (s *srtSource) Name() string {
	return s.urlStr
}

func (s *srtSource) Open() (reader io.ReadCloser, err error) {
	connect, err := srtOpen(s.urlStr)
	if err != nil {
		return nil, err
	}
	return connect()
}

func srtOpen(urlStr string) (connect func() (srt.Conn, error), err error) {
	e := errors.Template("srtProto.Open", errors.K.IO, "url", urlStr)

	srtConfig := srt.DefaultConfig()
	hostPort, err := srtConfig.UnmarshalURL(urlStr)
	if err != nil {
		return nil, e(err)
	}

	// force `message` transmission method: https://github.com/Haivision/srt/blob/master/docs/API/API.md#transmission-method-message
	// ensures message boundaries of the sender are preserved
	srtConfig.MessageAPI = true

	if !strings.Contains(urlStr, "listen") {
		return func() (srt.Conn, error) {
			// connect mode: connect to SRT server and pull the stream
			conn, err := srt.Dial("srt", hostPort, srtConfig)
			if err != nil {
				return nil, e(err)
			}
			return conn, nil
		}, nil
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

		req, err := listener.Accept2()
		if err != nil {
			return nil, e(err)
		}

		streamId := req.StreamId()
		log.Debug("new connection", "remote", req.RemoteAddr(), "srt_version", req.Version(), "stream_id", streamId)

		// if req.Version() > 4 && strings.Contains(streamId, "subscribe") {
		// 	req.Reject(srt.REJX_BAD_MODE)
		// 	return nil, e("reason", "accepting only publish (push) connections",
		// 		"remote", req.RemoteAddr(),
		// 		"srt_version", req.Version(),
		// 		"stream_id", streamId)
		// }

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

		log.Debug("new connection accepted", "remote", req.RemoteAddr(), "srt_version", req.Version(), "stream_id", streamId)
		return &wrappedConn{
			Conn:     conn,
			listener: listener,
		}, nil
	}, nil
}

type wrappedConn struct {
	srt.Conn
	listener srt.Listener
}

func (w *wrappedConn) Close() error {
	w.listener.Close()
	return w.Conn.Close()
}
