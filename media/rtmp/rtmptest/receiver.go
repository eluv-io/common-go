// Package rtmptest provides an in-process RTMP server that accepts one publisher at a time and records what it
// receives, for tests of RTMP publishing code.
package rtmptest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/amf0"
	"github.com/bluenviron/gortmplib/pkg/codecs"
	"github.com/bluenviron/gortmplib/pkg/message"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"

	"github.com/eluv-io/errors-go"
)

// Option configures a Receiver.
type Option func(*Receiver)

// WithTLS makes the receiver an rtmps server.
func WithTLS(cfg *tls.Config) Option {
	return func(r *Receiver) { r.tlsCfg = cfg }
}

// WithAuth requires Adobe-style authentication with the given credentials.
func WithAuth(user, pass string) Option {
	return func(r *Receiver) { r.user, r.pass = user, pass }
}

// Stats is what the receiver has seen so far.
//
// The JSON tags are there for logging: rendered with %v the struct spells every sequence header and the whole first
// access unit out as decimal byte arrays, which buries the counters a reader is after. Log it as JSON instead, e.g.
// with jsonutil.Stringer. The media bytes are left out of that rendering entirely; the byte counts describe them.
type Stats struct {
	Connections int `json:"connections"` // publisher connections accepted
	Active      int `json:"active"`      // publisher connections currently open
	Disconnects int `json:"disconnects"` // publisher connections that ended

	// Of the latest publish.
	App       string         `json:"app,omitempty"`
	StreamKey string         `json:"stream_key,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"` // the onMetaData object, if one was received
	SPS       []byte         `json:"sps,omitempty"`
	PPS       []byte         `json:"pps,omitempty"`
	ASC       []byte         `json:"asc,omitempty"`

	VideoFrames   int           `json:"video_frames"`
	Keyframes     int           `json:"keyframes"`
	AudioFrames   int           `json:"audio_frames"`
	VideoBytes    int           `json:"video_bytes"`
	AudioBytes    int           `json:"audio_bytes"`
	FirstVideoDTS time.Duration `json:"first_video_dts"`
	LastVideoDTS  time.Duration `json:"last_video_dts"`
	FirstAudioPTS time.Duration `json:"first_audio_pts"`
	LastAudioPTS  time.Duration `json:"last_audio_pts"`
	// VideoMonotonic and AudioMonotonic report whether every timestamp received so far was at or after the previous
	// one of its track. Reset per connection.
	VideoMonotonic bool `json:"video_monotonic"`
	AudioMonotonic bool `json:"audio_monotonic"`
	// FirstNALUs are the NAL units of the first video access unit received on the latest connection. It is a whole
	// picture, so it stays out of the JSON rendering.
	FirstNALUs [][]byte `json:"-"`
}

// Receiver is the in-process server. Listen starts it; Close stops it.
type Receiver struct {
	ln     net.Listener
	tlsCfg *tls.Config
	user   string
	pass   string

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	stats  Stats
	closed bool
}

// Listen starts a receiver on addr (e.g. "127.0.0.1:0").
func Listen(addr string, opts ...Option) (*Receiver, error) {
	r := &Receiver{conns: make(map[net.Conn]struct{})}
	for _, opt := range opts {
		opt(r)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.E("rtmptest.Listen", errors.K.IO, err, "addr", addr)
	}
	if r.tlsCfg != nil {
		ln = tls.NewListener(ln, r.tlsCfg)
	}
	r.ln = ln
	go r.accept()
	return r, nil
}

// Addr returns the listening address.
func (r *Receiver) Addr() net.Addr {
	return r.ln.Addr()
}

// URL returns the rtmp or rtmps URL of the receiver's "live" application, without a stream key.
func (r *Receiver) URL() string {
	scheme := "rtmp"
	if r.tlsCfg != nil {
		scheme = "rtmps"
	}
	return fmt.Sprintf("%s://%s/live", scheme, r.ln.Addr().String())
}

// Close stops listening and closes every connection.
func (r *Receiver) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	err := r.ln.Close()
	r.Kill()
	return err
}

// Kill closes every publisher connection while keeping the listener, to simulate a server-side drop.
func (r *Receiver) Kill() {
	r.mu.Lock()
	conns := make([]net.Conn, 0, len(r.conns))
	for c := range r.conns {
		conns = append(conns, c)
	}
	r.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// Stats returns a snapshot.
func (r *Receiver) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// WaitFor polls until cond holds for the stats or timeout elapses, and reports which.
func (r *Receiver) WaitFor(cond func(Stats) bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if cond(r.Stats()) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *Receiver) accept() {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			return
		}
		go r.handle(conn)
	}
}

func (r *Receiver) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.conns[conn] = struct{}{}
	r.mu.Unlock()

	err := r.serve(conn)

	r.mu.Lock()
	delete(r.conns, conn)
	if r.stats.Active > 0 {
		r.stats.Active--
		r.stats.Disconnects++
	}
	r.mu.Unlock()
	if err != nil {
		log.Debug("rtmptest: connection ended", "remote", conn.RemoteAddr(), "err", err)
	}
}

func (r *Receiver) serve(conn net.Conn) error {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	sc := &gortmplib.ServerConn{RW: conn}
	if err := sc.Initialize(); err != nil {
		return err
	}
	var err error
	if r.user != "" {
		err = sc.AcceptConnIfCredentialsMatch(r.user, r.pass)
	} else {
		err = sc.AcceptConn()
	}
	if err != nil {
		return err
	}
	if !sc.Publish {
		return sc.RejectAction()
	}

	r.mu.Lock()
	r.stats.Connections++
	r.stats.Active++
	r.stats.App = sc.URL.Path
	r.stats.StreamKey = ""
	if sc.URL.Path != "" {
		if i := lastSlash(sc.URL.Path); i >= 0 {
			r.stats.App = sc.URL.Path[:i]
			r.stats.StreamKey = sc.URL.Path[i+1:]
		}
	}
	r.resetStreamLocked()
	r.mu.Unlock()

	// The reader accepts the publish action itself when it is handed the ServerConn directly; behind the tap that
	// records onMetaData it cannot recognize it, so accept here. The publisher is blocked until this is sent.
	if err = sc.AcceptAction(); err != nil {
		return err
	}
	tap := &tapConn{Conn: sc, r: r}
	rd := &gortmplib.Reader{Conn: tap}
	if err = rd.Initialize(); err != nil {
		return err
	}
	for _, track := range rd.Tracks() {
		switch codec := track.Codec.(type) {
		case *codecs.H264:
			r.mu.Lock()
			r.stats.SPS, r.stats.PPS = codec.SPS, codec.PPS
			r.mu.Unlock()
			rd.OnDataH264(track, r.onVideo)
		case *codecs.MPEG4Audio:
			asc, _ := codec.Config.Marshal()
			r.mu.Lock()
			r.stats.ASC = asc
			r.mu.Unlock()
			rd.OnDataMPEG4Audio(track, r.onAudio)
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	for {
		if err = rd.Read(); err != nil {
			return err
		}
	}
}

func (r *Receiver) resetStreamLocked() {
	r.stats.Metadata = nil
	r.stats.SPS, r.stats.PPS, r.stats.ASC = nil, nil, nil
	r.stats.VideoFrames, r.stats.Keyframes, r.stats.AudioFrames = 0, 0, 0
	r.stats.VideoBytes, r.stats.AudioBytes = 0, 0
	r.stats.FirstVideoDTS, r.stats.LastVideoDTS, r.stats.FirstAudioPTS, r.stats.LastAudioPTS = 0, 0, 0, 0
	r.stats.VideoMonotonic, r.stats.AudioMonotonic = true, true
	r.stats.FirstNALUs = nil
}

func (r *Receiver) onVideo(pts, dts time.Duration, au [][]byte) {
	if isParameterSets(au) {
		// the reader reports the sequence header as an access unit of SPS and PPS; not a frame
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stats.VideoFrames == 0 {
		r.stats.FirstVideoDTS = dts
		r.stats.FirstNALUs = au
	} else if dts < r.stats.LastVideoDTS {
		r.stats.VideoMonotonic = false
	}
	r.stats.LastVideoDTS = dts
	r.stats.VideoFrames++
	if h264.IsRandomAccess(au) {
		r.stats.Keyframes++
	}
	for _, nalu := range au {
		r.stats.VideoBytes += len(nalu)
	}
	_ = pts
}

func (r *Receiver) onAudio(pts time.Duration, au []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stats.AudioFrames == 0 {
		r.stats.FirstAudioPTS = pts
	} else if pts < r.stats.LastAudioPTS {
		r.stats.AudioMonotonic = false
	}
	r.stats.LastAudioPTS = pts
	r.stats.AudioFrames++
	r.stats.AudioBytes += len(au)
}

func isParameterSets(au [][]byte) bool {
	if len(au) == 0 {
		return false
	}
	for _, nalu := range au {
		if len(nalu) == 0 {
			return false
		}
		switch h264.NALUType(nalu[0] & 0x1F) {
		case h264.NALUTypeSPS, h264.NALUTypePPS:
		default:
			return false
		}
	}
	return true
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// tapConn records the onMetaData message on its way to the reader.
type tapConn struct {
	gortmplib.Conn
	r *Receiver
}

func (t *tapConn) Read() (message.Message, error) {
	msg, err := t.Conn.Read()
	if err != nil {
		return nil, err
	}
	if data, ok := msg.(*message.DataAMF0); ok {
		t.r.recordMetadata(data)
	}
	return msg, nil
}

func (r *Receiver) recordMetadata(data *message.DataAMF0) {
	for i, item := range data.Payload {
		if name, ok := item.(string); ok && name == "onMetaData" && i+1 < len(data.Payload) {
			var obj amf0.Object
			switch v := data.Payload[i+1].(type) {
			case amf0.Object:
				obj = v
			case amf0.ECMAArray:
				obj = amf0.Object(v)
			default:
				return
			}
			meta := make(map[string]any, len(obj))
			for _, entry := range obj {
				meta[entry.Key] = entry.Value
			}
			r.mu.Lock()
			r.stats.Metadata = meta
			r.mu.Unlock()
			return
		}
	}
}

// SelfSignedCert generates a certificate for the given hosts (names or IPs) valid for a day, for rtmps tests. It
// returns the server certificate and a pool that trusts it.
func SelfSignedCert(hosts ...string) (tls.Certificate, *x509.CertPool, error) {
	e := errors.Template("rtmptest.SelfSignedCert", errors.K.Invalid)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, e(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "rtmptest"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, e(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, e(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}, pool, nil
}
