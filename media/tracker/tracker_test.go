package tracker

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/mpegts"
	"github.com/eluv-io/common-go/media/pktpool"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/utc-go"
)

// TestClockCorrelator_Drift feeds a perfectly regular media clock that runs 100 ppm faster than wall clock and
// verifies the estimated drift and skew. Inputs are exact, so the assertions are tight; it documents the sign
// convention clockCorrelator uses.
func TestClockCorrelator_Drift(t *testing.T) {
	c := newClockCorrelator("rtp", rtp.TicksToDuration) // RTP: 90 kHz clock

	base := utc.Now()
	const ticksPerSecond = 90009 // 90000 ticks/s + 100 ppm

	for i := 0; i <= 10; i++ {
		c.sample(base.Add(time.Duration(i)*time.Second), int64(i*ticksPerSecond))
	}

	require.EqualValues(t, 10, c.samples)
	require.InDelta(t, 100, c.driftPpm(), 0.5, "media clock runs ~100 ppm fast")

	// media advances faster than wall, so arrival falls progressively behind media => negative skew.
	require.Less(t, c.skewTotal.Mean, duration.Millis(0))
	require.InDelta(t, -1.0, c.lastSkew.AsFloat(), 0.01, "at t=10s the skew is -10s*100ppm = -1ms")
}

// TestMediaTracker_RawTs drives a raw (non-RTP) MPEG-TS source and verifies the aggregate stats and the PCR
// correlation.
func TestMediaTracker_RawTs(t *testing.T) {
	tr := NewMediaTracker("test", Config{})

	base := utc.Now()
	var pcr uint64 = 1_000_000
	const n = 20
	for i := 0; i < n; i++ {
		dg, _ := tsDatagramWithPCR(pcr)
		// The synthetic fixture reuses a fixed continuity counter, so from the second datagram onward
		// TrackDatagram legitimately returns a continuity-counter error - not asserted here, see
		// TestMediaTracker_MultiProgramPCR for continuity-counter-error coverage.
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	s := tr.Stats()
	require.EqualValues(t, n, s.Packets)
	require.Greater(t, s.Pps, 0.0)
	require.Greater(t, s.Bitrate, uint64(0))
	require.Greater(t, s.Ipd.Count, uint64(0))
	require.Len(t, s.Clocks, 1, "raw TS source has only the PCR correlation")
	require.Equal(t, "pcr", s.Clocks[0].Source)
	require.Greater(t, s.Clocks[0].Samples, uint64(0), "PCR samples must be correlated")
}

// TestMediaTracker_RTP drives an RTP-wrapped MPEG-TS source and verifies that both the RTP-timestamp and PCR clocks
// are correlated.
func TestMediaTracker_RTP(t *testing.T) {
	tr := NewMediaTracker("test", Config{Rtp: true})

	base := utc.Now()
	var (
		seq     uint16
		rtpTs   uint32 = 1_000
		pcrBase uint64 = 1_000_000
	)
	const n = 20
	for i := 0; i < n; i++ {
		dg, _ := tsDatagramWithPCR(pcrBase)
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), rtpWrap(seq, rtpTs, dg))
		seq++
		rtpTs += 180 // 90 kHz clock advanced over the 2 ms send interval
		pcrBase += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	s := tr.Stats()
	require.EqualValues(t, n, s.Packets)
	require.Len(t, s.Clocks, 2, "RTP source correlates both the RTP timestamp and the PCR clock")

	clocks := map[string]ClockStats{}
	for _, c := range s.Clocks {
		clocks[c.Source] = c
	}
	require.Contains(t, clocks, "rtp")
	require.Contains(t, clocks, "pcr")
	require.Greater(t, clocks["rtp"].Samples, uint64(0), "RTP timestamp samples must be correlated")
	require.Greater(t, clocks["pcr"].Samples, uint64(0), "PCR samples must be correlated")
	require.EqualValues(t, n, clocks["rtp"].PacketCount, "rtp ClockStats tracks its own packet count")
}

// TestMediaTracker_MultiProgramPCR drives a multi-program transport stream and verifies that both PCR-bearing PIDs
// are correlated independently.
func TestMediaTracker_MultiProgramPCR(t *testing.T) {
	tr := NewMediaTracker("test", Config{})

	base := utc.Now()
	pcrA, pcrB := uint64(1_000_000), uint64(50_000_000)
	const n = 20
	for i := 0; i < n; i++ {
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), tsDatagramTwoPrograms(pcrA, pcrB))
		step := mpegts.DurationToPcr(2 * time.Millisecond)
		pcrA += step
		pcrB += step
	}

	s := tr.Stats()
	require.Len(t, s.Clocks, 2, "each PCR-bearing PID is correlated independently")

	byPid := map[int]ClockStats{}
	for _, c := range s.Clocks {
		require.Equal(t, "pcr", c.Source)
		byPid[c.Pid] = c
	}
	require.Contains(t, byPid, 0x100)
	require.Contains(t, byPid, 0x200)
	require.Greater(t, byPid[0x100].Samples, uint64(0))
	require.Greater(t, byPid[0x200].Samples, uint64(0))

	require.NotNil(t, s.Ts, "TS program stats must be collected")
	require.Greater(t, s.Ts.PacketCount, 0)
	tsPids := map[int]int{}
	for _, st := range s.Ts.Streams {
		tsPids[st.Pid] = st.PacketCount
	}
	require.Contains(t, tsPids, 0x100)
	require.Contains(t, tsPids, 0x200)

	// the synthetic sender's fixed continuity counter yields errors, reported by PID
	ccByPid := map[int]int{}
	for _, e := range s.Errors.ByPid {
		ccByPid[e.Pid] = e.CcErrors
	}
	require.Contains(t, ccByPid, 0x100)
	require.Contains(t, ccByPid, 0x200)
	require.Greater(t, s.Errors.Total, 0)
	require.Greater(t, s.Errors.CcErrors, 0)
}

// TestMediaTracker_PeriodStats verifies that PeriodStats reports only the current window and resets it, while Stats
// keeps reporting the cumulative totals.
func TestMediaTracker_PeriodStats(t *testing.T) {
	tr := NewMediaTracker("test", Config{})

	base := utc.Now()
	var pcr uint64 = 1_000_000
	for i := 0; i < 5; i++ {
		dg, _ := tsDatagramWithPCR(pcr)
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	p1 := tr.PeriodStats()
	require.EqualValues(t, 5, p1.Packets)
	require.NotZero(t, p1.Window)

	for i := 5; i < 8; i++ {
		dg, _ := tsDatagramWithPCR(pcr)
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	p2 := tr.PeriodStats()
	require.EqualValues(t, 3, p2.Packets, "period counters reset after the previous PeriodStats call")

	total := tr.Stats()
	require.EqualValues(t, 8, total.Packets, "Stats keeps reporting the cumulative total")
}

// TestMediaTracker_TrackPacket verifies that feeding the same bytes via TrackPacket (the zero-reparse entry point
// used by a caller that already holds a decoded pktpool.Packet) produces the same aggregate stats as feeding them
// via TrackDatagram (the raw-bytes entry point used by a caller with no pktpool dependency).
func TestMediaTracker_TrackPacket(t *testing.T) {
	base := utc.Now()
	var seq uint16
	var rtpTs uint32 = 1_000
	var pcrBase uint64 = 1_000_000

	datagrams := make([][]byte, 0, 10)
	for i := 0; i < 10; i++ {
		dg, _ := tsDatagramWithPCR(pcrBase)
		datagrams = append(datagrams, rtpWrap(seq, rtpTs, dg))
		seq++
		rtpTs += 180
		pcrBase += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	// The synthetic fixture reuses a fixed continuity counter, so from the second datagram onward both trackers
	// legitimately return a continuity-counter error; what matters here is that they agree (checked below via the
	// aggregated Ts.ErrorCount), not that either returns nil.
	viaDatagram := NewMediaTracker("test", Config{Rtp: true})
	for i, dg := range datagrams {
		_ = viaDatagram.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
	}

	viaPacket := NewMediaTracker("test", Config{Rtp: true})
	for i, dg := range datagrams {
		p := pktpool.NewPacket(0, len(dg))
		require.NoError(t, p.From(dg))
		_ = viaPacket.TrackPacket(base.Add(time.Duration(i)*2*time.Millisecond), p)
	}

	sd, sp := viaDatagram.Stats(), viaPacket.Stats()
	require.Equal(t, sd.Packets, sp.Packets)
	require.Equal(t, sd.Bytes, sp.Bytes)
	require.Equal(t, len(sd.Clocks), len(sp.Clocks))
	for i := range sd.Clocks {
		require.Equal(t, sd.Clocks[i].Source, sp.Clocks[i].Source)
		require.Equal(t, sd.Clocks[i].Samples, sp.Clocks[i].Samples)
	}
	require.Equal(t, sd.Ts.PacketCount, sp.Ts.PacketCount)
	require.Equal(t, sd.Ts.ErrorCount, sp.Ts.ErrorCount)
}

// TestMediaTracker_SmallPacketsDropped verifies that undersized datagrams are dropped and counted, distinguishing a
// misdirected RTCP packet from a plain malformed one - logic ported from avpipe's MpegtsPacketProcessor.
func TestMediaTracker_SmallPacketsDropped(t *testing.T) {
	tr := NewMediaTracker("test", Config{Rtp: true})

	// too small to be RTP-wrapped MPEG-TS, and not RTCP-shaped
	require.NoError(t, tr.TrackDatagram(utc.Now(), []byte{0x80, 0x00}))
	s := tr.Stats()
	require.EqualValues(t, 1, s.Errors.SmallPacketsDropped)
	require.EqualValues(t, 0, s.Errors.RtcpPacketsDropped)

	// small and RTCP-shaped (payload type 200 = SR)
	require.NoError(t, tr.TrackDatagram(utc.Now(), []byte{0x80, 200}))
	s = tr.Stats()
	require.EqualValues(t, 2, s.Errors.SmallPacketsDropped)
	require.EqualValues(t, 1, s.Errors.RtcpPacketsDropped)
}

// TestMediaTracker_LongHeaders verifies that an RTP header longer than the expected 12 bytes (an extension-bearing
// packet) is counted, both via TrackDatagram and TrackPacket.
func TestMediaTracker_LongHeaders(t *testing.T) {
	dg, _ := tsDatagramWithPCR(1_000_000)
	pkt := rtpWrapWithExtension(0, 1000, dg)

	tr := NewMediaTracker("test", Config{Rtp: true})
	require.NoError(t, tr.TrackDatagram(utc.Now(), pkt))
	require.EqualValues(t, 1, tr.Stats().Errors.LongHeaders)

	tr2 := NewMediaTracker("test", Config{Rtp: true})
	p := pktpool.NewPacket(0, len(pkt))
	require.NoError(t, p.From(pkt))
	require.NoError(t, tr2.TrackPacket(utc.Now(), p))
	require.EqualValues(t, 1, tr2.Stats().Errors.LongHeaders)
}

// TestMediaTracker_FaultyPaddingPackets verifies that a null-PID packet whose payload is not valid 0xFF padding is
// counted - logic ported from avpipe's MpegtsPacketProcessor/RemoveTsPadding.
func TestMediaTracker_FaultyPaddingPackets(t *testing.T) {
	dg, _ := tsDatagramWithPCR(1_000_000)
	// corrupt one byte of the trailing null packet's payload (offset: 1 PCR packet + 3 null packets in, byte 5 of
	// the payload)
	dg[packet.PacketSize*4+5] = 0x00

	tr := NewMediaTracker("test", Config{})
	require.NoError(t, tr.TrackDatagram(utc.Now(), dg))
	require.EqualValues(t, 1, tr.Stats().Errors.FaultyPaddingPackets)
}

// TestMediaTracker_ValidPadding_AdaptationField is a regression test for isValidPadding unconditionally treating
// bytes 4..187 as payload: a null-PID packet is not required to be payload-only (adaptation_field_control 01) - it
// may legally carry an adaptation field too (11), in which case the real payload starts later than byte 4.
func TestMediaTracker_ValidPadding_AdaptationField(t *testing.T) {
	valid := append(tsPcrPacket(0x100, 1_000_000), tsNullPacketWithAdaptationField()...)
	valid = append(valid, tsNullPacket()...)

	tr := NewMediaTracker("test", Config{})
	require.NoError(t, tr.TrackDatagram(utc.Now(), valid))
	require.EqualValues(t, 0, tr.Stats().Errors.FaultyPaddingPackets,
		"a well-formed adaptation-field-bearing null packet must not be flagged as faulty")

	corrupted := append(tsPcrPacket(0x100, 1_000_000), tsNullPacketWithAdaptationField()...)
	corrupted[packet.PacketSize+6] = 0x00 // corrupt the first payload byte, after the adaptation field
	corrupted = append(corrupted, tsNullPacket()...)

	tr2 := NewMediaTracker("test", Config{})
	require.NoError(t, tr2.TrackDatagram(utc.Now(), corrupted))
	require.EqualValues(t, 1, tr2.Stats().Errors.FaultyPaddingPackets,
		"corruption after the adaptation field must still be detected")
}

// TestMediaTracker_NumWraps verifies that a true PCR wraparound (as opposed to a small backward jump from
// reordering) is counted - logic ported from avpipe's MpegtsPacketProcessor.updatePCR.
func TestMediaTracker_NumWraps(t *testing.T) {
	tr := NewMediaTracker("test", Config{})
	base := utc.Now()

	// approach MaxPCR, then wrap back down close to 0. As in the other multi-call tests, the synthetic fixture's
	// fixed continuity counter legitimately yields a continuity-counter error from the second datagram onward.
	near := mpegts.MaxPCR - mpegts.DurationToPcr(4*time.Millisecond)
	dg1, _ := tsDatagramWithPCR(near)
	_ = tr.TrackDatagram(base, dg1)
	dg2, _ := tsDatagramWithPCR(near + mpegts.DurationToPcr(2*time.Millisecond))
	_ = tr.TrackDatagram(base.Add(2*time.Millisecond), dg2)
	wrapped := (near + mpegts.DurationToPcr(4*time.Millisecond)) % (mpegts.MaxPCR + 1)
	dg3, _ := tsDatagramWithPCR(wrapped)
	_ = tr.TrackDatagram(base.Add(4*time.Millisecond), dg3)

	s := tr.Stats()
	require.Len(t, s.Clocks, 1)
	require.EqualValues(t, 1, s.Clocks[0].NumWraps)
}

// TestMediaTracker_ParityMalformedInputs feeds a battery of malformed/edge-case datagrams through both TrackDatagram
// and TrackPacket, asserting they produce identical ErrorStats. This is a regression test for a real divergence
// found when the two entry points had independent parsing paths: TrackDatagram used raw pion (rtp.ParsePacket),
// which never validates the RTP version field, while TrackPacket explicitly rejected a bad version - so a
// malformed-version datagram was silently accepted (and polluted clock stats) via TrackDatagram while being
// correctly rejected via TrackPacket. Both entry points now share trackLocked, closing the gap by construction.
func TestMediaTracker_ParityMalformedInputs(t *testing.T) {
	tsOK, _ := tsDatagramWithPCR(1_000_000)

	tsTruncated := append([]byte{}, tsOK[:189]...) // 1 full TS packet + 1 stray byte: not a multiple of 188

	cases := []struct {
		name string
		dg   []byte
	}{
		{"bad RTP version", rtpWrapVersion(1, 0, 1000, tsOK)},
		{"TS payload not multiple of 188", rtpWrap(0, 1000, tsTruncated)},
		{"undersized, not RTCP-shaped", []byte{0x80, 0x00}},
		{"undersized, RTCP-shaped", []byte{0x80, 200}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			trDg := NewMediaTracker("test", Config{Rtp: true})
			errDg := trDg.TrackDatagram(utc.Now(), c.dg)

			trPkt := NewMediaTracker("test", Config{Rtp: true})
			p := pktpool.NewPacket(0, len(c.dg))
			require.NoError(t, p.From(c.dg))
			errPkt := trPkt.TrackPacket(utc.Now(), p)

			require.Equal(t, errDg == nil, errPkt == nil, "error-or-not must agree between TrackDatagram and TrackPacket")
			require.Equal(t, trDg.Stats().Errors, trPkt.Stats().Errors)
		})
	}
}

// TestMediaTracker_BadSyncByte verifies that a corrupted TS sync byte mid-datagram is handled identically by
// TrackDatagram and TrackPacket, and that it only drops the one bad packet from the PCR/padding scan (not the rest
// of the datagram) - a second divergence found during the same review: TrackPacket's scanTsPackets had no sync-byte
// check at all before this fix, unlike TrackDatagram's (now-deleted) scanTsBytes. Note tsTracker.TrackPackets
// independently flags the same bad packet via CheckErrors (sync-byte validation is one of its checks), so both
// TrackDatagram and TrackPacket still return an error here regardless of the scanOneTsPacket fix below - what this
// test locks in is that the PCR scan itself skips only the one bad packet and keeps processing the rest.
func TestMediaTracker_BadSyncByte(t *testing.T) {
	base := utc.Now()
	pcr := uint64(1_000_000)
	nextDatagram := func(corrupt bool) []byte {
		dg, _ := tsDatagramWithPCR(pcr) // 7 TS packets, the first (PID 0x100) carries the PCR
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
		if corrupt {
			dg[packet.PacketSize*3] = 0x00 // corrupt the sync byte of the 4th packet only
		}
		return rtpWrap(0, 1000, dg)
	}

	// Two clean datagrams first, to establish the PCR correlator's reference and get one real sample (sample()
	// consumes its first call just arming the reference - see clockCorrelator.sample), then a third with a
	// corrupted sync byte.
	dgs := [][]byte{nextDatagram(false), nextDatagram(false), nextDatagram(true)}

	trDg := NewMediaTracker("test", Config{Rtp: true})
	var errDg error
	for i, dg := range dgs {
		errDg = trDg.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
	}

	trPkt := NewMediaTracker("test", Config{Rtp: true})
	var errPkt error
	for i, dg := range dgs {
		p := pktpool.NewPacket(0, len(dg))
		require.NoError(t, p.From(dg))
		errPkt = trPkt.TrackPacket(base.Add(time.Duration(i)*2*time.Millisecond), p)
	}

	require.Error(t, errDg, "tsTracker.TrackPackets flags the bad sync byte via CheckErrors")
	require.Error(t, errPkt)

	for _, tr := range []MediaTracker{trDg, trPkt} {
		s := tr.Stats()
		var pcrClock *ClockStats
		for i := range s.Clocks {
			if s.Clocks[i].Source == "pcr" {
				pcrClock = &s.Clocks[i]
			}
		}
		require.NotNil(t, pcrClock)
		require.EqualValues(t, 1, pcrClock.ParseErrors, "exactly the one corrupted packet is counted")
		require.EqualValues(t, 2, pcrClock.Samples,
			"the PCR-bearing packet in the 3rd (corrupted) datagram is still processed despite the bad packet elsewhere in it")
	}
}

// TestMediaTracker_TrackDatagram_Unbounded verifies TrackDatagram accepts a datagram far larger than any fixed
// buffer capacity would allow - the property RawPacket gives "for free" (no backing buffer, unlike the
// Wrap-on-Packet alternative considered and rejected during design, which would have needed a capacity limit).
func TestMediaTracker_TrackDatagram_Unbounded(t *testing.T) {
	var large []byte
	for i := 0; i < 1000; i++ { // ~188KB, well beyond any typical UDP/RTP datagram
		large = append(large, tsNullPacket()...)
	}

	tr := NewMediaTracker("test", Config{})
	require.NoError(t, tr.TrackDatagram(utc.Now(), large))
	require.EqualValues(t, 1000, tr.Stats().Ts.PacketCount)
}

// TestMediaTracker_Snapshot_ReusesSlices verifies that calling Snapshot repeatedly with the same destination reuses
// the Clocks slice, each ClockStats.Gaps slice, and Errors.ByPid's backing array instead of reallocating, as long as
// the shape (RTP + PCR-bearing PIDs, retained gaps) is stable across calls - the common case for a live stream.
func TestMediaTracker_Snapshot_ReusesSlices(t *testing.T) {
	tr := NewMediaTracker("test", Config{Rtp: true})
	base := utc.Now()
	var seq uint16
	var rtpTs uint32 = 1_000
	var pcr uint64 = 1_000_000

	track := func() {
		dg, _ := tsDatagramWithPCR(pcr)
		_ = tr.TrackDatagram(base, rtpWrap(seq, rtpTs, dg))
		seq++
		rtpTs += 180
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
		base = base.Add(2 * time.Millisecond)
	}
	for i := 0; i < 3; i++ {
		track()
	}

	snap := &Stats{}
	tr.Snapshot(snap, true, base, SnapshotOptions{})
	require.Len(t, snap.Clocks, 2)
	clocksArray := snap.Clocks
	gapsArray := snap.Clocks[0].Gaps // rtp entry always at index 0 when Config.Rtp is set

	for i := 0; i < 3; i++ {
		track()
	}
	tr.Snapshot(snap, true, base, SnapshotOptions{})
	require.Len(t, snap.Clocks, 2)
	require.Same(t, &clocksArray[0], &snap.Clocks[0], "Clocks' backing array is reused, not reallocated")
	if len(snap.Clocks[0].Gaps) > 0 && len(gapsArray) > 0 {
		require.Same(t, &gapsArray[0], &snap.Clocks[0].Gaps[0], "Gaps' backing array is reused, not reallocated")
	}
}

// TestMediaTracker_Snapshot_PeriodResets verifies Snapshot(total=false) resets the period window exactly like
// PeriodStats does - the defining behavior a reuse-minded caller (e.g. content-fabric's probe command) still needs
// when it calls Snapshot directly instead of PeriodStats.
func TestMediaTracker_Snapshot_PeriodResets(t *testing.T) {
	tr := NewMediaTracker("test", Config{})
	base := utc.Now()
	var pcr uint64 = 1_000_000
	for i := 0; i < 5; i++ {
		dg, _ := tsDatagramWithPCR(pcr)
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	snap := &Stats{}
	tr.Snapshot(snap, false, base, SnapshotOptions{})
	require.EqualValues(t, 5, snap.Packets)

	for i := 5; i < 8; i++ {
		dg, _ := tsDatagramWithPCR(pcr)
		_ = tr.TrackDatagram(base.Add(time.Duration(i)*2*time.Millisecond), dg)
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	tr.Snapshot(snap, false, base, SnapshotOptions{})
	require.EqualValues(t, 3, snap.Packets, "period counters reset after the previous Snapshot(total=false) call")

	require.EqualValues(t, 8, tr.Stats().Packets, "Stats keeps reporting the cumulative total")
}

// TestMediaTracker_Snapshot_TotalElapsedUsesCallerNow is a regression test for a bug where the total-path branch of
// Snapshot read the real wall clock (utc.Since(t.start)) instead of the caller-supplied now, breaking determinism
// for any caller/test passing a synthetic now - and silently diverging from the period branch a few lines below,
// which already used now correctly.
func TestMediaTracker_Snapshot_TotalElapsedUsesCallerNow(t *testing.T) {
	tr := NewMediaTracker("test", Config{})
	start := utc.Now()
	dg, _ := tsDatagramWithPCR(1_000_000)
	_ = tr.TrackDatagram(start, dg)

	// A synthetic "now" far from the real wall clock - only correct if Snapshot actually uses it.
	syntheticNow := start.Add(time.Hour)
	snap := &Stats{}
	tr.Snapshot(snap, true, syntheticNow, SnapshotOptions{})

	require.Equal(t, duration.Spec(time.Hour).RoundTo(2), snap.Elapsed,
		"Elapsed must be derived from the caller-supplied now, not the real wall clock")
}

// TestMediaTracker_Snapshot_PeriodOutagesReset verifies that a period snapshot's Outages reflects only the current
// period, resetting after each period snapshot like Packets/Bytes/Ipd already do - a regression test for outages
// previously always being reported from the cumulative (never-reset) counters even for total=false.
func TestMediaTracker_Snapshot_PeriodOutagesReset(t *testing.T) {
	tr := NewMediaTracker("test", Config{OutageThreshold: 10 * time.Millisecond})
	base := utc.Now()
	var pcr uint64 = 1_000_000

	track := func(at utc.UTC) {
		dg, _ := tsDatagramWithPCR(pcr)
		_ = tr.TrackDatagram(at, dg)
		pcr += mpegts.DurationToPcr(2 * time.Millisecond)
	}

	track(base)
	track(base.Add(20 * time.Millisecond)) // gap >= OutageThreshold: one outage

	p1 := tr.PeriodStats()
	require.EqualValues(t, 1, p1.Outages.Count, "period reports the outage that happened during this period")
	require.NotZero(t, p1.Outages.TotalMs)

	track(base.Add(21 * time.Millisecond)) // 1ms gap: below OutageThreshold, no new outage

	p2 := tr.PeriodStats()
	require.Zero(t, p2.Outages.Count, "period outages reset after the previous PeriodStats call")
	require.Zero(t, p2.Outages.TotalMs)

	require.EqualValues(t, 1, tr.Stats().Outages.Count, "total snapshot keeps reporting the cumulative outage count")
}

// TestMediaTracker_Snapshot_SkipTs verifies SkipTs leaves Ts nil and Errors.ByPid empty, while Errors.Total/CcErrors
// still populate correctly from mpegts.TsStreamTracker's cheap running totals.
func TestMediaTracker_Snapshot_SkipTs(t *testing.T) {
	tr := NewMediaTracker("test", Config{})
	base := utc.Now()
	dg, _ := tsDatagramWithPCR(1_000_000)
	_ = tr.TrackDatagram(base, dg)
	// A second datagram on the same fixed continuity counter legitimately trips a CC error (as in other tests).
	_ = tr.TrackDatagram(base.Add(2*time.Millisecond), dg)

	full := &Stats{}
	tr.Snapshot(full, true, base, SnapshotOptions{})
	require.NotNil(t, full.Ts)
	require.NotZero(t, full.Errors.CcErrors)
	require.NotEmpty(t, full.Errors.ByPid)

	lean := &Stats{}
	tr.Snapshot(lean, true, base, SnapshotOptions{SkipTs: true})
	require.Nil(t, lean.Ts)
	require.Empty(t, lean.Errors.ByPid)
	require.Equal(t, full.Errors.Total, lean.Errors.Total, "Total is available cheaply either way")
	require.Equal(t, full.Errors.CcErrors, lean.Errors.CcErrors, "CcErrors is available cheaply either way")
}

// TestMediaTracker_Stats_CopyInto verifies CopyInto produces a deeply independent copy - mutating the source via a
// tracker's next Snapshot call (simulating reuse) must not affect a destination copied out earlier.
func TestMediaTracker_Stats_CopyInto(t *testing.T) {
	tr := NewMediaTracker("test", Config{Rtp: true})
	base := utc.Now()
	dg, _ := tsDatagramWithPCR(1_000_000)
	_ = tr.TrackDatagram(base, rtpWrap(0, 1000, dg))
	_ = tr.TrackDatagram(base.Add(2*time.Millisecond), rtpWrap(1, 1180, dg))

	src := tr.Stats()
	dst := &Stats{}
	src.CopyInto(dst)
	require.Equal(t, src.Packets, dst.Packets)
	require.Len(t, dst.Clocks, len(src.Clocks))
	require.NotSame(t, &src.Clocks[0], &dst.Clocks[0])

	// Advance the tracker and re-snapshot into src (as a caller reusing src via Snapshot would) - dst must not see it.
	_ = tr.TrackDatagram(base.Add(4*time.Millisecond), rtpWrap(2, 1360, dg))
	tr.Snapshot(src, true, base.Add(4*time.Millisecond), SnapshotOptions{})
	require.NotEqual(t, src.Packets, dst.Packets, "dst must not have changed just because src was refreshed")
}

// ---------------------------------------------------------------------------------------------------------------------
// Fixture helpers below build synthetic RTP/MPEG-TS datagrams for the tests above.

// rtpWrap prepends a minimal 12-byte RTP header (payload type 33 = MP2T) carrying the given sequence number and
// timestamp to the given MPEG-TS payload.
func rtpWrap(seq uint16, ts uint32, payload []byte) []byte {
	pkt := make([]byte, 12+len(payload))
	pkt[0] = 0x80 // version 2, no padding/extension/CSRC
	pkt[1] = 33   // payload type MP2T
	binary.BigEndian.PutUint16(pkt[2:], seq)
	binary.BigEndian.PutUint32(pkt[4:], ts)
	binary.BigEndian.PutUint32(pkt[8:], 0xdeadbeef) // arbitrary SSRC
	copy(pkt[12:], payload)
	return pkt
}

// rtpWrapVersion is rtpWrap with an explicit (possibly invalid) RTP version instead of the fixed version 2.
func rtpWrapVersion(version byte, seq uint16, ts uint32, payload []byte) []byte {
	pkt := rtpWrap(seq, ts, payload)
	pkt[0] = (pkt[0] &^ 0xC0) | (version << 6)
	return pkt
}

// rtpWrapWithExtension is rtpWrap with a minimal (4-byte) one-byte-header RTP extension, so the header consumes 16
// bytes instead of 12.
func rtpWrapWithExtension(seq uint16, ts uint32, payload []byte) []byte {
	pkt := make([]byte, 16+len(payload))
	pkt[0] = 0x90 // version 2, extension bit set
	pkt[1] = 33
	binary.BigEndian.PutUint16(pkt[2:], seq)
	binary.BigEndian.PutUint32(pkt[4:], ts)
	binary.BigEndian.PutUint32(pkt[8:], 0xdeadbeef)
	binary.BigEndian.PutUint16(pkt[12:], 0xbede) // one-byte-header extension profile
	binary.BigEndian.PutUint16(pkt[14:], 0)      // 0 extension words
	copy(pkt[16:], payload)
	return pkt
}

// tsDatagramWithPCR builds a 7-packet TS datagram whose first packet (PID 0x100) carries the given PCR value (in
// 27 MHz units). The remaining packets are null packets. Returns the datagram bytes and the PCR value that
// mpegts.ExtractPCR will report for the first packet.
func tsDatagramWithPCR(pcr uint64) (datagram []byte, reported uint64) {
	datagram = append(datagram, tsPcrPacket(0x100, pcr)...)
	for i := 0; i < 6; i++ {
		datagram = append(datagram, tsNullPacket()...)
	}
	return datagram, pcr
}

// tsDatagramTwoPrograms builds a datagram emulating a two-program transport stream: two PCR-bearing packets on PIDs
// 0x100 and 0x200 (one per program) followed by null packets.
func tsDatagramTwoPrograms(pcrA, pcrB uint64) []byte {
	var datagram []byte
	datagram = append(datagram, tsPcrPacket(0x100, pcrA)...)
	datagram = append(datagram, tsPcrPacket(0x200, pcrB)...)
	for i := 0; i < 5; i++ {
		datagram = append(datagram, tsNullPacket()...)
	}
	return datagram
}

// tsPcrPacket crafts a 188-byte TS packet on the given PID with an adaptation field carrying the PCR. The PCR value
// is in 27 MHz units and is split into the 33-bit @90 kHz base and 9-bit @27 MHz extension per ISO 13818-1.
func tsPcrPacket(pid int, pcr uint64) []byte {
	base := pcr / 300
	ext := pcr % 300

	pkt := make([]byte, packet.PacketSize)
	pkt[0] = packet.SyncByte
	pkt[1] = byte(pid>>8) & 0x1f
	pkt[2] = byte(pid)
	pkt[3] = 0x30 // adaptation field + payload, continuity counter 0
	pkt[4] = 7    // adaptation field length: 1 flags byte + 6 PCR bytes
	pkt[5] = 0x10 // PCR flag
	pkt[6] = byte(base >> 25)
	pkt[7] = byte(base >> 17)
	pkt[8] = byte(base >> 9)
	pkt[9] = byte(base >> 1)
	pkt[10] = byte(base<<7)&0x80 | 0x7e | byte((ext>>8)&0x01)
	pkt[11] = byte(ext)
	for i := 12; i < packet.PacketSize; i++ {
		pkt[i] = 0xff
	}
	return pkt
}

func tsNullPacket() []byte {
	pkt := make([]byte, packet.PacketSize)
	pkt[0] = packet.SyncByte
	pkt[1] = 0x1f // PID 0x1fff (null)
	pkt[2] = 0xff
	pkt[3] = 0x10 // payload only, continuity counter 0
	for i := 4; i < packet.PacketSize; i++ {
		pkt[i] = 0xff
	}
	return pkt
}

// tsNullPacketWithAdaptationField builds a null-PID (0x1fff) TS packet carrying both an adaptation field and a
// payload (adaptation_field_control 11) - a spec-legal but unusual shape for a null packet, used to verify
// isValidPadding locates the payload after the adaptation field rather than assuming it always starts at byte 4.
func tsNullPacketWithAdaptationField() []byte {
	pkt := make([]byte, packet.PacketSize)
	pkt[0] = packet.SyncByte
	pkt[1] = 0x1f // PID 0x1fff (null)
	pkt[2] = 0xff
	pkt[3] = 0x30 // adaptation field + payload, continuity counter 0
	pkt[4] = 1    // adaptation field length: 1 flags byte, no PCR/OPCR/splicing/private data/extension
	pkt[5] = 0x00 // flags: none set
	for i := 6; i < packet.PacketSize; i++ {
		pkt[i] = 0xff // payload: valid padding
	}
	return pkt
}
