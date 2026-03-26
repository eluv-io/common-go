package mpegts

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/rtp"
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
// discardPeriod=0 means: first batch is discarded (T0 init), all subsequent are delivered immediately.
func defaultTestConfig(discardPeriod time.Duration) TsDisruptorPacerConfig {
	return TsDisruptorPacerConfig{
		Stream:            "test",
		StatsLog:          elog.Noop,
		EventLog:          elog.Noop,
		StatsInterval:     -1, // disable stats logging in tests
		BufferCapacity:    64,
		MinSleepThreshold: duration.Spec(time.Millisecond),
		TickerPeriod:      duration.Spec(time.Millisecond),
		Logic: rtp.PacerLogicConfig{
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

	var deliveryTimes []utc.UTC
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = pacer.Run(func(bts []byte, at utc.UTC) error {
			mu.Lock()
			deliveryTimes = append(deliveryTimes, at)
			mu.Unlock()
			return nil
		})
	}()

	// Push batches quickly. Target times are in the future (Delay=50ms + offsets).
	var pcr uint64
	for i := 0; i < numBatches; i++ {
		require.NoError(t, pacer.Push(makeTsBatch(pid, pcr, 7)))
		pcr += uint64(pcrPerBatch)
	}

	// Wait for numBatches-1 deliveries (first discarded).
	deadline := time.After(5 * time.Second)
	for {
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		n := len(deliveryTimes)
		mu.Unlock()
		if n >= numBatches-1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: got %d deliveries", n)
		default:
		}
	}

	mu.Lock()
	times := append([]utc.UTC(nil), deliveryTimes...)
	mu.Unlock()

	require.GreaterOrEqual(t, len(times), numBatches-1)

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

// TestTsDisruptorPacer_NoPCRBatch verifies that batches without PCR reuse the last known target time.
func TestTsDisruptorPacer_NoPCRBatch(t *testing.T) {
	const pid = 100

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(30 * time.Millisecond)
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered, done := runPacer(t, pacer)

	// Batch 1 (discarded): establishes T0.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))
	// Batch 2 (delivered): PCR=0, establishes baseline, lastTarget = now+30ms.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 0, 7)))
	// Batch 3 (delivered): no PCR, reuses lastTarget.
	require.NoError(t, pacer.Push(makeTsBatchNoPCR(pid, 7)))
	// Batch 4 (delivered): PCR=pcrPerBatch.
	require.NoError(t, pacer.Push(makeTsBatch(pid, 270_000, 7)))

	// All three post-discard batches should arrive.
	batches := waitDelivered(t, delivered, 3, 5*time.Second)
	require.Len(t, batches, 3)

	pacer.Shutdown()
	<-done
}

// TestTsDisruptorPacer_MultiPID verifies that two PCR PIDs are tracked independently.
func TestTsDisruptorPacer_MultiPID(t *testing.T) {
	const pid1 = 100
	const pid2 = 200
	const pcrPerBatch = uint64(270_000) // 10ms
	const numBatchesPerPID = 4

	conf := defaultTestConfig(0)
	conf.Logic.Delay = duration.Spec(30 * time.Millisecond)
	pacer, err := NewTsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered, done := runPacer(t, pacer)

	// Push batches alternating between the two PIDs.
	// First batch per PID is discarded (T0 init per PID), rest are delivered.
	var pcr1, pcr2 uint64
	for i := 0; i < numBatchesPerPID; i++ {
		require.NoError(t, pacer.Push(makeTsBatch(pid1, pcr1, 7)))
		pcr1 += pcrPerBatch
		require.NoError(t, pacer.Push(makeTsBatch(pid2, pcr2, 7)))
		pcr2 += pcrPerBatch
	}

	// numBatchesPerPID batches per PID pushed; first per PID is discarded.
	// Total delivered = (numBatchesPerPID-1) * 2.
	expectedDelivered := (numBatchesPerPID - 1) * 2
	batches := waitDelivered(t, delivered, expectedDelivered, 5*time.Second)
	require.Len(t, batches, expectedDelivered)

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

// TestTsDisruptorPacer_PcrUnwrapper verifies PCR wraparound is handled correctly.
func TestTsDisruptorPacer_PcrUnwrapper(t *testing.T) {
	var u tsPcrUnwrapper

	// First call: baseline established near MaxPCR so that wraparound can be exercised.
	nearMax := uint64(MaxPCR - 100)
	prev, curr := u.unwrap(nearMax)
	require.Equal(t, int64(nearMax)-1, prev) // fabricated previous
	require.Equal(t, int64(nearMax), curr)

	// Normal forward step (small increment within same range).
	prev, curr = u.unwrap(nearMax + 50)
	require.Equal(t, int64(nearMax), prev)
	require.Equal(t, int64(nearMax)+50, curr)

	// Wraparound: PCR wraps from near MaxPCR to near 0.
	prev, curr = u.unwrap(50)
	require.Equal(t, int64(nearMax)+50, prev)
	// diff = 50 - (nearMax+50) = -nearMax = -(MaxPCR-100) which is < -halfRange → forward wraparound
	// adjusted diff = 50 - (nearMax+50) + (MaxPCR+1) = MaxPCR+1 - MaxPCR + 100 - 50 + 50 = 151
	// Wait: diff = int64(50) - int64(nearMax+50) = -int64(nearMax) = -(MaxPCR-100)
	// diff += MaxPCR+1 = -(MaxPCR-100) + MaxPCR + 1 = 101
	require.Equal(t, int64(nearMax)+50+101, curr)

	// Next normal forward step (no wraparound).
	prev, curr = u.unwrap(151)
	require.Equal(t, int64(nearMax)+50+101, prev)
	require.Equal(t, int64(nearMax)+50+101+101, curr) // diff = 151 - 50 = 101
}

// TestTsDisruptorPacer_GapDetector verifies gap detection triggers correctly.
func TestTsDisruptorPacer_GapDetector(t *testing.T) {
	threshold := DurationToPcr(time.Second) // 1 second
	gd := tsPcrGapDetector{threshold: threshold}

	// First call: never a gap.
	prev, curr, gap := gd.detect(1000)
	require.False(t, gap)
	_ = prev
	_ = curr

	// Normal step: no gap.
	_, _, gap = gd.detect(2000)
	require.False(t, gap)

	// Large jump: gap detected.
	bigJump := uint64(2000) + uint64(threshold) + 1
	_, _, gap = gd.detect(bigJump)
	require.True(t, gap)

	// After gap, normal step: no gap.
	_, _, gap = gd.detect(bigJump + 1000)
	require.False(t, gap)
}
