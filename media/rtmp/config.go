// Package rtmp publishes H.264 and AAC access units to an RTMP or RTMPS server, as a live stream contribution to
// services such as YouTube or Facebook.
//
// A Conn is one publishing session: it connects, announces the tracks, and writes access units with timestamps
// rebased to start at zero at the first video keyframe. It does not reconnect; the caller opens a new Conn when
// Done closes.
//
// The handshake, the control messages and the sequence headers go through gortmplib's message layer, driven here
// rather than through gortmplib's Client so that the control messages a server sends (pings, acknowledgements) can be
// answered from the read loop without racing the media writer, which gortmplib's writer does not guard against.
// Access units are framed by this package's own chunk writer instead, which writes them to the socket without an
// intermediate copy; see chunkwriter.go.
package rtmp

import (
	"crypto/tls"
	"net/url"
	"strings"
	"time"

	"github.com/eluv-io/errors-go"
)

const (
	// DefaultConnectTimeout bounds the TCP connect, TLS and RTMP handshakes together.
	DefaultConnectTimeout = 10 * time.Second
	// DefaultWriteTimeout bounds a single message write. A peer that stops reading fills the socket buffers within
	// seconds at video bitrates, so a write that takes this long means the connection is dead.
	DefaultWriteTimeout = 10 * time.Second

	defaultRtmpPort  = "1935"
	defaultRtmpsPort = "443"
)

// Config configures a publishing connection.
type Config struct {
	// URL is the server and application: rtmp://host[:port]/app[/instance] or rtmps://... . It must carry no user
	// info and no fragment; the stream key and the credentials have fields of their own.
	URL string
	// StreamKey is the stream name published under URL's application. Servers hand it out as a secret.
	StreamKey string
	// Username and Password authenticate with a server that requires Adobe-style RTMP authentication. Both or none.
	Username string
	Password string
	// TLS configures an rtmps connection. Nil uses the system roots; the server name defaults to URL's host.
	TLS *tls.Config

	ConnectTimeout time.Duration // 0 = DefaultConnectTimeout
	WriteTimeout   time.Duration // 0 = DefaultWriteTimeout

	// Meta is what the onMetaData message announces about the stream. Width and Height are filled from the video
	// track's SPS when zero. FrameRate is too, when the SPS timing yields a plausible rate; otherwise it is left out
	// of the message.
	Meta Metadata
}

// Metadata is the stream description sent in the onMetaData message. Services show these values in their ingest
// diagnostics; a missing or wrong value does not stop a stream but produces warnings there.
type Metadata struct {
	Width        int
	Height       int
	FrameRate    float64
	VideoBitrate int // bits per second, 0 = unknown
	AudioBitrate int // bits per second, 0 = unknown
	Encoder      string
}

// Validate checks the configuration without connecting.
func (c *Config) Validate() error {
	_, err := c.publishURL()
	return err
}

// Encrypted reports whether the connection uses TLS (an rtmps URL).
func (c *Config) Encrypted() bool {
	return strings.HasPrefix(strings.ToLower(c.URL), "rtmps://")
}

// Redacted returns the destination for logs: the URL with the stream key and any password replaced by asterisks.
func (c *Config) Redacted() string {
	s := c.URL
	if c.Username != "" {
		s = strings.Replace(s, "://", "://"+c.Username+":***@", 1)
	}
	if c.StreamKey != "" {
		s += "/***"
	}
	return s
}

func (c *Config) connectTimeout() time.Duration {
	if c.ConnectTimeout > 0 {
		return c.ConnectTimeout
	}
	return DefaultConnectTimeout
}

func (c *Config) writeTimeout() time.Duration {
	if c.WriteTimeout > 0 {
		return c.WriteTimeout
	}
	return DefaultWriteTimeout
}

// publishURL validates the configuration and returns the URL the session connects to: scheme, host with an
// explicit port, and the application path, with the stream key as the fragment and the credentials as user info.
// This is the form gortmplib's Client takes, kept so the two stay interchangeable.
func (c *Config) publishURL() (*url.URL, error) {
	e := errors.Template("rtmp.Config", errors.K.Invalid, "url", c.URL)

	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, e(err)
	}
	switch u.Scheme {
	case "rtmp", "rtmps":
	default:
		return nil, e("reason", "scheme must be rtmp or rtmps", "scheme", u.Scheme)
	}
	if u.Host == "" || u.Hostname() == "" {
		return nil, e("reason", "url has no host")
	}
	if u.User != nil {
		return nil, e("reason", "url must not carry user info - use username and password")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return nil, e("reason", "url must not carry a fragment - use stream_key")
	}
	if strings.Trim(u.Path, "/") == "" {
		return nil, e("reason", "url has no application path")
	}
	if c.StreamKey == "" {
		return nil, e("reason", "stream key is required")
	}
	if strings.ContainsAny(c.StreamKey, "/#?") {
		return nil, e("reason", "stream key must not contain '/', '#' or '?'")
	}
	if (c.Username == "") != (c.Password == "") {
		return nil, e("reason", "username and password must be set together")
	}
	if u.Port() == "" {
		port := defaultRtmpPort
		if u.Scheme == "rtmps" {
			port = defaultRtmpsPort
		}
		u.Host = u.Hostname() + ":" + port
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Fragment = c.StreamKey
	if c.Username != "" {
		u.User = url.UserPassword(c.Username, c.Password)
	}
	return u, nil
}
