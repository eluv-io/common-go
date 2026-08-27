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

// SnapshotOptions controls which parts of a Snapshot call are populated, letting a caller skip computing fields it
// doesn't need.
type SnapshotOptions struct {
	// SkipTs, if true, skips mpegts.TsStreamTracker's per-PID walk and histogram capture - the most expensive part
	// of a snapshot. Stats.Ts is left nil; Errors.Total/CcErrors still populate from cheap running totals, but
	// Errors.ByPid (which requires the per-PID breakdown) is left empty.
	SkipTs bool
}

// MediaTracker tracks the timing and integrity of a live media stream. See the package doc for an overview.
type MediaTracker interface {
	// TrackDatagram processes one raw network datagram received at now (the datagram's arrival time), parsing the
	// RTP header (if the tracker is configured for RTP) and the contained MPEG-TS packets.
	TrackDatagram(now utc.UTC, datagram []byte) error
	// TrackPacket processes one already-decoded pooled packet received at now (the packet's arrival time), reusing
	// its already-parsed RTP/MPEG-TS layers instead of re-parsing pkt.Data.
	TrackPacket(now utc.UTC, pkt *pktpool.Packet) error
	// Stats returns the cumulative stats for the whole capture. Equivalent to Snapshot(&Stats{}, true, utc.Now(),
	// SnapshotOptions{}).
	Stats() *Stats
	// PeriodStats returns the stats for the current reporting period and resets the period window. Equivalent to
	// Snapshot(&Stats{}, false, utc.Now(), SnapshotOptions{}).
	PeriodStats() *Stats
	// Snapshot populates snap with the cumulative (total=true) or current-period (total=false) stats as of now,
	// reusing snap's existing slices (Clocks, each ClockStats.Gaps, Errors.ByPid) where possible instead of
	// allocating new ones - for a caller that polls periodically and wants to bound per-call garbage. When
	// total is false, this also resets the period window, exactly like PeriodStats. snap's nested slices are
	// invalidated by the next Snapshot/Stats/PeriodStats call that reuses the same snap; do not read them
	// concurrently with such a call, and see Stats.CopyInto to detach a copy that outlives the next call.
	Snapshot(snap *Stats, total bool, now utc.UTC, opts SnapshotOptions) *Stats
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
		rawPkt:        pktpool.NewRawPacket(nil),
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

	start       utc.UTC // arrival time of the first packet ever tracked - never reset by resetPeriod, only by Reset
	lastArrival utc.UTC // arrival time of the previous packet, used to compute the next inter-packet delay

	// cumulative counters, reset only by Reset (never by resetPeriod/PeriodStats/Snapshot(total: false)). These, and
	// their periodXxx counterparts below, are the fields recorded for every datagram exactly as received on the wire,
	// unconditionally, regardless of whether it's later classified as dropped/invalid. That is a *different* signal
	// from the validation/integrity counters further down (smallPacketsDropped, badPackets, etc.), which only increment
	// for a datagram/packet these have already counted here - "how much arrived" vs. "how much of what arrived was
	// valid media".
	totalPackets uint64          // every datagram TrackDatagram/TrackPacket has ever been called with
	totalBytes   uint64          // sum of every such datagram's length, in bytes
	outageCount  uint64          // number of inter-packet gaps >= cfg.OutageThreshold, ever
	outageTotal  duration.Millis // sum of those gaps' durations

	// cumulative distributions, reset only by Reset.
	ipdTotal     statsutil.Statistics[duration.Millis] // inter-packet-delay distribution over the whole capture
	ipdHistTotal *histogram.Histogram[duration.Millis] // same, as a histogram (for percentiles)
	rateTotal    statsutil.Statistics[float64]         // per-period packet rate (packets/s) - captures rate variation
	bitrateTotal statsutil.Statistics[float64]         // per-period bitrate (bits/s) - captures bitrate variation

	// current period counters and distributions: the periodXxx counterpart of each cumulative field above,
	// tracking the exact same thing but reset to zero by resetPeriod on each PeriodStats/Snapshot(total: false)
	// call, so a caller polling periodically gets "since I last asked" instead of "since the stream started".
	periodStart       utc.UTC                               // start of the current period
	periodPackets     uint64                                // totalPackets' counterpart, this period only
	periodBytes       uint64                                // totalBytes' counterpart, this period only
	periodOutageCount uint64                                // outageCount's counterpart, this period only
	periodOutageTotal duration.Millis                       // outageTotal's counterpart, this period only
	ipdPeriod         statsutil.Statistics[duration.Millis] // ipdTotal's counterpart, this period only
	ipdHistPeriod     *histogram.Histogram[duration.Millis] // ipdHistTotal's counterpart, this period only

	// arrival vs media-clock correlation. The PCR correlation is always present (extracted from the MPEG-TS
	// payload); for RTP sources the RTP-timestamp correlation is present too, so both clocks are correlated.
	rtpClock *rtpCorrelator // nil for raw MPEG-TS sources
	pcrClock *pcrCorrelator

	// tsTracker validates the transport stream and collects per-program/PID structure (stream types from the PMT,
	// packet counts, continuity-counter errors).
	tsTracker mpegts.TsStreamTracker

	// rawPkt is owned exclusively by this tracker and reused across TrackDatagram calls so it decodes without
	// allocating; it aliases the datagram passed to TrackDatagram only for the duration of that call - never
	// retained after trackLocked returns.
	rawPkt *pktpool.RawPacket

	// tsScratch is a reused destination for gathering tsTracker's stats when the result isn't exposed via
	// Stats.Ts itself (a period snapshot, which never sets Ts - see Snapshot) but is still needed to derive
	// Errors.Total/CcErrors/ByPid. Lazily allocated on first use.
	tsScratch *mpegts.Stats

	// counters for input validation/integrity conditions not covered by tsTracker/the clock correlators.
	// Cumulative only - no periodXxx counterpart, unlike the arrival counters above. Each of these increments for
	// a datagram/packet that recordArrival has *already* counted in totalPackets/totalBytes above; they're the
	// "how much of what arrived was actually valid media" half of that pair, not a separate arrival count.
	smallPacketsDropped   uint64 // datagram too small to plausibly contain a TS packet (+ RTP header, if configured)
	rtcpPacketsDropped    uint64 // subset of smallPacketsDropped that looks like a misdirected RTCP packet
	badPackets            uint64 // RTP header failed to parse, or carried an unsupported RTP version
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
	return t.trackLocked(now, datagram, t.rawPkt.Reset(datagram))
}

// TrackPacket processes one already-decoded pooled packet received at now (the packet's arrival time), reusing its
// already-parsed RTP/MPEG-TS layers - see pktpool.Packet's layer-decoding contract - instead of re-parsing pkt.Data.
func (t *mediaTracker) TrackPacket(now utc.UTC, pkt *pktpool.Packet) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.trackLocked(now, pkt.Data, pkt)
}

// trackLocked contains the RTP/MPEG-TS parsing and stats logic shared by TrackDatagram and TrackPacket - the two
// differ only in how they obtain a pktpool.Decoder for the datagram (a reused pktpool.RawPacket vs. an
// already-decoded pktpool.Packet), not in how that decoder's layers are processed. Must be called with t.mu held.
// datagram is src's full extent, used for the length checks below; src provides the actual layer decoding.
func (t *mediaTracker) trackLocked(now utc.UTC, datagram []byte, src pktpool.Decoder) error {
	t.recordArrival(now, len(datagram))

	if t.rtpClock != nil {
		if len(datagram) < rtpTsMinLen {
			t.smallPacketsDropped++
			if isRTCP(datagram) {
				t.rtcpPacketsDropped++
			}
			return nil
		}
		rtpLayer, err := src.Rtp()
		if err != nil {
			t.badPackets++
			t.rtpClock.parseErrors++
			return err
		}
		hdr := rtpLayer.Packet().Header
		if hdr.Version != 2 {
			t.badPackets++
			t.rtpClock.parseErrors++
			return errors.NoTrace("MediaTracker.trackLocked", errors.K.Invalid,
				"reason", "unsupported RTP version", "version", hdr.Version)
		}
		if t.cfg.OnRtpSample != nil {
			t.cfg.OnRtpSample(hdr.SequenceNumber, hdr.Timestamp)
		}
		if headerLen := len(datagram) - len(rtpLayer.Payload) - int(hdr.PaddingSize); headerLen != 12 {
			t.longHeaders++
		}
		if len(rtpLayer.Payload) < packet.PacketSize {
			// rtpTsMinLen above only bounds the whole datagram, not its payload. Drop the datagram if it doesn't at
			// least contain one TS packet.
			t.smallPacketsDropped++
			return nil
		}
		t.rtpClock.record(now, hdr.SequenceNumber, hdr.Timestamp)
	} else if len(datagram) < packet.PacketSize {
		t.smallPacketsDropped++
		return nil
	}

	tsLayer, err := src.Ts()
	if err != nil {
		t.incompletePackets++
		return err
	}
	tsPkts := tsLayer.Packets()

	_, errList := t.tsTracker.TrackPackets(tsPkts)
	t.scanTsPackets(now, tsPkts)
	return errList
}

// recordArrival folds a single packet's arrival into the packet/byte counters and the inter-packet-delay/outage
// statistics. now is the packet's arrival time, as supplied by the caller (never sampled internally), so callers
// whose packets pass through an intermediate queue (e.g. a channel) can supply the true network arrival time rather
// than the time TrackDatagram/TrackPacket happens to be invoked.
//
// trackLocked calls this unconditionally, before any of its own drop/validation checks - so Packets/Bytes/Pps/
// Bitrate/Ipd/Outages count every datagram exactly as received on the wire, including ones later classified as
// dropped or invalid (too small, malformed RTP, misdirected RTCP, ...). This is deliberate, not an oversight: it's
// the network-level arrival signal (useful for spotting e.g. a flood of garbage/misdirected traffic), distinct from
// "how much valid media arrived" - which is tracked separately via smallPacketsDropped/rtcpPacketsDropped/
// badPackets/incompletePackets/etc. Do not move this call after validation without considering both signals.
func (t *mediaTracker) recordArrival(now utc.UTC, n int) {
	if t.start.IsZero() {
		t.start = now
		t.periodStart = now
	}
	t.totalPackets++
	t.totalBytes += uint64(n)
	t.periodPackets++
	t.periodBytes += uint64(n)

	arrival := now
	if !t.lastArrival.IsZero() {
		if arrival.Before(t.lastArrival) {
			// This packet was delivered out of arrival-time order - e.g. a reorder-correction consumer released an
			// earlier-arriving packet after a later one. Its true incremental delay relative to delivery order is
			// undefined, so record it as 0 rather than a negative interval: every packet still contributes exactly
			// one IPD sample, keeping ipdTotal/ipdHistTotal/etc. in sync with totalPackets/periodPackets above.
			arrival = t.lastArrival
		}
		ipd := arrival.Sub(t.lastArrival)
		ipdMs := duration.Millis(ipd)
		t.ipdTotal.Update(now, ipdMs)
		t.ipdPeriod.Update(now, ipdMs)
		t.ipdHistTotal.Observe(ipdMs)
		t.ipdHistPeriod.Observe(ipdMs)
		if ipd >= t.cfg.OutageThreshold {
			t.outageCount++
			t.outageTotal += duration.Millis(ipd)
			t.periodOutageCount++
			t.periodOutageTotal += duration.Millis(ipd)
		}
	}
	// arrival is already clamped above, so this never regresses lastArrival.
	t.lastArrival = arrival
}

// scanTsPackets walks the given already-parsed TS packets, extracting PCR values and checking null-packet integrity.
// It does not perform continuity-counter/PID/PMT validation - that's tsTracker's job, fed separately via
// TrackPackets so the caller controls whether/how re-parsing happens.
func (t *mediaTracker) scanTsPackets(now utc.UTC, pkts []*packet.Packet) {
	for _, pkt := range pkts {
		t.scanOneTsPacket(now, pkt)
	}
}

// scanOneTsPacket extracts a PCR value from pkt (feeding the PID correlator and the optional OnPcr callback) and, for
// a null-PID packet, checks that its payload is valid 0xFF padding. A bad sync byte only drops this one packet (counted
// via pcrClock.parseErrors) - it does not abort scanning the rest of the caller's packet slice, mirroring how
// tsTracker.TrackPackets already treats a CheckErrors failure as per-packet, not per-datagram.
func (t *mediaTracker) scanOneTsPacket(now utc.UTC, pkt *packet.Packet) {
	if pkt[0] != packet.SyncByte {
		t.pcrClock.parseErrors++
		return
	}
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
	return t.Snapshot(&Stats{}, true, utc.Now(), SnapshotOptions{})
}

func (t *mediaTracker) PeriodStats() *Stats {
	return t.Snapshot(&Stats{}, false, utc.Now(), SnapshotOptions{})
}

func (t *mediaTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.start, t.lastArrival, t.periodStart = utc.Zero, utc.Zero, utc.Zero
	t.totalPackets, t.totalBytes = 0, 0
	t.outageCount, t.outageTotal = 0, 0
	t.periodPackets, t.periodBytes = 0, 0
	t.periodOutageCount, t.periodOutageTotal = 0, 0
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

// Snapshot populates snap with the cumulative (total=true) or current-period (total=false) stats as of now. See
// the MediaTracker interface doc for the reuse/reset contract.
func (t *mediaTracker) Snapshot(snap *Stats, total bool, now utc.UTC, opts SnapshotOptions) *Stats {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.start.IsZero() {
		*snap = Stats{Source: t.streamId}
		return snap
	}

	var elapsed time.Duration
	var packets, bytes uint64
	ipdStats, ipdHist := &t.ipdPeriod, t.ipdHistPeriod
	if total {
		elapsed = now.Sub(t.start)
		packets, bytes = t.totalPackets, t.totalBytes
		ipdStats, ipdHist = &t.ipdTotal, t.ipdHistTotal
	} else {
		elapsed = now.Sub(t.periodStart)
		packets, bytes = t.periodPackets, t.periodBytes
	}
	pps, bps := rates(packets, bytes, elapsed)

	if total {
		if snap.Rate == nil {
			snap.Rate = &RateStats{}
		}
		snap.Rate.PpsMean = round(t.rateTotal.Mean, 1)
		snap.Rate.PpsStddev = round(stddev(&t.rateTotal), 1)
		snap.Rate.PpsMin = round(t.rateTotal.Min, 1)
		snap.Rate.PpsMax = round(t.rateTotal.Max, 1)
		snap.Rate.BpsMean = uint64(t.bitrateTotal.Mean)
		snap.Rate.BpsStddev = uint64(stddev(&t.bitrateTotal))
		snap.Rate.BpsMin = uint64(t.bitrateTotal.Min)
		snap.Rate.BpsMax = uint64(t.bitrateTotal.Max)
	} else {
		snap.Rate = nil // period snapshots never populate Rate, same as today
		if packets > 0 {
			t.rateTotal.Update(now, pps)
			t.bitrateTotal.Update(now, bps)
		}
	}

	// Gather tsTracker's stats into snap.Ts itself when it will be exposed (total, not skipped), or into a
	// private scratch buffer otherwise (period snapshots never expose Ts; SkipTs skips the expensive walk
	// either way) - errorStats still needs the result either way to derive Errors.Total/CcErrors/ByPid.
	var tsStats *mpegts.Stats
	if total && !opts.SkipTs {
		if snap.Ts == nil {
			snap.Ts = &mpegts.Stats{}
		}
		tsStats = snap.Ts
	} else {
		snap.Ts = nil
		if t.tsScratch == nil {
			t.tsScratch = &mpegts.Stats{}
		}
		tsStats = t.tsScratch
	}
	t.tsTracker.Snapshot(tsStats, !opts.SkipTs)

	snap.Source = t.streamId
	snap.Elapsed = duration.Spec(elapsed).RoundTo(2)
	snap.Packets = packets
	snap.Bytes = bytes
	snap.Pps = round(pps, 1)
	snap.Bitrate = uint64(bps)
	snap.Ipd = newDistribution(ipdStats, ipdHist)
	snap.Clocks = t.clockStats(snap.Clocks, total)
	if total {
		snap.Outages = OutageStats{Count: t.outageCount, TotalMs: t.outageTotal}
	} else {
		snap.Outages = OutageStats{Count: t.periodOutageCount, TotalMs: t.periodOutageTotal}
	}
	t.errorStats(&snap.Errors, tsStats)
	if total {
		snap.Window = 0
	} else {
		snap.Window = duration.Spec(elapsed).RoundTo(2)
		t.resetPeriod(now)
	}
	return snap
}

func (t *mediaTracker) resetPeriod(now utc.UTC) {
	t.periodStart = now
	t.periodPackets, t.periodBytes = 0, 0
	t.periodOutageCount, t.periodOutageTotal = 0, 0
	t.ipdPeriod = statsutil.Statistics[duration.Millis]{}
	t.ipdHistPeriod.Clear()
	if t.rtpClock != nil {
		t.rtpClock.resetPeriod()
	}
	t.pcrClock.resetPeriod()
}

// clockStats builds a ClockStats for each active correlation into dst (reused across calls, see Snapshot): the RTP
// timestamp clock (if any) followed by one PCR entry per PCR-bearing PID.
func (t *mediaTracker) clockStats(dst []ClockStats, total bool) []ClockStats {
	dst = dst[:0]
	if t.rtpClock != nil {
		// Grow back to length 1 without overwriting index 0 with a zero ClockStats{} - doing so would clobber that
		// entry's Gaps slice (and its backing array) if this dst is being reused across calls, forcing report's own
		// append(dst.Gaps[:0], ...) to reallocate on every single call instead of reusing it.
		if cap(dst) >= 1 {
			dst = dst[:1]
		} else {
			dst = append(dst, ClockStats{})
		}
		t.rtpClock.report(&dst[0], total)
	}
	// pcrClock.reports appends after whatever's already in dst (the rtp report, if any), starting at len(dst).
	return t.pcrClock.reports(dst, total)
}

// errorStats summarizes the transport-stream validation errors and input-integrity conditions accumulated so far
// into dst (reused across calls, see Snapshot). ts.CcErrors/ts.Streams reflect the cheap running total / the
// per-PID walk respectively - see TsStreamTracker.Snapshot for when the latter is populated.
func (t *mediaTracker) errorStats(dst *ErrorStats, ts *mpegts.Stats) {
	dst.Total = ts.ErrorCount
	dst.CcErrors = ts.CcErrors
	dst.SmallPacketsDropped = t.smallPacketsDropped
	dst.RtcpPacketsDropped = t.rtcpPacketsDropped
	dst.BadPackets = t.badPackets
	dst.IncompletePackets = t.incompletePackets
	dst.AdaptationFieldErrors = t.adaptationFieldErrors
	dst.FaultyPaddingPackets = t.faultyPaddingPackets
	dst.LongHeaders = t.longHeaders

	dst.ByPid = dst.ByPid[:0]
	for _, stream := range ts.Streams {
		if stream.CcErrors > 0 {
			dst.ByPid = append(dst.ByPid, PidErrors{Pid: stream.Pid, CcErrors: stream.CcErrors})
		}
	}
}

// isValidPadding reports whether pkt's payload is valid 0xFF padding. A null-PID packet is not required to be
// payload-only (adaptation_field_control 01) - the spec permits an adaptation field here too (10/11), in which case
// the payload starts later than byte 4, or there may be no payload at all.
func isValidPadding(pkt *packet.Packet) bool {
	payloadStart := 4
	if pkt.HasAdaptationField() {
		af, err := pkt.AdaptationField()
		if err != nil {
			return false
		}
		payloadStart = 4 + 1 + af.Length() // TS header + the length byte itself + its content
	}
	if !pkt.HasPayload() {
		return true // adaptation-field-only null packet: no payload bytes to validate
	}
	for i := payloadStart; i < packet.PacketSize; i++ {
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
