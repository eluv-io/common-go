package mpegts

import (
	"fmt"
	"strings"
	"time"

	"github.com/Comcast/gots/v2"
	"github.com/Comcast/gots/v2/packet"
	"github.com/Comcast/gots/v2/psi"
	"github.com/HdrHistogram/hdrhistogram-go"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/maputil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// TsStreamTracker is a component that validates and tracks MPEG Transport Streams. It checks TS packets for errors
// (sync byte, continuity counter, etc.) and collects statistics about encapsulated elementary streams (PID, PCR,
// jitter, etc.). It optionally logs statistics at a specified interval.
type TsStreamTracker interface {
	// Track feeds TS packets to the tracker. The packet bytes (bts) can consist of a single or multiple TS packets. The
	// method will validate each packet and aggregate any errors. The method returns nil if all packets are valid or a
	// list of errors otherwise.
	Track(bts []byte) (packetCount int, errList error)
	// Stats returns TS statistics
	Stats() *Stats
	// Reset resets the tracker state, clearing all statistics and errors. It keeps the list of discovered streams and
	// their information from the PMT (Program Map Table).
	Reset()
}

// NewTsStreamTracker creates a tracker for MPEG TS elementary streams.
func NewTsStreamTracker(streamId string, statsLogPeriod time.Duration, stripRtp bool) TsStreamTracker {
	tracker := &tsStreamTracker{
		streamId:    streamId,
		stripRtp:    stripRtp,
		statsLogger: NoopPeriodic{},
		start:       utc.Now(),
		streams:     make(map[int]*Stream),
		pmtAcc:      packet.NewAccumulator(psi.PmtAccumulatorDoneFunc),
		logThrottle: timeutil.NewPeriodic(10 * time.Second),
	}
	if statsLogPeriod > 0 {
		tracker.statsLogger = timeutil.NewPeriodic(statsLogPeriod)
	}
	return tracker
}

type tsStreamTracker struct {
	streamId    string
	stripRtp    bool
	statsLogger timeutil.Periodic
	start       utc.UTC
	errCount    int
	streams     map[int]*Stream
	pmtParsed   bool // true if the PMT has been parsed
	pmtAcc      packet.Accumulator
	pat         psi.PAT
	logThrottle timeutil.Periodic
	panics      int
}

func (t *tsStreamTracker) Track(bts []byte) (packetCount int, errList error) {
	errCount := 0

	defer func() {
		t.errCount += errCount
		t.statsLogger.Do(func() {
			statsLog.Info("ts-stream-tracker", "stream", t.streamId, "stats", jsonutil.Stringer(t.Stats()))
		})
	}()

	appendErr := func(err error) {
		errList = errors.Append(errList, err)
		errCount++
	}

	if t.stripRtp {
		var err error
		bts, err = rtp.StripHeader(bts)
		if err != nil {
			return 0, err
		}
	}

	for ; len(bts) >= packet.PacketSize; bts = bts[packet.PacketSize:] {
		if errCount >= 20 {
			appendErr(fmt.Errorf("too many errors: %d", errCount))
			packetCount += len(bts) / packet.PacketSize
			return packetCount, errList
		}
		packetCount++
		pkt := packet.Packet(bts)

		err := pkt.CheckErrors()
		if err != nil {
			appendErr(fmt.Errorf("checkerr=%s ts-packet=%d", err, packetCount))
			continue
		}

		pid := pkt.PID()

		cc := pkt.ContinuityCounter()
		stream, ok := t.streams[pid]
		if !ok {
			stream = t.newStream(pid, cc)
			t.streams[pid] = stream
		} else if stream.cc == -1 {
			stream.cc = cc
		} else if pid != packet.NullPacketPid {
			expectCC := stream.cc
			if pkt.HasPayload() {
				expectCC = (stream.cc + 1) % 16
			}
			if cc != expectCC {
				stream.ccErrors++
				err = fmt.Errorf("continuity counter mismatch: expected=%02d actual=%02d ts-packet=%d pid=%d", expectCC, cc, packetCount, pid)
				appendErr(err)
			}
			stream.cc = cc
		}
		stream.packetCount++

		if pcr, ok := ExtractPCR(&pkt); ok {
			now := utc.Now()
			if stream.pcr0 == utc.Zero {
				stream.pcr0 = now.Add(-PcrToDuration(pcr))
			} else {
				if pcr < stream.pcr {
					if pcr+100_000_000 < stream.pcr {
						// PCR wrapped around. Reset the reference time.
						stream.pcr0 = stream.pcr0.Add(PcrToDuration(MaxPCR + 1))
					} else {
						// likely packet re-ordering or an encoder bug. Ignore the
					}
				}
				jitter := PcrToDuration(pcr) - now.Sub(stream.pcr0)
				if jitter < 0 {
					jitter = -jitter
				}
				stream.jitter = jitter
				err = stream.jitterMillisHist.RecordValue(jitter.Milliseconds())
				if err != nil {
					// appendErr(errors.E("jitter histogram", errors.K.Invalid, err))
				}
			}
			stream.pcr = pcr
		}

		err = t.parsePmt(&pkt)
		if err != nil {
			appendErr(err)
		}
	}
	if len(bts) > 0 {
		err := fmt.Errorf("packet too short: %d ts-packet=%d", len(bts), packetCount+1)
		appendErr(err)
	}

	return packetCount, errList
}

func (t *tsStreamTracker) newStream(pid int, cc int) *Stream {
	return &Stream{
		pid:              pid,
		cc:               cc,
		jitterMillisHist: hdrhistogram.New(1, int64(time.Minute/time.Millisecond), 3),
	}
}

func (t *tsStreamTracker) parsePmt(pkt *packet.Packet) error {
	if t.pmtParsed {
		return nil
	}

	defer func() {
		// gots has bugs and may panic...
		if r := recover(); r != nil {
			t.panics++
			t.logThrottle.Do(func() {
				log.Warn("recovered from panic", "error", r, "count", t.panics)
				fmt.Println("tsStreamTracker - recovered from panic:", r, "count", t.panics)
			})
		}
	}()

	if t.pat == nil {
		if !packet.IsPat(pkt) {
			return nil
		}
		pat, err := psi.NewPAT(pkt[:])
		if err != nil {
			return err
		}
		t.pat = pat
		return nil
	}

	if ok, err := psi.IsPMT(pkt, t.pat); err != nil {
		return err
	} else if !ok {
		return nil
	}

	_, err := t.pmtAcc.WritePacket(pkt)
	if errors.Is(err, gots.ErrAccumulatorDone) {
		// done
	} else if err != nil {
		return err
	} else {
		// not done
		return nil
	}

	payload := t.pmtAcc.Bytes()
	pmt, err := psi.NewPMT(payload)
	if err != nil {
		return err
	}

	for _, es := range pmt.ElementaryStreams() {
		pid := int(es.ElementaryPid())
		stream := t.streams[pid]
		if stream == nil {
			stream = t.newStream(pid, -1)
			t.streams[pid] = stream
		}
		stream.pes = es
	}

	t.pmtParsed = true

	return nil
}

func (t *tsStreamTracker) Stats() *Stats {
	res := &Stats{
		Start:      t.start,
		Duration:   duration.Spec(utc.Since(t.start)).RoundTo(2),
		ErrorCount: t.errCount,
		Streams:    make([]*StreamStats, 0, len(t.streams)),
	}

	keys := maputil.SortedKeys(t.streams)
	for _, pid := range keys {
		stream := t.streams[pid]
		s := &StreamStats{
			Pid:         pid,
			PacketCount: stream.packetCount,
			Cc:          stream.cc,
			CcErrors:    stream.ccErrors,
			Pcr:         stream.pcr,
			Jitter:      duration.Spec(stream.jitter).RoundTo(2),
		}
		if stream.pcr0 != utc.Zero {
			s.Pcr0 = &stream.pcr0
		}

		if stream.pes != nil {
			s.Info = fmt.Sprintf("%d: %s", stream.pes.StreamType(), stream.pes.StreamTypeDescription())
		}
		if stream.jitterMillisHist.TotalCount() > 0 {
			s.JitterMillisHist = &HistogramCapture{}
			CaptureHistogram(stream.jitterMillisHist, s.JitterMillisHist)
		}
		res.Streams = append(res.Streams, s)

		res.PacketCount += stream.packetCount
	}
	return res
}

func (t *tsStreamTracker) Reset() {
	t.start = utc.Now()
	t.errCount = 0
	t.pmtParsed = false
	t.pmtAcc.Reset()
	for _, stream := range t.streams {
		// retain these fields: pid, cc, pcr, pcr0, pes
		// reset all stats fields.
		stream.packetCount = 0
		stream.ccErrors = 0
		// stream.pcr0 = utc.Zero
		stream.jitter = 0
		stream.jitterMillisHist.Reset()
	}
}

// ---------------------------------------------------------------------------------------------------------------------

type Stream struct {
	pid              int                     // packet identifier 13 bits
	packetCount      int                     // total number of packets for this stream
	cc               int                     // continuity counter 4 bits
	ccErrors         int                     // cumulated continuity counter errors
	pcr              uint64                  // program clock reference 33+9 bits
	pcr0             utc.UTC                 // time corresponding to PCR 0
	jitter           time.Duration           // jitter between PCR and system time
	jitterMillisHist *hdrhistogram.Histogram // jitter histogram
	pes              psi.PmtElementaryStream // stream info
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopTracker struct{}

func (n NoopTracker) Track(bts []byte) (int, error) {
	return 0, nil
}

func (n NoopTracker) Stats() *Stats {
	return nil
}

func (n NoopTracker) Reset() {}

// ---------------------------------------------------------------------------------------------------------------------

type Stats struct {
	Start       utc.UTC        `json:"start"`
	Duration    duration.Spec  `json:"duration"`
	PacketCount int            `json:"packet_count"`
	ErrorCount  int            `json:"error_count"`
	Streams     []*StreamStats `json:"streams"`
}

// Categorize returns a packet count per stream type.
func (s *Stats) Categorize() (stats PacketStats) {
	for _, stream := range s.Streams {
		if stream.Pid == packet.NullPacketPid {
			stats.Padding += stream.PacketCount
		} else {
			switch {
			case strings.Contains(stream.Info, "Audio"):
				stats.Audio += stream.PacketCount
			case strings.Contains(stream.Info, "Video"):
				stats.Video += stream.PacketCount
			}
		}
		stats.Total += stream.PacketCount
	}

	rat := func(n int) float64 {
		if stats.Total == 0 {
			return 0
		}
		return float64(n) / float64(stats.Total)
	}

	stats.Other = stats.Total - stats.Audio - stats.Video - stats.Padding
	stats.AudioRatio = rat(stats.Audio)
	stats.VideoRatio = rat(stats.Video)
	stats.PaddingRatio = rat(stats.Padding)
	stats.OtherRatio = rat(stats.Other)
	return
}

type PacketStats struct {
	Total        int     `json:"total"`
	Audio        int     `json:"audio"`
	Video        int     `json:"video"`
	Padding      int     `json:"padding"`
	Other        int     `json:"other"`
	AudioRatio   float64 `json:"audio_rat"`
	VideoRatio   float64 `json:"video_rat"`
	PaddingRatio float64 `json:"padding_rat"`
	OtherRatio   float64 `json:"other_rat"`
}

type StreamStats struct {
	Pid              int               `json:"pid"`                              // packet identifier 13 bits
	PacketCount      int               `json:"packet_count"`                     // total number of packets for this stream
	Cc               int               `json:"cc"`                               // continuity counter 4 bits
	CcErrors         int               `json:"cc_errors"`                        // cumulated continuity counter errors
	Pcr              uint64            `json:"pcr,omitempty"`                    // program clock reference 33+9 bits
	Pcr0             *utc.UTC          `json:"pcr_0,omitempty"`                  // time corresponding to PCR 0
	Jitter           duration.Spec     `json:"jitter,omitempty"`                 // jitter between PCR and system time
	JitterMillisHist *HistogramCapture `json:"jitter_abs_millis_hist,omitempty"` // jitter histogram in absolute millis
	Info             string            `json:"info,omitempty"`                   // stream info
}

type HistogramCapture struct {
	Min             int64   `json:"min"`
	Max             int64   `json:"max"`
	Mean            float64 `json:"mean"`
	StdDev          float64 `json:"std_dev"`
	Percentile_01_0 int64   `json:"percentile_01_0"`
	Percentile_02_5 int64   `json:"percentile_02_5"`
	Percentile_50_0 int64   `json:"percentile_50_0"`
	Percentile_97_5 int64   `json:"percentile_97_5"`
	Percentile_99_0 int64   `json:"percentile_99_0"`
	Percentile_99_9 int64   `json:"percentile_99_9"`
}

func CaptureHistogram(h *hdrhistogram.Histogram, c *HistogramCapture) {
	if h == nil || c == nil {
		return
	}

	c.Min = h.Min()
	c.Max = h.Max()
	c.Mean = h.Mean()

	c.StdDev = h.StdDev()
	c.Percentile_01_0 = h.ValueAtPercentile(1.0)
	c.Percentile_02_5 = h.ValueAtPercentile(2.5)
	c.Percentile_50_0 = h.ValueAtPercentile(50.0)
	c.Percentile_97_5 = h.ValueAtPercentile(97.5)
	c.Percentile_99_0 = h.ValueAtPercentile(99.0)
	c.Percentile_99_9 = h.ValueAtPercentile(99.9)
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopPeriodic struct{}

func (n NoopPeriodic) Do(f func()) bool {
	return false
}
