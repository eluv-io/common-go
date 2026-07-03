package mpegts

import (
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/util/jsonutil"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// makeAtsPacket returns an ATS-TS packet: an 8-byte big-endian arrival timestamp (nanoseconds) followed by n 188-byte
// TS packets. The TS payload is filled with sync bytes; the ATS pacer paces purely on the timestamp and does not
// inspect the TS packets.
func makeAtsPacket(arrivalNs int64, n int) []byte {
	b := make([]byte, AtsTimestampLen+n*188)
	binary.BigEndian.PutUint64(b[:AtsTimestampLen], uint64(arrivalNs))
	for i := 0; i < n; i++ {
		b[AtsTimestampLen+i*188] = 0x47 // TS sync byte
	}
	return b
}

// defaultAtsTestConfig returns an AtsDisruptorPacerConfig suitable for unit tests. DiscardPeriod=0 disables the discard
// phase: the first packet establishes the baseline and is delivered, as is every subsequent packet.
func defaultAtsTestConfig() AtsDisruptorPacerConfig {
	return AtsDisruptorPacerConfig{
		Stream:            "test",
		StatsLog:          elog.Noop,
		EventLog:          elog.Noop,
		StatsInterval:     -1, // disable stats logging in tests
		BufferCapacity:    64,
		MinSleepThreshold: duration.Spec(time.Millisecond),
		TickerPeriod:      duration.Spec(time.Millisecond),
		Logic:             pacer.PacerLogicConfig{}, // DiscardPeriod=0, Delay=0
	}
}

// runAtsPacer starts the pacer's Run loop and returns a channel receiving all delivered payloads.
func runAtsPacer(t *testing.T, p *AtsDisruptorPacer) (delivered chan []byte, done chan struct{}) {
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

// TestAtsDisruptorPacer_BasicDelivery verifies that all pushed packets are delivered and that the arrival-timestamp
// prefix is stripped from the delivered payload.
func TestAtsDisruptorPacer_BasicDelivery(t *testing.T) {
	const numPackets = 10
	const tsPerPacket = 7

	p, err := NewAtsDisruptorPacer(defaultAtsTestConfig())
	require.NoError(t, err)

	delivered, done := runAtsPacer(t, p)

	base := utc.Now().UnixNano()
	for i := 0; i < numPackets; i++ {
		require.NoError(t, p.Push(makeAtsPacket(base+int64(i)*10_000_000, tsPerPacket)))
	}

	batches := waitDelivered(t, delivered, numPackets, 5*time.Second)
	require.Len(t, batches, numPackets)
	for _, b := range batches {
		// The 8-byte arrival-timestamp prefix must be stripped; only the TS payload remains.
		require.Equal(t, tsPerPacket*188, len(b))
		require.Equal(t, byte(0x47), b[0], "delivered payload must start with a TS sync byte, not the timestamp")
	}

	p.Shutdown()
	<-done
}

// TestAtsDisruptorPacer_PacedDelivery verifies that packets are delivered spaced by their arrival-timestamp intervals.
func TestAtsDisruptorPacer_PacedDelivery(t *testing.T) {
	const interval = 20 * time.Millisecond
	const numPackets = 6
	const tolerance = 12 * time.Millisecond

	conf := defaultAtsTestConfig()
	conf.Logic.Delay = duration.Spec(50 * time.Millisecond) // jitter buffer so targets are in the future
	p, err := NewAtsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered := make(chan utc.UTC, numPackets)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Run(func(bts []byte, at utc.UTC) error {
			delivered <- at
			return nil
		})
	}()

	base := utc.Now().UnixNano()
	for i := 0; i < numPackets; i++ {
		require.NoError(t, p.Push(makeAtsPacket(base+int64(i)*interval.Nanoseconds(), 7)))
	}

	times := make([]utc.UTC, numPackets)
	deadline := time.After(5 * time.Second)
	for i := range times {
		select {
		case at := <-delivered:
			times[i] = at
		case <-deadline:
			t.Fatalf("timed out waiting for delivery %d of %d", i+1, numPackets)
		}
	}

	for i := 1; i < len(times); i++ {
		ipd := times[i].Sub(times[i-1])
		require.InDeltaf(t, float64(interval), float64(ipd), float64(tolerance),
			"IPD at index %d: got %v, want %v ± %v", i, ipd, interval, tolerance)
	}

	p.Shutdown()
	<-done
}

// TestAtsDisruptorPacer_Delay verifies that the first delivered packet respects the configured Delay (de-jitter
// buffer). Because arrival timestamps are absolute wall-clock nanoseconds, the target for the first packet is
// now + Delay.
func TestAtsDisruptorPacer_Delay(t *testing.T) {
	const delay = 60 * time.Millisecond
	const tolerance = 20 * time.Millisecond

	conf := defaultAtsTestConfig()
	conf.Logic.Delay = duration.Spec(delay)
	p, err := NewAtsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered, done := runAtsPacer(t, p)

	pushTime := time.Now()
	require.NoError(t, p.Push(makeAtsPacket(utc.Now().UnixNano(), 7)))

	batches := waitDelivered(t, delivered, 1, 5*time.Second)
	require.Len(t, batches, 1)

	deliveryDelay := time.Since(pushTime)
	require.InDeltaf(t, float64(delay), float64(deliveryDelay), float64(tolerance),
		"delivery delay: got %v, want %v ± %v", deliveryDelay, delay, tolerance)

	p.Shutdown()
	<-done
}

// TestAtsDisruptorPacer_ShortPacket verifies that a packet too short to contain the arrival timestamp is rejected.
func TestAtsDisruptorPacer_ShortPacket(t *testing.T) {
	p, err := NewAtsDisruptorPacer(defaultAtsTestConfig())
	require.NoError(t, err)

	_, done := runAtsPacer(t, p)

	err = p.Push([]byte{0x01, 0x02, 0x03}) // shorter than AtsTimestampLen
	require.Error(t, err)

	p.Shutdown()
	<-done
}

// TestAtsDisruptorPacer_GapReset verifies that an arrival-timestamp jump larger than GapThreshold triggers a stream
// reset.
func TestAtsDisruptorPacer_GapReset(t *testing.T) {
	conf := defaultAtsTestConfig()
	conf.GapThreshold = duration.Spec(500 * time.Millisecond)
	p, err := NewAtsDisruptorPacer(conf)
	require.NoError(t, err)

	delivered, done := runAtsPacer(t, p)

	base := utc.Now().UnixNano()
	require.NoError(t, p.Push(makeAtsPacket(base, 7)))
	require.NoError(t, p.Push(makeAtsPacket(base+10_000_000, 7)))                  // +10ms, no gap
	require.NoError(t, p.Push(makeAtsPacket(base+2*time.Second.Nanoseconds(), 7))) // +2s, gap > threshold

	waitDelivered(t, delivered, 3, 5*time.Second)

	stats := p.Stats()
	require.Equal(t, 1, stats.In.StreamResets, "one arrival gap should trigger exactly one stream reset")
	require.Equal(t, base+2*time.Second.Nanoseconds(), stats.In.Ats.ArrivalNs)

	p.Shutdown()
	<-done
}

// TestAtsDisruptorPacer_ShutdownInterruptsSleep verifies that Shutdown promptly wakes a sleeping consumer.
func TestAtsDisruptorPacer_ShutdownInterruptsSleep(t *testing.T) {
	conf := defaultAtsTestConfig()
	conf.Logic.Delay = duration.Spec(30 * time.Second) // long delay so the consumer sleeps
	p, err := NewAtsDisruptorPacer(conf)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Run(func(bts []byte, at utc.UTC) error { return nil })
	}()

	require.NoError(t, p.Push(makeAtsPacket(utc.Now().UnixNano(), 7)))
	time.Sleep(20 * time.Millisecond) // let the consumer begin sleeping

	start := time.Now()
	p.Shutdown()

	select {
	case <-done:
		require.Less(t, time.Since(start), 500*time.Millisecond, "Shutdown should interrupt consumer sleep promptly")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run to return after Shutdown")
	}
}

// TestAtsDisruptorPacer_Config verifies InitDefaults and JSON round-tripping of the config.
func TestAtsDisruptorPacer_Config(t *testing.T) {
	t.Run("InitDefaults-empty-json", func(t *testing.T) {
		cfg := AtsDisruptorPacerConfig{}
		cfg.InitDefaults()
		require.NoError(t, json.Unmarshal([]byte(`{}`), &cfg))
		require.Equal(t, *new(AtsDisruptorPacerConfig).InitDefaults(), cfg)
	})

	t.Run("round-trip", func(t *testing.T) {
		cfg := AtsDisruptorPacerConfig{
			Logic:             *new(pacer.PacerLogicConfig).InitDefaults(),
			GapThreshold:      duration.Minute,
			BufferCapacity:    1024,
			MinSleepThreshold: duration.Millisecond,
			TickerPeriod:      duration.Millisecond,
			StatsInterval:     duration.Minute,
			SendAhead:         50 * duration.Millisecond,
			DeliveryMargin:    25 * duration.Millisecond,
		}
		marshaled, err := json.Marshal(cfg)
		require.NoError(t, err)
		t.Log("marshaled config", "json", "\n"+jsonutil.MustPretty(string(marshaled)))

		unmarshaled := AtsDisruptorPacerConfig{}
		require.NoError(t, json.Unmarshal(marshaled, &unmarshaled))
		require.Equal(t, cfg, unmarshaled)
	})
}

// TestAtsDisruptorPacer_BufferCap verifies that the configured capacity is rounded up to the next power of 2.
func TestAtsDisruptorPacer_BufferCap(t *testing.T) {
	conf := defaultAtsTestConfig()
	conf.BufferCapacity = 100 // not a power of 2
	p, err := NewAtsDisruptorPacer(conf)
	require.NoError(t, err)
	require.Equal(t, 128, p.BufferCap())
	p.Shutdown()
}
