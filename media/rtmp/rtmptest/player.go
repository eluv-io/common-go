package rtmptest

import (
	"context"
	"crypto/tls"
	"net/url"
	"time"

	"github.com/bluenviron/gortmplib"
	"github.com/bluenviron/gortmplib/pkg/message"

	"github.com/eluv-io/errors-go"
)

// Player plays a stream from an RTMP server and records what it receives, for tests of RTMP serving code. Play
// starts it; Close stops it. Its Stats count one connection.
type Player struct {
	recorder

	client *gortmplib.Client
	done   chan struct{}
	err    error
}

// Play connects to rawURL, rtmp://host:port/app/stream or rtmps://..., and plays the stream. It returns once the
// server has accepted the play and announced the tracks, and records the samples in the background. tlsCfg is used
// for rtmps and may be nil.
func Play(ctx context.Context, rawURL string, tlsCfg *tls.Config) (*Player, error) {
	e := errors.Template("rtmptest.Play", errors.K.IO, "url", rawURL)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, e(errors.K.Invalid, err)
	}
	// gortmplib's client plays the stream named by the fragment. Split the path at its last slash instead, the way
	// ffmpeg and most players do, so the server sees the application and stream name such players send.
	cu := *u
	if i := lastSlash(cu.Path); cu.Fragment == "" && i > 0 {
		cu.Path, cu.Fragment = u.Path[:i], u.Path[i+1:]
		cu.RawPath = ""
	}
	p := &Player{
		client: &gortmplib.Client{URL: &cu, TLSConfig: tlsCfg},
		done:   make(chan struct{}),
	}
	if err = p.client.Initialize(ctx); err != nil {
		return nil, e(err)
	}
	p.mu.Lock()
	p.stats.Connections, p.stats.Active = 1, 1
	p.stats.App = u.Path
	if i := lastSlash(u.Path); i >= 0 {
		p.stats.App, p.stats.StreamKey = u.Path[:i], u.Path[i+1:]
	}
	p.resetStreamLocked()
	p.mu.Unlock()

	// the reader waits for the tracks; a server that accepts the play but never announces them must not hang the test
	nconn := p.client.NetConn()
	if deadline, ok := ctx.Deadline(); ok {
		_ = nconn.SetReadDeadline(deadline)
	}
	rd, err := p.newReader(p.client)
	if err != nil {
		p.client.Close()
		return nil, e(err, "reason", "no tracks announced")
	}
	_ = nconn.SetReadDeadline(time.Time{})
	go p.read(rd)
	return p, nil
}

func (p *Player) read(rd *gortmplib.Reader) {
	defer close(p.done)
	var err error
	for err == nil {
		err = rd.Read()
	}
	p.mu.Lock()
	p.err = err
	p.stats.Active = 0
	p.stats.Disconnects = 1
	p.mu.Unlock()
}

// Done closes when the connection has ended.
func (p *Player) Done() <-chan struct{} {
	return p.done
}

// Err returns why the connection ended, nil while it is open.
func (p *Player) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Stop asks the server to end the stream with deleteStream, as a player does when its user stops it, then closes the
// connection.
func (p *Player) Stop() error {
	err := p.client.Write(&message.CommandAMF0{
		ChunkStreamID: 3,
		Name:          "deleteStream",
		Arguments:     []any{nil, float64(1)},
	})
	p.Close()
	return err
}

// Close ends the connection and waits for the reader to stop.
func (p *Player) Close() {
	p.client.Close()
	<-p.done
}
