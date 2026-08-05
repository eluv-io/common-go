// Package tracker provides MediaTracker, a component that tracks the timing and integrity of a live media stream
// received over the network. It composes the existing mpegts.TsStreamTracker (MPEG-TS continuity-counter/PID/PMT
// validation) with arrival-vs-media-clock correlation (skew/jitter/drift, for both RTP timestamps and per-PID MPEG-TS
// PCR) and datagram-level timing statistics (packet rate, bitrate, inter-packet delay, outages).
//
// MediaTracker exposes two entry points for feeding it packets: TrackDatagram, for a caller that only has the raw
// network bytes (e.g. a CLI probe reading from a socket), and TrackPacket, for a caller that already holds a
// decoded pktpool.Packet (e.g. a media pipeline that parses once and fans out to multiple consumers) - the latter
// avoids re-parsing the RTP/MPEG-TS layers a second time.
package tracker

import (
	"sync"
	"time"

	"github.com/Comcast/gots/v2/packet"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/mpegts"
	"github.com/eluv-io/common-go/media/pktpool"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/histogram"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// defaultOutageThreshold is the default minimum gap between two consecutive packets counted as an outage.
const defaultOutageThreshold = time.Second

// defaultMaxGaps is the default bound on the number of RTP Gap events retained in a ClockStats.
const defaultMaxGaps = 100

// rtpTsMinLen is the minimum datagram length for RTP-wrapped MPEG-TS: an RTP header plus at least one TS packet.
const rtpTsMinLen = 12 + packet.PacketSize

// Config configures a MediaTracker.
type Config struct {
	// Rtp selects whether the input is RTP-wrapped MPEG-TS. When true, the RTP timestamp clock is correlated in
	// addition to the per-PID MPEG-TS PCR clock, which is always tracked.
	Rtp bool

	// StatsLogPeriod, when > 0, enables the embedded mpegts.TsStreamTracker's own periodic stats logging.
	StatsLogPeriod time.Duration

	// OutageThreshold is the minimum gap between two consecutive packets counted as an outage. Defaults to 1s.
	OutageThreshold time.Duration

	// MaxGaps bounds the number of RTP Gap events retained in ClockStats.Gaps; once the bound is reached, further
	// gaps are only counted (GapsOverflow), not retained. Defaults to 100.
	MaxGaps int

	// OnRtpSample, if non-nil, is called for every RTP packet's raw header fields (sequence number and timestamp,
	// in packet order), before gap detection or unwrapping. Used by a caller that needs the raw per-packet timing
	// inputs, e.g. a debug trace.
	OnRtpSample func(seq uint16, ts uint32)

	// OnPcr, if non-nil, is called for every PCR value extracted from the stream (in packet order), tagged with the
	// PID that carried it. Used by a caller that needs the raw per-packet timing inputs, e.g. a debug trace.
	OnPcr func(pid int, pcr uint64)
}

// MediaTracker tracks the timing and integrity of a live media stream. See the package doc for an overview.
type MediaTracker interface {
	// TrackDatagram processes one raw network datagram received at now (the datagram's arrival time), parsing the
	// RTP header (if the tracker is configured for RTP) and the contained MPEG-TS packets.
	TrackDatagram(now utc.UTC, datagram []byte) error
	// TrackPacket processes one already-decoded pooled packet received at now (the packet's arrival time), reusing
	// its already-parsed RTP/MPEG-TS layers instead of re-parsing pkt.Data.
	TrackPacket(now utc.UTC, pkt *pktpool.Packet) error
	// Stats returns the cumulative stats for the whole capture.
	Stats() *Stats
	// PeriodStats returns the stats for the current reporting period and resets the period window.
	PeriodStats() *Stats
	// Reset resets the tracker state, clearing all statistics.
	Reset()
}

// NewMediaTracker creates a MediaTracker for a single stream.
func NewMediaTracker(streamId string, cfg Config) MediaTracker {
	if cfg.OutageThreshold <= 0 {
		cfg.OutageThreshold = defaultOutageThreshold
	}
	if cfg.MaxGaps <= 0 {
		cfg.MaxGaps = defaultMaxGaps
	}
	t := &mediaTracker{
		streamId: streamId,
		cfg:      cfg,
		// We feed already-decapsulated TS bytes/packets to the tracker (stripRtp=false), since MediaTracker itself
		// strips the RTP layer before feeding the tracker.
		tsTracker:     mpegts.NewTsStreamTracker(streamId, cfg.StatsLogPeriod, false),
		pcrClock:      newPcrCorrelator(cfg.MaxGaps),
		ipdHistTotal:  newIpdHistogram(),
		ipdHistPeriod: newIpdHistogram(),
	}
	if cfg.Rtp {
		t.rtpClock = newRtpCorrelator(cfg.MaxGaps)
	}
	return t
}

type mediaTracker struct {
	mu       sync.Mutex
	streamId string
	cfg      Config

	start       utc.UTC // arrival time of the first packet
	lastArrival utc.UTC // arrival time of the previous packet

	// cumulative counters
	totalPackets uint64
	totalBytes   uint64
	outageCount  uint64
	outageTotal  duration.Millis

	// cumulative distributions
	ipdTotal     statsutil.Statistics[duration.Millis]
	ipdHistTotal *histogram.Histogram[duration.Millis]
	rateTotal    statsutil.Statistics[float64] // per-period packet rate (packets/s) - captures rate variation
	bitrateTotal statsutil.Statistics[float64] // per-period bitrate (bits/s) - captures bitrate variation

	// current period counters and distributions (reset on each PeriodStats call)
	periodStart   utc.UTC
	periodPackets uint64
	periodBytes   uint64
	ipdPeriod     statsutil.Statistics[duration.Millis]
	ipdHistPeriod *histogram.Histogram[duration.Millis]

	// arrival vs media-clock correlation. The PCR correlation is always present (extracted from the MPEG-TS
	// payload); for RTP sources the RTP-timestamp correlation is present too, so both clocks are correlated.
	rtpClock *rtpCorrelator // nil for raw MPEG-TS sources
	pcrClock *pcrCorrelator

	// tsTracker validates the transport stream and collects per-program/PID structure (stream types from the PMT,
	// packet counts, continuity-counter errors).
	tsTracker mpegts.TsStreamTracker

	// counters for input validation/integrity conditions not covered by tsTracker/the clock correlators.
	smallPacketsDropped   uint64 // datagram too small to plausibly contain a TS packet (+ RTP header, if configured)
	rtcpPacketsDropped    uint64 // subset of smallPacketsDropped that looks like a misdirected RTCP packet
	badPackets            uint64 // datagram dropped due to a malformed RTP header or a bad TS packet within it
	incompletePackets     uint64 // TS payload length not a multiple of the TS packet size
	adaptationFieldErrors uint64 // TS adaptation-field/PCR parse errors
	faultyPaddingPackets  uint64 // null-PID packet whose payload is not valid 0xFF padding
	longHeaders           uint64 // RTP header longer than the expected 12 bytes (extension-bearing packets)
}

// TrackDatagram processes one raw network datagram received at now (the datagram's arrival time), parsing the RTP
// header (if the tracker is configured for RTP) and the contained MPEG-TS packets.
func (t *mediaTracker) TrackDatagram(now utc.UTC, datagram []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.recordArrival(now, len(datagram))

	tsBytes := datagram
	if t.rtpClock != nil {
		if len(datagram) < rtpTsMinLen {
			t.smallPacketsDropped++
			if isRTCP(datagram) {
				t.rtcpPacketsDropped++
			}
			return nil
		}
		pkt, err := rtp.ParsePacket(datagram)
		if err != nil {
			t.badPackets++
			t.rtpClock.parseErrors++
			return err
		}
		if t.cfg.OnRtpSample != nil {
			t.cfg.OnRtpSample(pkt.SequenceNumber, pkt.Timestamp)
		}
		if headerLen := len(datagram) - len(pkt.Payload) - int(pkt.PaddingSize); headerLen != 12 {
			t.longHeaders++
		}
		t.rtpClock.record(now, pkt.SequenceNumber, pkt.Timestamp)
		tsBytes = pkt.Payload
	} else if len(datagram) < packet.PacketSize {
		t.smallPacketsDropped++
		return nil
	}

	if len(tsBytes)%packet.PacketSize != 0 {
		t.incompletePackets++
		return errors.NoTrace("MediaTracker.TrackDatagram", errors.K.Invalid,
			"reason", "ts payload length is not a multiple of the TS packet size", "len", len(tsBytes))
	}

	_, errList := t.tsTracker.Track(tsBytes)
	if errList != nil {
		t.badPackets++
	}
	t.scanTsBytes(now, tsBytes)
	return errList
}

// TrackPacket processes one already-decoded pooled packet received at now (the packet's arrival time), reusing its
// already-parsed RTP/MPEG-TS layers - see pktpool.Packet's layer-decoding contract - instead of re-parsing pkt.Data.
func (t *mediaTracker) TrackPacket(now utc.UTC, pkt *pktpool.Packet) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	datagram := pkt.Data
	t.recordArrival(now, len(datagram))

	if t.rtpClock != nil {
		if len(datagram) < rtpTsMinLen {
			t.smallPacketsDropped++
			if isRTCP(datagram) {
				t.rtcpPacketsDropped++
			}
			return nil
		}
		rtpLayer, err := pkt.Rtp()
		if err != nil {
			t.badPackets++
			t.rtpClock.parseErrors++
			return err
		}
		hdr := rtpLayer.Packet().Header
		if hdr.Version != 2 {
			t.badPackets++
			t.rtpClock.parseErrors++
			return errors.NoTrace("MediaTracker.TrackPacket", errors.K.Invalid,
				"reason", "unsupported RTP version", "version", hdr.Version)
		}
		if t.cfg.OnRtpSample != nil {
			t.cfg.OnRtpSample(hdr.SequenceNumber, hdr.Timestamp)
		}
		if headerLen := len(datagram) - len(rtpLayer.Payload) - int(hdr.PaddingSize); headerLen != 12 {
			t.longHeaders++
		}
		t.rtpClock.record(now, hdr.SequenceNumber, hdr.Timestamp)
	} else if len(datagram) < packet.PacketSize {
		t.smallPacketsDropped++
		return nil
	}

	tsLayer, err := pkt.Ts()
	if err != nil {
		t.incompletePackets++
		return err
	}
	tsPkts := tsLayer.Packets()

	_, errList := t.tsTracker.TrackPackets(tsPkts)
	if errList != nil {
		t.badPackets++
	}
	t.scanTsPackets(now, tsPkts)
	return errList
}

// recordArrival folds a single packet's arrival into the packet/byte counters and the inter-packet-delay/outage
// statistics. now is the packet's arrival time, as supplied by the caller (never sampled internally), so callers
// whose packets pass through an intermediate queue (e.g. a channel) can supply the true network arrival time rather
// than the time TrackDatagram/TrackPacket happens to be invoked.
func (t *mediaTracker) recordArrival(now utc.UTC, n int) {
	if t.start.IsZero() {
		t.start = now
		t.periodStart = now
	}
	t.totalPackets++
	t.totalBytes += uint64(n)
	t.periodPackets++
	t.periodBytes += uint64(n)

	if !t.lastArrival.IsZero() {
		ipd := now.Sub(t.lastArrival)
		ipdMs := duration.Millis(ipd)
		t.ipdTotal.Update(now, ipdMs)
		t.ipdPeriod.Update(now, ipdMs)
		t.ipdHistTotal.Observe(ipdMs)
		t.ipdHistPeriod.Observe(ipdMs)
		if ipd >= t.cfg.OutageThreshold {
			t.outageCount++
			t.outageTotal += duration.Millis(ipd)
		}
	}
	t.lastArrival = now
}

// scanTsBytes walks the given already-validated (length is a multiple of the TS packet size) raw TS bytes, extracting
// PCR values and checking null-packet integrity. It stops (and counts a stream-level parse error) at the first
// TS-misaligned packet, mirroring scanTsPackets's per-packet checks for a caller that only has raw bytes.
func (t *mediaTracker) scanTsBytes(now utc.UTC, tsBytes []byte) {
	for off := 0; off+packet.PacketSize <= len(tsBytes); off += packet.PacketSize {
		if tsBytes[off] != packet.SyncByte {
			t.pcrClock.parseErrors++
			return
		}
		pkt := (*packet.Packet)(tsBytes[off : off+packet.PacketSize])
		t.scanOneTsPacket(now, pkt)
	}
}

// scanTsPackets walks the given already-parsed TS packets, extracting PCR values and checking null-packet integrity.
// It does not perform continuity-counter/PID/PMT validation - that's tsTracker's job, fed separately via
// Track/TrackPackets so the caller controls whether/how re-parsing happens.
func (t *mediaTracker) scanTsPackets(now utc.UTC, pkts []*packet.Packet) {
	for _, pkt := range pkts {
		t.scanOneTsPacket(now, pkt)
	}
}

// scanOneTsPacket extracts a PCR value from pkt (feeding the PID correlator and the optional OnPcr callback) and, for
// a null-PID packet, checks that its payload is valid 0xFF padding.
func (t *mediaTracker) scanOneTsPacket(now utc.UTC, pkt *packet.Packet) {
	if pkt.IsNull() {
		if !isValidPadding(pkt) {
			t.faultyPaddingPackets++
		}
		return
	}
	if !pkt.HasAdaptationField() {
		return
	}
	af, err := pkt.AdaptationField()
	if err != nil {
		t.adaptationFieldErrors++
		return
	}
	hasPcr, err := af.HasPCR()
	if err != nil {
		t.adaptationFieldErrors++
		return
	} else if !hasPcr {
		return
	}
	pcr, err := af.PCR()
	if err != nil {
		t.adaptationFieldErrors++
		return
	}

	pid := pkt.PID()
	if t.cfg.OnPcr != nil {
		t.cfg.OnPcr(pid, pcr)
	}
	t.pcrClock.forPid(pid).record(now, pcr)
}

func (t *mediaTracker) Stats() *Stats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshot(true, utc.Now())
}

func (t *mediaTracker) PeriodStats() *Stats {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := utc.Now()
	s := t.snapshot(false, now)
	t.resetPeriod(now)
	return s
}

func (t *mediaTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.start, t.lastArrival, t.periodStart = utc.Zero, utc.Zero, utc.Zero
	t.totalPackets, t.totalBytes = 0, 0
	t.outageCount, t.outageTotal = 0, 0
	t.periodPackets, t.periodBytes = 0, 0
	t.ipdTotal = statsutil.Statistics[duration.Millis]{}
	t.ipdPeriod = statsutil.Statistics[duration.Millis]{}
	t.ipdHistTotal.Clear()
	t.ipdHistPeriod.Clear()
	t.rateTotal = statsutil.Statistics[float64]{}
	t.bitrateTotal = statsutil.Statistics[float64]{}
	t.smallPacketsDropped, t.rtcpPacketsDropped = 0, 0
	t.badPackets, t.incompletePackets, t.adaptationFieldErrors = 0, 0, 0
	t.faultyPaddingPackets, t.longHeaders = 0, 0

	if t.rtpClock != nil {
		t.rtpClock = newRtpCorrelator(t.cfg.MaxGaps)
	}
	t.pcrClock = newPcrCorrelator(t.cfg.MaxGaps)
	t.tsTracker.Reset()
}

// snapshot builds a Stats for the whole capture (total) or the current period, as of now. Must be called with t.mu
// held.
func (t *mediaTracker) snapshot(total bool, now utc.UTC) *Stats {
	if t.start.IsZero() {
		return &Stats{Source: t.streamId}
	}

	var elapsed time.Duration
	var packets, bytes uint64
	ipdStats, ipdHist := &t.ipdPeriod, t.ipdHistPeriod
	if total {
		elapsed = utc.Since(t.start)
		packets, bytes = t.totalPackets, t.totalBytes
		ipdStats, ipdHist = &t.ipdTotal, t.ipdHistTotal
	} else {
		elapsed = now.Sub(t.periodStart)
		packets, bytes = t.periodPackets, t.periodBytes
	}
	pps, bps := rates(packets, bytes, elapsed)

	var rate *RateStats
	if total {
		rate = &RateStats{
			PpsMean:   round(t.rateTotal.Mean, 1),
			PpsStddev: round(stddev(&t.rateTotal), 1),
			PpsMin:    round(t.rateTotal.Min, 1),
			PpsMax:    round(t.rateTotal.Max, 1),
			BpsMean:   uint64(t.bitrateTotal.Mean),
			BpsStddev: uint64(stddev(&t.bitrateTotal)),
			BpsMin:    uint64(t.bitrateTotal.Min),
			BpsMax:    uint64(t.bitrateTotal.Max),
		}
	} else if packets > 0 {
		t.rateTotal.Update(now, pps)
		t.bitrateTotal.Update(now, bps)
	}

	tsStats := t.tsTracker.Stats()
	s := &Stats{
		Source:  t.streamId,
		Elapsed: duration.Spec(elapsed).RoundTo(2),
		Packets: packets,
		Bytes:   bytes,
		Pps:     round(pps, 1),
		Bitrate: uint64(bps),
		Rate:    rate,
		Ipd:     newDistribution(ipdStats, ipdHist),
		Clocks:  t.clockStats(total),
		Outages: OutageStats{Count: t.outageCount, TotalMs: t.outageTotal},
		Errors:  t.errorStats(tsStats),
	}
	if total {
		s.Ts = tsStats
	} else {
		s.Window = duration.Spec(elapsed).RoundTo(2)
	}
	return s
}

func (t *mediaTracker) resetPeriod(now utc.UTC) {
	t.periodStart = now
	t.periodPackets, t.periodBytes = 0, 0
	t.ipdPeriod = statsutil.Statistics[duration.Millis]{}
	t.ipdHistPeriod.Clear()
	if t.rtpClock != nil {
		t.rtpClock.resetPeriod()
	}
	t.pcrClock.resetPeriod()
}

// clockStats builds a ClockStats for each active correlation: the RTP timestamp clock (if any) followed by one PCR
// entry per PCR-bearing PID.
func (t *mediaTracker) clockStats(total bool) []ClockStats {
	var stats []ClockStats
	if t.rtpClock != nil {
		stats = append(stats, t.rtpClock.report(total))
	}
	return append(stats, t.pcrClock.reports(total)...)
}

// errorStats summarizes the transport-stream validation errors and input-integrity conditions accumulated so far.
func (t *mediaTracker) errorStats(ts *mpegts.Stats) ErrorStats {
	r := ErrorStats{
		Total:                 ts.ErrorCount,
		SmallPacketsDropped:   t.smallPacketsDropped,
		RtcpPacketsDropped:    t.rtcpPacketsDropped,
		BadPackets:            t.badPackets,
		IncompletePackets:     t.incompletePackets,
		AdaptationFieldErrors: t.adaptationFieldErrors,
		FaultyPaddingPackets:  t.faultyPaddingPackets,
		LongHeaders:           t.longHeaders,
	}
	for _, stream := range ts.Streams {
		r.CcErrors += stream.CcErrors
		if stream.CcErrors > 0 {
			r.ByPid = append(r.ByPid, PidErrors{Pid: stream.Pid, CcErrors: stream.CcErrors})
		}
	}
	return r
}

// isValidPadding reports whether pkt's payload (bytes 4 through 187) is valid 0xFF padding.
func isValidPadding(pkt *packet.Packet) bool {
	for i := 4; i < packet.PacketSize; i++ {
		if pkt[i] != 0xFF {
			return false
		}
	}
	return true
}

// isRTCP heuristically identifies an RTCP packet by its payload-type byte, to distinguish a genuinely malformed small
// datagram from an RTCP packet misdirected onto the media socket.
func isRTCP(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	pt := data[1]
	return pt >= 200 && pt <= 204
}
