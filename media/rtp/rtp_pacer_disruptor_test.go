package rtp_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/log-go"
)

func newTestDisruptorPacer(
	t testing.TB,
	discardPeriod, delay time.Duration,
	adapt ...func(rtp.DisruptorPacerConfig) rtp.DisruptorPacerConfig,
) *rtp.DisruptorPacer {
	t.Helper()
	cfg := rtp.DisruptorPacerConfig{
		Stream:   "test",
		StatsLog: log.Get("/test/rtp/disruptor"),
		EventLog: log.Get("/test/rtp/disruptor"),
		Logic: rtp.PacerLogicConfig{
			DiscardPeriod:    duration.Spec(discardPeriod),
			MaxDiscardPeriod: duration.Spec(max(discardPeriod*10, time.Second)),
			Delay:            duration.Spec(delay),
			RtpSeqThreshold:  1,
			RtpTsThreshold:   duration.Spec(time.Second),
		},
	}
	if len(adapt) > 0 {
		cfg = adapt[0](cfg)
	}
	pacer, err := rtp.NewDisruptorPacer(cfg)
	require.NoError(t, err)
	return pacer
}

// startDisruptorConsumer starts the consumer goroutine. The returned function blocks until the consumer exits and
// returns all received raw packet bytes.
func startDisruptorConsumer(pacer *rtp.DisruptorPacer) func() [][]byte {
	ch := make(chan []byte, 10_000)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = pacer.Run(func(pkt []byte, _ time.Time) error {
			cp := make([]byte, len(pkt))
			copy(cp, pkt)
			ch <- cp
			return nil
		})
		close(ch)
	}()
	return func() [][]byte {
		wg.Wait()
		var packets [][]byte
		for pkt := range ch {
			packets = append(packets, pkt)
		}
		return packets
	}
}

// TestDisruptorPacer_NonPowerOfTwoCapacity verifies that a non-power-of-2 buffer capacity is automatically rounded up
// to the next power of 2 rather than being rejected.
func TestDisruptorPacer_NonPowerOfTwoCapacity(t *testing.T) {
	pacer, err := rtp.NewDisruptorPacer(rtp.DisruptorPacerConfig{
		Logic: rtp.PacerLogicConfig{
			EventLog:        log.Get("/test"),
			RtpSeqThreshold: 1,
			RtpTsThreshold:  duration.Spec(time.Second),
		},
		BufferCapacity: 1000, // rounds up to 1024
	})
	require.NoError(t, err)
	require.Equal(t, 1024, pacer.BufferCap(), "1000 should round up to 1024")
	pacer.Shutdown()
}

// TestDisruptorPacer_BasicDelivery verifies that all pushed packets are delivered exactly once in sequence order.
// All packets use ts=0 so their T0 is stable (T0 = now, which only increases), ensuring no packets are discarded.
func TestDisruptorPacer_BasicDelivery(t *testing.T) {
	pacer := newTestDisruptorPacer(t, 0, 0)
	collect := startDisruptorConsumer(pacer)

	const n = 20
	for i := range n {
		require.NoError(t, pacer.Push(pack(t, i, 0))) // ts=0 keeps T0 stable
	}

	time.Sleep(50 * time.Millisecond)
	pacer.Shutdown()
	received := collect()

	require.Equal(t, n, len(received), "all packets must be delivered")
	for i, raw := range received {
		pkt, err := rtp.ParsePacket(raw)
		require.NoError(t, err)
		require.Equal(t, uint16(i), pkt.SequenceNumber, "out-of-order delivery at index %d", i)
	}
}

// TestDisruptorPacer_PacedDelivery verifies that all pushed packets are delivered according to their RTP timestamp.
// Packets are pushed one after another, but with timestamps increasing by 10ms each. The test ensures that they are
// delivered in the correct order and with the expected delay between each packet.
func TestDisruptorPacer_PacedDelivery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	const n = 500
	const ipd = 10 * time.Millisecond
	const delay = 200 * time.Millisecond

	pacer := newTestDisruptorPacer(t, 0, delay, func(config rtp.DisruptorPacerConfig) rtp.DisruptorPacerConfig {
		config.MinSleepThreshold = duration.Millisecond
		return config
	})

	type delivery struct {
		seq uint16
		at  time.Time
	}
	ch := make(chan delivery, n)
	var wg sync.WaitGroup
	wg.Add(1)

	remaining := n
	go func() {
		defer wg.Done()
		_ = pacer.Run(func(raw []byte, _ time.Time) error {
			pkt, err := rtp.ParsePacket(raw)
			if err != nil {
				return err
			}
			ch <- delivery{seq: pkt.SequenceNumber, at: time.Now()}
			remaining--
			if remaining == 0 {
				pacer.Shutdown()
			}
			return nil
		})
		close(ch)
	}()

	watch := timeutil.StartWatch()

	// Push all packets immediately; timestamps spaced 10ms apart so the pacer delivers them ~10ms apart.
	for i := range n {
		ts := int(rtp.DurationToTicks(time.Duration(i) * ipd))
		require.NoError(t, pacer.Push(pack(t, i, ts)))
	}

	wg.Wait()
	watch.Stop()

	var received []delivery
	for d := range ch {
		received = append(received, d)
	}

	require.Equal(t, n, len(received), "all packets must be delivered")
	require.LessOrEqual(t, ipd*(n-1)+delay, watch.Duration()+5*time.Millisecond, "packets must be paced according to their timestamps")
	require.GreaterOrEqual(t, ipd*(n-1)+delay, watch.Duration()-5*time.Millisecond, "packets must be paced according to their timestamps")

	offCount := 0
	for i, d := range received {
		require.Equal(t, uint16(i), d.seq, "wrong sequence at index %d", i)
		if i > 0 {
			gap := d.at.Sub(received[i-1].at)
			if gap < ipd/2 || gap > ipd*3/2 {
				t.Logf("gap between packets %d and %d: %v (expected ~%v)", i-1, i, gap, ipd)
				offCount++
			}
		}
	}

	require.LessOrEqual(t, offCount, 2, "too many irregular ipd gaps: %d", offCount)

}

// TestDisruptorPacer_Delay verifies that the first packet is delivered approximately Delay after it is pushed.
func TestDisruptorPacer_Delay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}
	const delay = 100 * time.Millisecond
	pacer := newTestDisruptorPacer(t, 0, delay)

	delivered := make(chan time.Time, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = pacer.Run(func(_ []byte, _ time.Time) error {
			select {
			case delivered <- time.Now():
			default:
			}
			return nil
		})
	}()

	pushTime := time.Now()
	require.NoError(t, pacer.Push(pack(t, 0, 0)))

	select {
	case deliveredAt := <-delivered:
		elapsed := deliveredAt.Sub(pushTime)
		require.GreaterOrEqual(t, elapsed, delay-20*time.Millisecond, "packet delivered too early")
		require.Less(t, elapsed, delay+200*time.Millisecond, "packet delivered too late")
	case <-time.After(delay + 500*time.Millisecond):
		t.Fatal("packet not delivered within deadline")
	}

	pacer.Shutdown()
	wg.Wait()
}

// TestDisruptorPacer_ShutdownInterruptsSleep verifies that Shutdown returns promptly even when the consumer is
// sleeping while waiting for a far-future target time.
func TestDisruptorPacer_ShutdownInterruptsSleep(t *testing.T) {
	const delay = 30 * time.Second
	pacer := newTestDisruptorPacer(t, 0, delay)
	collect := startDisruptorConsumer(pacer)

	require.NoError(t, pacer.Push(pack(t, 0, 0)))

	start := time.Now()
	pacer.Shutdown()
	collect() // wait for consumer to exit

	require.Less(t, time.Since(start), time.Second, "Shutdown must interrupt the consumer sleep promptly")
}

// TestDisruptorPacer_DiscardPhase verifies that packets pushed during the discard window are dropped, and only the
// first packet that arrives after the window has elapsed is delivered.
//
// Using ts=0 for all packets keeps T0 = now (monotonically increasing) so no T0 backward adjustments interfere with
// the timed discard logic.
func TestDisruptorPacer_DiscardPhase(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}
	const discardPeriod = 20 * time.Millisecond
	pacer := newTestDisruptorPacer(t, discardPeriod, 0)
	collect := startDisruptorConsumer(pacer)

	// Packet 0 starts the discard timer and is itself discarded (first packet is always discarded when
	// DiscardPeriod > 0).
	require.NoError(t, pacer.Push(pack(t, 0, 0)))

	// Wait well past the discard period.
	time.Sleep(discardPeriod * 3)

	// This packet arrives after the discard window; should be delivered.
	require.NoError(t, pacer.Push(pack(t, 1, 0)))

	time.Sleep(50 * time.Millisecond)
	pacer.Shutdown()
	received := collect()

	require.Equal(t, 1, len(received), "only the post-discard packet should be delivered")
	pkt, err := rtp.ParsePacket(received[0])
	require.NoError(t, err)
	require.Equal(t, uint16(1), pkt.SequenceNumber)
}

// TestDisruptorPacer_PushAfterShutdown verifies that Push returns an error once the pacer has been shut down.
func TestDisruptorPacer_PushAfterShutdown(t *testing.T) {
	pacer := newTestDisruptorPacer(t, 0, 0)
	collect := startDisruptorConsumer(pacer)
	pacer.Shutdown()
	collect()

	err := pacer.Push(pack(t, 0, 0))
	require.Error(t, err, "Push after Shutdown must return an error")
}

// TestDisruptorPacer_ShutdownIdempotent verifies that calling Shutdown multiple times does not panic.
func TestDisruptorPacer_ShutdownIdempotent(t *testing.T) {
	pacer := newTestDisruptorPacer(t, 0, 0)
	collect := startDisruptorConsumer(pacer)
	pacer.Shutdown()
	pacer.Shutdown()
	pacer.Shutdown()
	collect()
}

// TestDisruptorPacer_Delay verifies that the first packet is delivered approximately Delay after it is pushed.
func TestDisruptorPacer_DelayContinuous(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}
	const (
		delay      = time.Second
		queueAhead = 120 * duration.Millisecond
		ipd        = 10 * time.Millisecond
		packets    = 300
	)

	pacer := newTestDisruptorPacer(t, 0, delay, func(config rtp.DisruptorPacerConfig) rtp.DisruptorPacerConfig {
		config.StatsInterval = duration.Second
		config.QueueAhead = queueAhead
		return config
	})

	delivered := 0

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = pacer.Run(func(_ []byte, _ time.Time) error {
			delivered++
			return nil
		})
	}()

	ticker := time.NewTicker(ipd)
	for i := 1; i <= packets; i++ {
		require.NoError(t, pacer.Push(pack(t, i, i*(int)(rtp.DurationToTicks(ipd)))))
		<-ticker.C
	}

	time.Sleep(delay + 10*time.Millisecond)

	pacer.Shutdown()
	wg.Wait()

	require.Equal(t, packets, delivered)

	in, out := pacer.Stats()

	log.Info("total stats", "in", jsonutil.MarshalString(in), "out", jsonutil.MarshalString(out))

	require.EqualValues(t, packets, in.PushAhead.Count)
	require.InDelta(t, time.Second, in.PushAhead.Mean, float64(time.Millisecond))
	require.EqualValues(t, packets, in.Sequ)
	require.EqualValues(t, packets*(int)(rtp.DurationToTicks(ipd)), in.Tsu)

	require.EqualValues(t, packets, out.CHD.Count)
	require.InDelta(t, 880*time.Millisecond, out.CHD.Mean, float64(time.Millisecond))
	require.EqualValues(t, packets, out.IPD.Count)
	require.InDelta(t, ipd, out.IPD.Mean, float64(time.Millisecond))
	require.EqualValues(t, 0, out.Lateness.Count)
	require.EqualValues(t, 0, out.OverSleeps.Count)
	require.EqualValues(t, packets, out.SendAhead.Count)
	require.InDelta(t, float64(queueAhead), out.SendAhead.Mean, float64(time.Millisecond))
}
