package tracker

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/mpegts"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/utc-go"
)

// clockCorrelator correlates packet arrival (wall clock) with a single media clock, whose samples are supplied by an
// enclosing extractor (rtpCorrelator or pcrPidCorrelator). It measures the delivery skew (arrival minus media time),
// the jitter (variation of that skew) and the long-term clock drift (rate error via linear regression of media on
// wall time). All arrival timestamps come from the caller (see sample), never from an internal clock read, so the
// measured skew/jitter reflect the caller's own notion of "arrival time" (e.g. true network-read time even when the
// caller processes packets from an intermediate queue).
type clockCorrelator struct {
	source string                    // "rtp" or "pcr"
	toWall func(int64) time.Duration // converts unwrapped media ticks to wall-clock duration

	// reference established from the first sample of the current continuous segment
	hasRef bool
	wall0  utc.UTC
	media0 int64 // unwrapped media ticks at the reference

	// linear-regression accumulators over the whole capture (x = wall seconds, y = media seconds)
	n, sumX, sumY, sumXX, sumXY float64

	samples         uint64
	discontinuities uint64
	parseErrors     uint64
	lastSkew        duration.Millis

	skewTotal  statsutil.Statistics[duration.Millis] // arrival-minus-media skew (whole capture)
	skewPeriod statsutil.Statistics[duration.Millis] // arrival-minus-media skew (current period)
}

func newClockCorrelator(source string, toWall func(int64) time.Duration) *clockCorrelator {
	return &clockCorrelator{source: source, toWall: toWall}
}

// discontinuity records a media-clock discontinuity (gap) and restarts the reference segment so the next sample
// re-establishes the arrival-vs-media reference.
func (c *clockCorrelator) discontinuity() {
	c.discontinuities++
	c.hasRef = false
}

// sample folds a single (arrival, media-ticks) observation into the skew statistics and regression accumulators. now
// is the caller-supplied arrival time.
func (c *clockCorrelator) sample(now utc.UTC, ticks int64) {
	if !c.hasRef {
		c.hasRef = true
		c.wall0 = now
		c.media0 = ticks
		return
	}
	wallDur := now.Sub(c.wall0)
	mediaDur := c.toWall(ticks - c.media0)
	skew := duration.Millis(wallDur - mediaDur)

	c.samples++
	c.lastSkew = skew
	c.skewTotal.Update(now, skew)
	c.skewPeriod.Update(now, skew)

	c.n++
	wall := wallDur.Seconds()
	media := mediaDur.Seconds()
	c.sumX += wall
	c.sumY += media
	c.sumXX += wall * wall
	c.sumXY += wall * media
}

func (c *clockCorrelator) resetPeriod() {
	c.skewPeriod = statsutil.Statistics[duration.Millis]{}
}

// driftPpm returns the clock rate error in parts-per-million, estimated as the slope of media time over wall time. A
// positive value means the media clock runs faster than wall clock (sender clock ahead of receiver).
func (c *clockCorrelator) driftPpm() float64 {
	if c.n < 2 {
		return 0
	}
	den := c.n*c.sumXX - c.sumX*c.sumX
	if den == 0 {
		return 0
	}
	slope := (c.n*c.sumXY - c.sumX*c.sumY) / den
	return (slope - 1) * 1e6
}

// report writes this correlator's clock stats into dst, so a caller can reuse the same ClockStats across calls
// instead of allocating a new one each time (see MediaTracker.Snapshot). It only sets the fields common to every
// clock source (rtp and pcr); the caller-specific fields (Pid, NumWraps, PacketCount, ErrorCount, Gaps,
// GapsOverflow) are left to rtpCorrelator.report/pcrCorrelator.reports, since which of those apply depends on the
// concrete correlator, not this base type.
func (c *clockCorrelator) report(dst *ClockStats, total bool) {
	skew := &c.skewPeriod
	if total {
		skew = &c.skewTotal
	}
	dst.Source = c.source
	dst.Samples = c.samples
	dst.CurrentSkewMs = c.lastSkew
	dst.SkewMinMs = skew.Min
	dst.SkewMeanMs = skew.Mean
	dst.SkewMaxMs = skew.Max
	dst.JitterMs = duration.Millis(stddev(skew))
	dst.DriftPpm = round(c.driftPpm(), 1)
	dst.Discontinuities = c.discontinuities
	dst.ParseErrors = c.parseErrors
}

// ---------------------------------------------------------------------------------------------------------------------

// rtpCorrelator extracts the media clock from RTP header timestamps. In addition to the skew/jitter/drift correlation
// (via the embedded clockCorrelator), it tracks RTP-level packet/error counts and a bounded list of detected gaps -
// filling the role that avpipe's now-superseded, always-zero SeqNumSkipTot/SeqNumSkipCount fields never actually did.
type rtpCorrelator struct {
	*clockCorrelator
	gap *rtp.GapDetector // RTP sequence/timestamp gap detection and unwrapping

	maxGaps      int
	packetCount  uint64
	errorCount   uint64
	gaps         []rtp.Gap
	gapsOverflow uint64
}

func newRtpCorrelator(maxGaps int) *rtpCorrelator {
	return &rtpCorrelator{
		clockCorrelator: newClockCorrelator("rtp", rtp.TicksToDuration),
		gap:             rtp.NewGapDetector(1, time.Second),
		maxGaps:         maxGaps,
	}
}

// record folds a single RTP packet's timestamp into the correlation statistics. now is the caller-supplied arrival
// time.
func (c *rtpCorrelator) record(now utc.UTC, seq uint16, ts uint32) {
	c.packetCount++
	seqCur, tsCur, err := c.gap.Detect(seq, ts)
	if err != nil {
		c.errorCount++
		c.recordGap(seqCur, tsCur)
		c.discontinuity()
		return
	}
	c.sample(now, tsCur)
}

// recordGap appends a Gap event, or - once maxGaps has been reached - only counts it (gapsOverflow), so a long-running
// stream with sustained gaps cannot grow this list without bound.
func (c *rtpCorrelator) recordGap(seq, ts int64) {
	if len(c.gaps) >= c.maxGaps {
		c.gapsOverflow++
		return
	}
	c.gaps = append(c.gaps, rtp.Gap{
		PacketNum: int(c.packetCount),
		Seq:       seq,
		SeqPrev:   c.gap.Sequence.Previous(),
		SeqDiff:   seq - c.gap.Sequence.Previous(),
		Ts:        ts,
		TsPrev:    c.gap.Timestamp.Previous(),
		TsDiff:    ts - c.gap.Timestamp.Previous(),
	})
}

// report writes this correlator's clock stats into dst, reusing dst.Gaps's backing array where capacity allows
// instead of the defensive copy this used to allocate unconditionally.
func (c *rtpCorrelator) report(dst *ClockStats, total bool) {
	c.clockCorrelator.report(dst, total)
	dst.PacketCount = c.packetCount
	dst.ErrorCount = c.errorCount
	dst.Gaps = append(dst.Gaps[:0], c.gaps...)
	dst.GapsOverflow = c.gapsOverflow
}

// ---------------------------------------------------------------------------------------------------------------------

// pcrCorrelator correlates arrival with the MPEG-TS PCR of each PCR-bearing PID. A multi-program transport stream
// carries a separate PCR reference per program, so every PID that carries a PCR is tracked independently, each with
// its own gap detector and correlation state.
type pcrCorrelator struct {
	byPid       map[int]*pcrPidCorrelator
	pids        []int // PIDs in discovery order, for stable reporting
	maxGaps     int
	parseErrors uint64 // TS-alignment errors (stream-level, not attributable to a specific program)
}

func newPcrCorrelator(maxGaps int) *pcrCorrelator {
	return &pcrCorrelator{byPid: map[int]*pcrPidCorrelator{}, maxGaps: maxGaps}
}

// pcrPidCorrelator is the PCR correlator for a single PID.
type pcrPidCorrelator struct {
	*clockCorrelator
	gap mpegts.PcrGapDetector // PCR gap detection and unwrapping
	pid int

	hasLastRaw bool
	lastRawPcr uint64
	numWraps   int64
}

func newPcrPidCorrelator(pid int) *pcrPidCorrelator {
	return &pcrPidCorrelator{
		clockCorrelator: newClockCorrelator("pcr",
			func(ticks int64) time.Duration { return mpegts.PcrToDuration(uint64(ticks)) }),
		gap: mpegts.PcrGapDetector{Threshold: mpegts.DurationToPcr(time.Second)},
		pid: pid,
	}
}

// forPid returns the correlator for the given PID, creating it the first time the PID is seen.
func (c *pcrCorrelator) forPid(pid int) *pcrPidCorrelator {
	p := c.byPid[pid]
	if p == nil {
		p = newPcrPidCorrelator(pid)
		c.byPid[pid] = p
		c.pids = append(c.pids, pid)
	}
	return p
}

func (c *pcrCorrelator) resetPeriod() {
	for _, pid := range c.pids {
		c.byPid[pid].resetPeriod()
	}
}

// reports builds one ClockStats per PCR-bearing PID, in discovery order, appending after whatever dst already holds
// (e.g. an rtp report at index 0 - see MediaTracker.clockStats) instead of allocating a new slice every call. PIDs
// are stable for a live stream, so position base+i usually already holds the right PID's ClockStats from the
// previous call. Stream-level TS-alignment parse errors are surfaced on the first program's report (they cannot be
// attributed to a specific program). Before any PCR is seen a single placeholder report is returned so parse errors
// remain visible.
func (c *pcrCorrelator) reports(dst []ClockStats, total bool) []ClockStats {
	base := len(dst)
	n := len(c.pids)
	if n == 0 {
		n = 1 // the placeholder entry
	}

	if cap(dst) < base+n {
		grown := make([]ClockStats, base+n)
		copy(grown, dst)
		dst = grown
	} else {
		dst = dst[:base+n]
	}

	if len(c.pids) == 0 {
		dst[base] = ClockStats{Source: "pcr", ParseErrors: c.parseErrors}
		return dst
	}

	for i, pid := range c.pids {
		p := c.byPid[pid]
		p.report(&dst[base+i], total)
		dst[base+i].Pid = p.pid
		dst[base+i].NumWraps = p.numWraps
	}
	dst[base].ParseErrors += c.parseErrors
	return dst
}

// record folds a single PCR sample into the PID's correlation statistics. now is the caller-supplied arrival time.
// It also detects true PCR wraparound (as opposed to reordering/encoder-bug backward jumps), counted in numWraps.
func (c *pcrPidCorrelator) record(now utc.UTC, pcr uint64) {
	if c.hasLastRaw && pcr < c.lastRawPcr && c.lastRawPcr-pcr > mpegts.MaxPCR/2 {
		c.numWraps++
	}
	c.hasLastRaw = true
	c.lastRawPcr = pcr

	_, cur, gap := c.gap.Detect(pcr)
	if gap {
		c.discontinuity()
		return
	}
	c.sample(now, cur)
}
