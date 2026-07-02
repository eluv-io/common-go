package mpegts

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// makeTsPacketWithPCR returns a single valid 188-byte TS packet with PCR set in the adaptation field.
func makeTsPacketWithPCR(pid int, pcr uint64) []byte {
	pkt := packet.Create(pid)
	if err := pkt.SetAdaptationFieldControl(packet.AdaptationFieldFlag); err != nil {
		panic(err)
	}
	af, err := pkt.AdaptationField()
	if err != nil {
		panic(err)
	}
	if err = af.SetHasPCR(true); err != nil {
		panic(err)
	}
	if err = af.SetPCR(pcr); err != nil {
		panic(err)
	}
	bts := make([]byte, packet.PacketSize)
	copy(bts, (*pkt)[:])
	return bts
}

// makeTsPacketNoPCR returns a single valid 188-byte TS packet without PCR.
func makeTsPacketNoPCR(pid int) []byte {
	pkt := packet.Create(pid, packet.WithHasPayloadFlag)
	bts := make([]byte, packet.PacketSize)
	copy(bts, (*pkt)[:])
	return bts
}

// makeTsBatch creates a TS batch of n 188-byte packets. The first packet has the given PCR; the rest have no PCR.
func makeTsBatch(pid int, pcr uint64, n int) []byte {
	result := make([]byte, n*packet.PacketSize)
	copy(result, makeTsPacketWithPCR(pid, pcr))
	for i := 1; i < n; i++ {
		copy(result[i*packet.PacketSize:], makeTsPacketNoPCR(pid))
	}
	return result
}

// makeTsBatchNoPCR creates a TS batch of n 188-byte packets, none with PCR.
func makeTsBatchNoPCR(pid, n int) []byte {
	result := make([]byte, n*packet.PacketSize)
	for i := 0; i < n; i++ {
		copy(result[i*packet.PacketSize:], makeTsPacketNoPCR(pid))
	}
	return result
}

// defaultTestConfig returns a TsDisruptorPacerConfig suitable for unit tests.
// discardPeriod=0 disables the discard phase: the first batch establishes the T0 baseline and is delivered (not
// discarded), as is every subsequent batch.
func defaultTestConfig(discardPeriod time.Duration) TsDisruptorPacerConfig {
	return TsDisruptorPacerConfig{
		Stream:            "test",
		StatsLog:          elog.Noop,
		EventLog:          elog.Noop,
		StatsInterval:     -1, // disable stats logging in tests
		BufferCapacity:    64,
		MinSleepThreshold: duration.Spec(time.Millisecond),
		TickerPeriod:      duration.Spec(time.Millisecond),
		Logic: pacer.PacerLogicConfig{
			DiscardPeriod:    duration.Spec(discardPeriod),
			MaxDiscardPeriod: duration.Spec(discardPeriod * 10),
		},
	}
}

// runPacerTest starts the pacer's Run loop in a goroutine and returns a channel that receives all delivered batches.
// The caller must call Shutdown and drain the returned channel.
func runPacer(t *testing.T, p *TsDisruptorPacer) (delivered chan []byte, done chan struct{}) {
	t.Helper()
	delivered = make(chan []byte, 1024)
	done = make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Run(func(bts []byte, at utc.UTC) error {
			cp := make([]byte, len(bts))
			copy(cp, bts)
			delivered <- cp
			return nil
		})
	}()
	return
}

// waitDelivered waits until exactly n items are received on ch, or fails after timeout.
func waitDelivered(t *testing.T, ch <-chan []byte, n int, timeout time.Duration) [][]byte {
	t.Helper()
	result := make([][]byte, 0, n)
	deadline := time.After(timeout)
	for len(result) < n {
		select {
		case pkt := <-ch:
			result = append(result, pkt)
		case <-deadline:
			t.Fatalf("timed out waiting for delivery: got %d of %d", len(result), n)
		}
	}
	return result
}

// ----- Tests -----

// TestTsDisruptorPacer_BasicDelivery verifies that batches pushed after the discard phase are all delivered.
func TestTsDisruptorPacer_BasicDelivery(t *testing.T) {
	const pid = 100
	const pcrPerBatch = 270_000 // 10ms at 27MHz
	const numBatches = 10

	pacer, err := NewTsDisruptorPacer(defaultTestConfig(0))
	require.NoError(t, err)

	delivered, done := runPacer(t, pacer)

	// Push numBatches batches. First batch is always discarded (T0 initialization).
	// All subsequent batches with PCR=0 are delivered with target ≈ now.
	var pcr uint64
	for i := 0; i < numBatches; i++ {
		require.NoError(t, pacer.Push(makeTsBatch(pid, pcr, 7)))
	}

	// Expect numBatches-1 deliveries (first batch discarded).
	batches := waitDelivered(t, delivered, numBatches-1, 5*time.Second)
	require.Len(t, batches, numBatches-1)
	for _, b := range batches {
		require.Equal(t, 7*packet.PacketSize, len(b))
	}

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_PacedDelivery verifies that batches are delivered spaced by PCR-derived intervals.
func TestTsDisruptorPacer_PacedDelivery(t *testing.T) {
	const pid = 100
	const batchInterval = 20 * time.Millisecond
	const numBatches = 6
	const tolerance = 12 * time.Millisecond

	pcrPerBatch := DurationToPcr(batchInterval)

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(50 * time.Millisecond) // 50ms jitter buffer
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered := make(chan utc.UTC, numBatches)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(bts []byte, at utc.UTC) error {
			delivered <- at
			return nil
		})
	}()

	// Push batches quickly. Target times are in the future (Delay=50ms + offsets).
	var pcr uint64
	for i := 0; i < numBatches; i++ {
		require.NoError(t, pacer.Push(makeTsBatch(pid, pcr, 7)))
		pcr += pcrPerBatch
	}

	// Collect all delivery times.
	times := make([]utc.UTC, numBatches)
	deadline := time.After(5 * time.Second)
	for i := range times {
		select {
		case at := <-delivered:
			times[i] = at
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %d of %d", i+1, numBatches)
		}
	}

	// Verify inter-delivery intervals are close to batchInterval.
	for i := 1; i < len(times); i++ {
		ipd := times[i].Sub(times[i-1])
		require.InDeltaf(t, float64(batchInterval), float64(ipd), float64(tolerance),
			"IPD at index %d: got %v, want %v ± %v", i, ipd, batchInterval, tolerance)
	}

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_Delay verifies that the first delivered batch respects the configured Delay.
func TestTsDisruptorPacer_Delay(t *testing.T) {
	const pid = 100
	const delay = 60 * time.Millisecond
	const tolerance = 15 * time.Millisecond

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(delay)
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered, done := runPacer(t, pacer)

	pushTime := time.Now()
	// Push two batches: first is discarded (T0 init), second establishes baseline and is delivered.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))

	batches := waitDelivered(t, delivered, 1, 5*time.Second)
	require.Len(t, batches, 1)

	deliveryDelay := time.Since(pushTime)
	require.InDeltaf(t, float64(delay), float64(deliveryDelay), float64(tolerance),
		"delivery delay: got %v, want %v ± %v", deliveryDelay, delay, tolerance)

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_ShutdownInterruptsSleep verifies that Shutdown promptly wakes the consumer.
func TestTsDisruptorPacer_ShutdownInterruptsSleep(t *testing.T) {
	const pid = 100

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(30 * time.Second) // very long delay so consumer will be sleeping
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(bts []byte, at utc.UTC) error { return nil })
	}()

	// Push two batches: first discarded, second goes into ring buffer with target ~30s in future.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))

	// Give Run time to start and the consumer to begin sleeping.
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	pacer.Shutdown()

	select {
	case <-done:
		elapsed := time.Since(start)
		require.Less(t, elapsed, 500*time.Millisecond, "Shutdown should interrupt consumer sleep promptly")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run to return after Shutdown")
	}
}

// TestTsDisruptorPacer_DiscardPhase verifies that batches pushed during the startup discard phase are dropped.
func TestTsDisruptorPacer_DiscardPhase(t *testing.T) {
	const pid = 100
	const discardPeriod = 50 * time.Millisecond
	const pcrPerBatch = uint64(270_000) // 10ms

	conf := defaultTestConfig(discardPeriod)
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	var deliveredCount atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(bts []byte, at utc.UTC) error {
			deliveredCount.Add(1)
			return nil
		})
	}()

	// Push batches very quickly — all within a few ms, well before discardPeriod elapses.
	var pcr uint64
	for i := 0; i < 10; i++ {
		require.NoError(t, pacer.Push(makeTsBatch(pid, pcr, 7)))
		pcr += pcrPerBatch
	}

	// None should be delivered yet (still in discard phase).
	time.Sleep(5 * time.Millisecond)
	require.EqualValues(t, 0, deliveredCount.Load(), "no packets should be delivered during discard phase")

	// Wait past discardPeriod, then push more batches.
	time.Sleep(discardPeriod + 10*time.Millisecond)
	for i := 0; i < 5; i++ {
		require.NoError(t, pacer.Push(makeTsBatch(pid, pcr, 7)))
		pcr += pcrPerBatch
	}

	// Wait for these to be delivered.
	deadline := time.After(2 * time.Second)
	for {
		if deliveredCount.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("expected at least one packet to be delivered after discard phase")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_NoPCRBatch verifies that no-PCR batches are scheduled relative to the arrival time of the last
// PCR batch: a no-PCR batch pushed T ms after the preceding PCR batch should be delivered ~T ms later than it.
func TestTsDisruptorPacer_NoPCRBatch(t *testing.T) {
	const pid = 100
	const delay = 50 * time.Millisecond
	const sleepBetween = 20 * time.Millisecond
	const tolerance = 12 * time.Millisecond

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(delay)
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered := make(chan utc.UTC, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(bts []byte, at utc.UTC) error {
			delivered <- at
			return nil
		})
	}()

	// Batch 1 (delivered): PCR=0, establishes baseline.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))
	// Sleep to create a measurable arrival-time gap before the no-PCR batch.
	time.Sleep(sleepBetween)
	// Batch 2 (delivered): no PCR; should be scheduled ~sleepBetween after batch 1's target.
	require.NoError(t, pacer.Push(makeTsBatchNoPCR(pid, 7)))

	var times [2]utc.UTC
	deadline := time.After(5 * time.Second)
	for i := range times {
		select {
		case at := <-delivered:
			times[i] = at
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %d of 2", i+1)
		}
	}

	// The no-PCR batch must be scheduled ~sleepBetween after the PCR batch's target.
	interval := times[1].Sub(times[0])
	require.InDeltaf(t, float64(sleepBetween), float64(interval), float64(tolerance),
		"no-PCR batch delivery interval: got %v, want %v ± %v", interval, sleepBetween, tolerance)

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_PinsPcrToFirstPid verifies that PCR timing is pinned to the first PID a PCR is detected on:
// PCRs on any other PID (e.g. a second program in a multi-program transport stream, carrying an independent clock) are
// ignored and do not drive scheduling.
func TestTsDisruptorPacer_PinsPcrToFirstPid(t *testing.T) {
	const (
		pinnedPid = 100
		otherPid  = 200
		tick10ms  = 270_000 // PCR ticks for 10ms at 27 MHz
	)

	pacer, err := NewTsDisruptorPacer(defaultTestConfig(0))
	require.NoError(t, err)

	delivered := make(chan utc.UTC, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(_ []byte, at utc.UTC) error {
			delivered <- at
			return nil
		})
	}()

	// Batch 1: PCR on the pinned PID. Establishes the timeline and pins timing to pinnedPid.
	require.NoError(t, pacer.Push(makeTsBatch(pinnedPid, 100*tick10ms, 4)))

	// Batch 2: a mixed batch where another program's PCR (otherPid) appears BEFORE the pinned PID's PCR and carries a
	// wildly different clock value. With pinning, otherPid is ignored and the pinned PID's PCR (+10ms) drives timing.
	// Without pinning, the bogus PCR would schedule this batch ~hours into the future and it would never be delivered.
	mixed := append(append([]byte{},
		makeTsPacketWithPCR(otherPid, 900_000*tick10ms)...),
		makeTsPacketWithPCR(pinnedPid, 101*tick10ms)...)
	require.NoError(t, pacer.Push(mixed))

	var times [2]utc.UTC
	deadline := time.After(5 * time.Second)
	for i := range times {
		select {
		case at := <-delivered:
			times[i] = at
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %d of 2 (batch scheduled from the wrong PCR PID?)", i+1)
		}
	}

	// The pinned PID's PCR advanced by 10ms between the two batches, so their targets must be ~10ms apart - proving
	// the other program's PCR was ignored rather than driving the timeline.
	interval := times[1].Sub(times[0])
	require.InDeltaf(t, float64(10*time.Millisecond), float64(interval), float64(2*time.Millisecond),
		"pinned-PID cadence: got %v, want ~10ms", interval)

	// Stats() exposes the pinned PID and its last PCR (from the pinned PID, not the other program's).
	st := pacer.Stats()
	require.Equal(t, pinnedPid, st.In.Ts.PID, "Stats should expose the pinned PCR PID")
	require.EqualValues(t, 101*tick10ms, st.In.Ts.PCR, "Stats should expose the pinned PID's last PCR")

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_EstimatePcrRate verifies that when EstimatePcrRate is enabled, no-PCR batches are scheduled
// using the PCR-derived rate rather than raw arrival time, so arrival jitter does not propagate to delivery times.
// It also verifies that no-PCR batches arriving before the estimate is established are counted correctly in the
// denominator when the first estimate is computed.
func TestTsDisruptorPacer_EstimatePcrRate(t *testing.T) {
	const pid = 100
	const delay = 50 * time.Millisecond
	const tolerance = 5 * time.Millisecond
	pcrPerBatch := DurationToPcr(10 * time.Millisecond)

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(delay)
	conf.EstimatePcrRate = true
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered := make(chan utc.UTC, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(bts []byte, at utc.UTC) error {
			delivered <- at
			return nil
		})
	}()

	// Batch 1 (PCR=0): establishes baseline; estimate not yet available.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))
	// Batch 2 (no PCR): arrives before the estimate exists; noPcrBatchCount must still be incremented so the
	// first estimate uses the correct denominator (2 intervals, not 1).
	require.NoError(t, pacer.Push(makeTsBatchNoPCR(pid, 7)))
	// Batch 3 (PCR=2*pcrPerBatch): first estimate computed — pcrDelta=2*pcrPerBatch over 2 intervals → pcrPerBatch/batch.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 2*pcrPerBatch, 7)))
	// Sleep longer than pcrPerBatch to introduce jitter that should NOT affect the estimate.
	time.Sleep(25 * time.Millisecond)
	// Batch 4 (no PCR): should be scheduled one estimated interval (10ms) after batch 3, not 25ms after.
	require.NoError(t, pacer.Push(makeTsBatchNoPCR(pid, 7)))

	var times [4]utc.UTC
	deadline := time.After(5 * time.Second)
	for i := range times {
		select {
		case at := <-delivered:
			times[i] = at
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %d of 4", i+1)
		}
	}

	// batch3→batch4 must be ~pcrPerBatch (10ms), not the 25ms sleep, proving the estimate is correct.
	// If noPcrBatchCount were not incremented before the estimate was available, the estimate would be
	// 2*pcrPerBatch (20ms) instead of pcrPerBatch (10ms), and this check would fail.
	want := PcrToDuration(pcrPerBatch)
	ipd34 := times[3].Sub(times[2])
	require.InDeltaf(t, float64(want), float64(ipd34), float64(tolerance),
		"batch3→batch4 interval: got %v, want %v ± %v", ipd34, want, tolerance)

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_NonPowerOfTwoCapacity verifies that a non-power-of-two buffer capacity is rounded up.
func TestTsDisruptorPacer_NonPowerOfTwoCapacity(t *testing.T) {
	conf := defaultTestConfig(0)
	conf.BufferCapacity = 100 // not a power of 2
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)
	require.Equal(t, 128, pacer.BufferCap())
	pacer.Shutdown()
}
