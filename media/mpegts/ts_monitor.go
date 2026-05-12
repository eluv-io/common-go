package mpegts

import (
	"context"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/utc-go"
)

type TsMonitor interface {
	Start(stream string)
	Stop()
	SignalPacket(size int)  // a packet of the given size has been received/sent
	SignalPart(hash string) // serving a new part has started
}

// ---------------------------------------------------------------------------------------------------------------------

func NewTsMonitor() TsMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &tsMonitor{
		stats:       newStats(),
		chanPackets: make(chan int, 10),
		chanParts:   make(chan string, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
}

type tsMonitor struct {
	stats       stats
	chanPackets chan int
	chanParts   chan string
	ctx         context.Context
	cancel      context.CancelFunc
}

func (s *tsMonitor) Start(stream string) {
	s.stats.Stream = stream
	go s.run()
}

func (s *tsMonitor) Stop() {
	s.cancel()
}

func (s *tsMonitor) SignalPacket(size int) {
	select {
	case s.chanPackets <- size:
	case <-s.ctx.Done():
	}
}

func (s *tsMonitor) SignalPart(hash string) {
	select {
	case s.chanParts <- hash:
	default: // don't block
	}
}

func (s *tsMonitor) run() {
	log.Info("tsMonitor: stream started", "stream", s.stats.Stream)
	wait := 1 * time.Second
	ticker := time.NewTicker(time.Minute) // initially wait 1 minute before notifying stream stalls
	for {
		select {
		case <-ticker.C:
			log.Info("tsMonitor: stream stalled", s.stats.Fields()...)
			wait = wait * 2
			ticker.Reset(wait)
		case hash := <-s.chanParts:
			s.stats.CurrentPart = hash
			log.Debug("tsMonitor: new part", s.stats.Fields()...)
		case size := <-s.chanPackets:
			wait = time.Second
			ticker.Reset(wait)
			s.stats.Update(size)
		case <-s.ctx.Done():
			log.Info("tsMonitor: stream stopped", "stream", s.stats.Stream, "current_part", s.stats.CurrentPart)
			return
		}
	}
}

func newStats() stats {
	now := utc.Now()
	return stats{
		Start: now,
		Last:  now,
	}
}

type stats struct {
	Stream      string  `json:"stream"`
	Start       utc.UTC `json:"start"`
	Last        utc.UTC `json:"last"`
	Packets     uint64  `json:"packets"`
	Bytes       uint64  `json:"bytes"`
	CurrentPart string  `json:"current_part"`
}

func (s *stats) Update(size int) {
	s.Last = utc.Now()
	s.Packets++
	s.Bytes += uint64(size)
}

func (s *stats) Fields() []any {
	now := utc.Now()
	durStart := now.Sub(s.Start)
	durLast := now.Sub(s.Last)
	return []any{
		"stream", s.Stream,
		"dur", duration.Spec(durStart).RoundTo(2),
		"ipd", duration.Spec(durLast).RoundTo(2),
		"packets", humanize.SI(float64(s.Packets), "P"),
		"bytes", humanize.Bytes(s.Bytes),
		"bytes_avg", humanize.SIWithDigits(float64(s.Bytes)/durStart.Seconds(), 2, "B/s"),
		"packets_avg", humanize.SIWithDigits(float64(s.Packets)/durStart.Seconds(), 2, "P/s"),
		"current_part", s.CurrentPart,
	}
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopMonitor struct{}

func (n NoopMonitor) Start(string)      {}
func (n NoopMonitor) Stop()             {}
func (n NoopMonitor) SignalPacket(int)  {}
func (n NoopMonitor) SignalPart(string) {}
