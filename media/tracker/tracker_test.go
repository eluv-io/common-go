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
// verifies the estimated drift and skew. Inputs are exact, so the assertions are tight. Ported from
// content-fabric's qfab/cmd/media/probe/probe_test.go to document the sign convention this package inherited.
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
// correlation, mirroring content-fabric's TestProbe_UDP.
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
// are correlated, mirroring content-fabric's TestProbe_RTP.
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
// are correlated independently, mirroring content-fabric's TestProbe_MultiProgramPCR.
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
// used by a caller that already holds a decoded pktpool.Packet, e.g. avpipe) produces the same aggregate stats as
// feeding them via TrackDatagram (the raw-bytes entry point used by the probe CLI).
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
	require.ErrorIs(t, tr.TrackDatagram(utc.Now(), []byte{0x80, 0x00}), ErrDropped)
	s := tr.Stats()
	require.EqualValues(t, 1, s.Errors.SmallPacketsDropped)
	require.EqualValues(t, 0, s.Errors.RtcpPacketsDropped)

	// small and RTCP-shaped (payload type 200 = SR)
	require.ErrorIs(t, tr.TrackDatagram(utc.Now(), []byte{0x80, 200}), ErrDropped)
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

// ---------------------------------------------------------------------------------------------------------------------
// Fixtures below are ported from content-fabric's qfab/cmd/media/probe/probe_test.go.

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
