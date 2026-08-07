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
	// TrackPackets feeds already-decoded MPEG-TS packets to the tracker, e.g. ones obtained from
	// pktpool.Packet.Ts().Packets(). It performs the same validation and statistics aggregation as Track, without
	// re-parsing or re-framing bytes that have already been decoded - so unlike Track, no TsFraming is applied; the
	// caller/decoder must already have stripped any RTP header or ATS-TS timestamp prefix.
	TrackPackets(pkts []*packet.Packet) (packetCount int, errList error)
	// Stats returns TS statistics
	Stats() *Stats
	// Snapshot populates snap with the tracker's current stats, reusing snap's Streams slice (and each entry's
	// JitterMillisHist) where possible instead of allocating new ones - for a caller that polls periodically and
	// wants to bound per-call garbage. When full is false, it skips the per-PID walk and histogram capture
	// entirely (the expensive part): Streams is cleared, not populated, and only the cheap running totals
	// (ErrorCount, PacketCount) are set. Stats() is equivalent to Snapshot(&Stats{}, true).
	Snapshot(snap *Stats, full bool) *Stats
	// Reset resets the tracker state, clearing all statistics and errors. It keeps the list of discovered streams and
	// their information from the PMT (Program Map Table).
	Reset()
}

// TsFraming describes how each Track() input is framed on top of the raw MPEG-TS packets. A given stream uses exactly
// one framing for its lifetime.
type TsFraming int

const (
	// TsFramingNone indicates raw MPEG-TS packets with no additional per-input framing.
	TsFramingNone TsFraming = iota
	// TsFramingRtp indicates each input is an RTP packet; the RTP header is stripped before TS parsing.
	TsFramingRtp
	// TsFramingAts indicates each input is an ATS-TS value: an 8-byte big-endian arrival timestamp (nanoseconds since
	// the Unix epoch, see AtsTimestampLen) followed by the raw MPEG-TS packets. The timestamp prefix is stripped before
	// TS parsing.
	TsFramingAts
)

func (f TsFraming) String() string {
	switch f {
	case TsFramingNone:
		return "none"
	case TsFramingRtp:
		return "rtp"
	case TsFramingAts:
		return "ats"
	default:
		return fmt.Sprintf("unknown:%d", int(f))
	}
}

// NewTsStreamTracker creates a tracker for MPEG TS elementary streams. stripRtp selects between raw (TsFramingNone) and
// RTP (TsFramingRtp) input framing; use NewTsStreamTrackerFramed for ATS-TS or to specify the framing explicitly.
func NewTsStreamTracker(streamId string, statsLogPeriod time.Duration, stripRtp bool) TsStreamTracker {
	framing := TsFramingNone
	if stripRtp {
		framing = TsFramingRtp
	}
	return NewTsStreamTrackerFramed(streamId, statsLogPeriod, framing)
}

// NewTsStreamTrackerFramed creates a tracker for MPEG TS elementary streams, specifying how each Track() input is
// framed (see TsFraming). Use this for ATS-TS input (TsFramingAts), which carries an 8-byte arrival-timestamp prefix
// that is stripped before TS parsing.
func NewTsStreamTrackerFramed(streamId string, statsLogPeriod time.Duration, framing TsFraming) TsStreamTracker {
	tracker := &tsStreamTracker{
		streamId:    streamId,
		framing:     framing,
		statsLogger: NoopPeriodic{},
		start:       utc.Now(),
		streams:     make(map[int]*Stream),
		pmtAcc:      packet.NewAccumulator(psi.PmtAccumulatorDoneFunc),
		logThrottle: timeutil.NewPeriodic(10 * time.Second),
	}
	if statsLogPeriod > 0 {
		tracker.statsLogger = timeutil.NewPeriodic(statsLogPeriod)
	}
	tracker.statsLogFunc = func() {
		statsLog.Info("ts-stream-tracker", "stream", tracker.streamId, "stats", jsonutil.Stringer(tracker.Stats()))
	}
	return tracker
}

type tsStreamTracker struct {
	streamId    string
	framing     TsFraming
	statsLogger timeutil.Periodic
	// pre-allocated closure for periodic stats logging, stored to avoid the per-call allocation that a bound method
	// value or an inline closure with a capture of t `t.statsLogger.Do(func() { t.Xyz ...}` would cause.
	statsLogFunc func()
	start        utc.UTC
	errCount     int
	// totalCcErrors/totalPacketCount mirror the sum of every stream's ccErrors/packetCount, kept in sync
	// incrementally in trackPacket so Snapshot(full=false) can report them without walking streams.
	totalCcErrors    int
	totalPacketCount int
	streams          map[int]*Stream
	pmtParsed        bool // true if the PMT has been parsed
	pmtAcc           packet.Accumulator
	pat              psi.PAT
	logThrottle      timeutil.Periodic
	panics           int
}

func (t *tsStreamTracker) Track(bts []byte) (packetCount int, errList error) {
	errCount := 0

	defer func() {
		t.errCount += errCount
		t.statsLogger.Do(t.statsLogFunc)
	}()

	appendErr := func(err error) {
		errList = errors.Append(errList, err)
		errCount++
	}

	switch t.framing {
	case TsFramingNone:
		// raw MPEG-TS: no per-input framing to strip
	case TsFramingRtp:
		var err error
		bts, err = rtp.StripHeader(bts)
		if err != nil {
			return 0, err
		}
	case TsFramingAts:
		var err error
		_, bts, err = ParseAtsTs(bts)
		if err != nil {
			return 0, err
		}
	default:
		return 0, errors.NoTrace("TsStreamTracker.Track", errors.K.Invalid,
			"reason", "unknown TS framing",
			"framing", int(t.framing))
	}

	for ; len(bts) >= packet.PacketSize; bts = bts[packet.PacketSize:] {
		if errCount >= 20 {
			appendErr(fmt.Errorf("too many errors: %d", errCount))
			packetCount += len(bts) / packet.PacketSize
			return packetCount, errList
		}
		packetCount++
		// Previous code `pkt := packet.Packet(bts)` copied 188 bytes into a local value, which escaped to the heap
		// because its address was taken further below (&pkt). Fixed by casting the slice to a pointer directly, so no
		// copy or heap allocation occurs.
		pkt := (*packet.Packet)(bts[:packet.PacketSize])
		t.trackPacket(pkt, packetCount, appendErr)
	}
	if len(bts) > 0 {
		err := fmt.Errorf("packet too short: %d ts-packet=%d", len(bts), packetCount+1)
		appendErr(err)
	}

	return packetCount, errList
}

// TrackPackets feeds already-decoded MPEG-TS packets to the tracker, e.g. ones obtained from
// pktpool.Packet.Ts().Packets(). It performs the same validation and statistics aggregation as Track, without
// re-parsing or re-framing bytes that have already been decoded - letting a caller that already holds decoded
// packets (such as pooled, lazily-decoded ones) avoid a second parse.
func (t *tsStreamTracker) TrackPackets(pkts []*packet.Packet) (packetCount int, errList error) {
	errCount := 0

	defer func() {
		t.errCount += errCount
		t.statsLogger.Do(t.statsLogFunc)
	}()

	appendErr := func(err error) {
		errList = errors.Append(errList, err)
		errCount++
	}

	for _, pkt := range pkts {
		if errCount >= 20 {
			appendErr(fmt.Errorf("too many errors: %d", errCount))
			return len(pkts), errList
		}
		packetCount++
		t.trackPacket(pkt, packetCount, appendErr)
	}

	return packetCount, errList
}

// trackPacket validates a single already-positioned TS packet and updates stream statistics accordingly. packetCount
// is used only to identify the offending packet in diagnostic error messages (its position within the current
// Track/TrackPackets call).
func (t *tsStreamTracker) trackPacket(pkt *packet.Packet, packetCount int, appendErr func(error)) {
	err := pkt.CheckErrors()
	if err != nil {
		appendErr(fmt.Errorf("checkerr=%s ts-packet=%d", err, packetCount))
		return
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
			t.totalCcErrors++
			err = fmt.Errorf("continuity counter mismatch: expected=%02d actual=%02d ts-packet=%d pid=%d", expectCC, cc, packetCount, pid)
			appendErr(err)
		}
		stream.cc = cc
	}
	stream.packetCount++
	t.totalPacketCount++

	if pcr, ok := ExtractPCR(pkt); ok {
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

	err = t.parsePmt(pkt)
	if err != nil {
		appendErr(err)
	}
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

// Stats returns the tracker's current stats as a freshly-allocated Stats. Equivalent to
// Snapshot(&Stats{}, true).
func (t *tsStreamTracker) Stats() *Stats {
	return t.Snapshot(&Stats{}, true)
}

// Snapshot populates snap with the tracker's current stats. See the TsStreamTracker interface doc for the
// full/reuse contract.
func (t *tsStreamTracker) Snapshot(snap *Stats, full bool) *Stats {
	snap.Start = t.start
	snap.Duration = duration.Spec(utc.Since(t.start)).RoundTo(2)
	snap.ErrorCount = t.errCount
	snap.CcErrors = t.totalCcErrors

	if !full {
		snap.Streams = snap.Streams[:0]
		snap.PacketCount = t.totalPacketCount
		return snap
	}

	keys := maputil.SortedKeys(t.streams)
	for i, pid := range keys {
		stream := t.streams[pid]

		var s *StreamStats
		if i < len(snap.Streams) && snap.Streams[i] != nil && snap.Streams[i].Pid == pid {
			s = snap.Streams[i] // reuse in place - same PID as last time at this position
		} else {
			s = &StreamStats{}
			if i < len(snap.Streams) {
				snap.Streams[i] = s
			} else {
				snap.Streams = append(snap.Streams, s)
			}
		}

		s.Pid = pid
		s.PacketCount = stream.packetCount
		s.Cc = stream.cc
		s.CcErrors = stream.ccErrors
		s.Pcr = stream.pcr
		s.Jitter = duration.Spec(stream.jitter).RoundTo(2)
		s.Info = ""
		if stream.pcr0 != utc.Zero {
			s.Pcr0 = &stream.pcr0
		} else {
			s.Pcr0 = nil
		}
		if stream.pes != nil {
			s.Info = fmt.Sprintf("%d: %s", stream.pes.StreamType(), stream.pes.StreamTypeDescription())
		}
		if stream.jitterMillisHist.TotalCount() > 0 {
			if s.JitterMillisHist == nil {
				s.JitterMillisHist = &HistogramCapture{}
			}
			CaptureHistogram(stream.jitterMillisHist, s.JitterMillisHist)
		} else {
			s.JitterMillisHist = nil
		}
	}
	snap.Streams = snap.Streams[:len(keys)]
	snap.PacketCount = t.totalPacketCount
	return snap
}

func (t *tsStreamTracker) Reset() {
	t.start = utc.Now()
	t.errCount = 0
	t.totalCcErrors = 0
	t.totalPacketCount = 0
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

func (n NoopTracker) TrackPackets(pkts []*packet.Packet) (int, error) {
	return 0, nil
}

func (n NoopTracker) Stats() *Stats {
	return nil
}

func (n NoopTracker) Snapshot(snap *Stats, full bool) *Stats {
	return snap
}

func (n NoopTracker) Reset() {}

// ---------------------------------------------------------------------------------------------------------------------

type Stats struct {
	Start       utc.UTC       `json:"start"`
	Duration    duration.Spec `json:"duration"`
	PacketCount int           `json:"packet_count"`
	ErrorCount  int           `json:"error_count"`
	// CcErrors is the sum of every stream's continuity-counter error count - available cheaply (from an
	// incrementally-maintained running total) even when Streams itself is empty (see Snapshot's full=false case).
	CcErrors int            `json:"cc_errors"`
	Streams  []*StreamStats `json:"streams"`
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

// CopyInto deep-copies s into dst, reusing dst's existing Streams slice (and each entry's
// JitterMillisHist) where their shape already matches, allocating only where it doesn't. Use this to
// detach a Stats obtained from a caller that may reuse/mutate it later (see TsStreamTracker.Snapshot)
// into memory the receiver owns outright - unlike a plain struct copy, which would still alias Streams
// and its nested pointers.
func (s *Stats) CopyInto(dst *Stats) {
	dst.Start = s.Start
	dst.Duration = s.Duration
	dst.PacketCount = s.PacketCount
	dst.ErrorCount = s.ErrorCount
	dst.CcErrors = s.CcErrors

	for i, stream := range s.Streams {
		var d *StreamStats
		if i < len(dst.Streams) && dst.Streams[i] != nil {
			d = dst.Streams[i]
		} else {
			d = &StreamStats{}
			if i < len(dst.Streams) {
				dst.Streams[i] = d
			} else {
				dst.Streams = append(dst.Streams, d)
			}
		}
		d.Pid = stream.Pid
		d.PacketCount = stream.PacketCount
		d.Cc = stream.Cc
		d.CcErrors = stream.CcErrors
		d.Pcr = stream.Pcr
		d.Jitter = stream.Jitter
		d.Info = stream.Info
		if stream.Pcr0 != nil {
			pcr0 := *stream.Pcr0
			d.Pcr0 = &pcr0
		} else {
			d.Pcr0 = nil
		}
		if stream.JitterMillisHist != nil {
			if d.JitterMillisHist == nil {
				d.JitterMillisHist = &HistogramCapture{}
			}
			*d.JitterMillisHist = *stream.JitterMillisHist
		} else {
			d.JitterMillisHist = nil
		}
	}
	dst.Streams = dst.Streams[:len(s.Streams)]
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
