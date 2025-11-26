package rtp_test

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	pionrtp "github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	rtp2 "github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/media/tlv"
	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

const ms = time.Millisecond

var (
	t1   = rtp2.TicksToDuration(1)   // 11.1µs
	t100 = rtp2.TicksToDuration(100) // 1.11ms
)

func TestTicksToDuration(t *testing.T) {
	assert.Equal(t, time.Duration(11_111), rtp2.TicksToDuration(1))
	assert.Equal(t, int64(1), rtp2.DurationToTicks(11_111))
	assert.Equal(t, time.Duration(11_111), rtp2.TicksToDuration(rtp2.DurationToTicks(11_111)))
	assert.Equal(t, int64(1), rtp2.DurationToTicks(rtp2.TicksToDuration(1)))

	assert.Equal(t, time.Second, rtp2.TicksToDuration(90000))
	assert.Equal(t, int64(90000), rtp2.DurationToTicks(time.Second))
	assert.Equal(t, time.Second, rtp2.TicksToDuration(rtp2.DurationToTicks(time.Second)))
	assert.Equal(t, int64(90000), rtp2.DurationToTicks(rtp2.TicksToDuration(int64(90000))))

	assert.Equal(t, time.Millisecond, rtp2.TicksToDuration(90))
	assert.Equal(t, int64(90), rtp2.DurationToTicks(time.Millisecond))
	assert.Equal(t, time.Millisecond, rtp2.TicksToDuration(rtp2.DurationToTicks(time.Millisecond)))
	assert.Equal(t, int64(90), rtp2.DurationToTicks(rtp2.TicksToDuration(int64(90))))
}

func TestRtpPacer_constants(t *testing.T) {
	require.Equal(t, time.Duration(11_111), t1) // 11.111 µs
	require.Equal(t, duration.MustParse("13h15m21.859s").Duration(), rtp2.WrapDuration.Round(ms))
}

func TestRtpPacer_calculateWait(t *testing.T) {
	if testing.Short() {
		// PENDING(LUK): fix tests
		t.Skip("broken - fix!")
	}

	type packets struct {
		seq      uint16
		ts       uint32
		wantWait time.Duration
	}
	tests := []struct {
		name    string
		pacer   *rtp2.RtpPacer
		packets []packets
	}{
		{
			name:  "basic",
			pacer: rtp2.NewRtpPacer(),
			packets: []packets{
				{seq: 0, ts: 0, wantWait: 0},
				{seq: 1, ts: 100, wantWait: 0},
				{seq: 2, ts: 100, wantWait: -t100}, // time has advanced by 1000 ticks, but ts is still the same
				{seq: 3, ts: 200, wantWait: -t100}, // still behind
				{seq: 4, ts: 400, wantWait: 0},     // caught up
				{seq: 5, ts: 500, wantWait: 0},
				{seq: 6, ts: 600, wantWait: 0},
				{seq: 7, ts: 800, wantWait: t100},
				{seq: 8, ts: 800, wantWait: 0},
			},
		},
		{
			name:  "adjust time ref",
			pacer: rtp2.NewRtpPacer().WithAdjustTimeRef(true),
			packets: []packets{
				{seq: 0, ts: 0, wantWait: 0},
				{seq: 1, ts: 100, wantWait: 0},
				{seq: 2, ts: 100, wantWait: -t100}, // time has advanced by 1000 ticks, but ts is still the same
				{seq: 3, ts: 200, wantWait: -t100}, // still behind
				{seq: 4, ts: 400, wantWait: 0},     // caught up
				{seq: 5, ts: 500, wantWait: 0},
				{seq: 6, ts: 600, wantWait: 0},
				{seq: 7, ts: 800, wantWait: 0},     // time ref adjusted here
				{seq: 8, ts: 800, wantWait: -t100}, // since we adjusted, we are now behind
				{seq: 9, ts: 1000, wantWait: 0},    // caught up
			},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.name), func(t *testing.T) {
			now := utc.UnixMilli(0)
			for _, packet := range test.packets {
				t.Run(fmt.Sprint(packet), func(t *testing.T) {
					assert.EqualValues(t, packet.wantWait, test.pacer.CalculateWait(now, packet.seq, packet.ts))
					now = now.Add(t100)
				})
			}
		})
	}

	t.Run("wrap-around", func(t *testing.T) {
		pacer := rtp2.NewRtpPacer()

		seq := uint16(0)
		increment := int64(90_000)
		for i := int64(0); i < 2*math.MaxUint32; i += increment {
			now := utc.Unix(0, rtp2.TicksToDuration(i).Nanoseconds())
			wait := pacer.CalculateWait(now, seq, uint32(i))
			require.EqualValues(t, 0, wait, "i=%d", i)
			seq++
		}
	})
}

func TestRtpPacer_AsyncBasic(t *testing.T) {
	pacer := rtp2.NewRtpPacer()

	collect := collectPackets(pacer)

	for i := 0; i < 20; i++ {
		err := pacer.Push(pack(t, i, i*100))
		require.NoError(t, err)
	}

	time.Sleep(100 * time.Millisecond)
	pacer.Shutdown()
	received, err := collect()
	require.NoError(t, err)
	require.Equal(t, 20, len(received))
}

func ManualTestRtpPacer_AsyncPartDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("manual test - relies on local files")
	}

	pacerSource := rtp2.NewRtpPacer().WithStream("source").WithNoLog()
	pacer := rtp2.NewRtpPacer().WithStream("test").WithDelay(50 * time.Millisecond).WithAdjustTimeRef(true)

	// Remote address (change as needed)
	remoteAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:9001")

	// Open a UDP connection
	conn, err := net.DialUDP("udp", nil, remoteAddr)
	if err != nil {
		require.NoError(t, err)
	}
	defer errors.Log(conn.Close, log.Warn)
	wait := writePackets(pacer, conn, true, 0)

	// f, err := os.OpenFile("/Users/luk/dev/tmp/troubleshooting/srt-rtp/test.out", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	// require.NoError(t, err)
	// defer errors.Log(f.Close, log.Warn)

	// wait := writePackets(pacer, f, true, 0*time.Second)

	// f2, err := os.OpenFile("/Users/luk/dev/tmp/troubleshooting/srt-rtp/test.ref", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	// require.NoError(t, err)
	// defer errors.Log(f.Close, log.Warn)

	reader := largeSource(t)
	defer errors.Log(reader.Close, log.Warn)

	// import "github.com/juju/ratelimit"
	// bucket := ratelimit.NewBucketWithRate(5_500_000, 1024*20) // 5 MB/s, burst 20 KB
	// limitedReader := ratelimit.Reader(reader, bucket)

	log.Info("start producing packets")
	// watch := timeutil.StartWatch()
	pacerStartLogged := false
	packetCount := 0
	packetizer := tlv.NewTlvPacketizer(2 * 1500)
	buf := make([]byte, 1500)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			packetizer.Write(buf[:n])
			for pkt, err := packetizer.Next(); len(pkt) > 0; pkt, err = packetizer.Next() {
				packetCount++
				if packetCount > 1000 {
					if !pacerStartLogged {
						log.Info("start pacing")
						pacerStartLogged = true
					}
					pacerSource.Wait(pkt)
				}
				err = pacer.Push(pkt)
				require.NoError(t, err)
				// bts, err := rtp.StripHeader(pkt)
				// require.NoError(t, err)
				// _, err = f2.Write(bts)
				// require.NoError(t, err)
			}
		}
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	pacer.Shutdown()
	pktCount, err := wait()
	require.NoError(t, err)
	log.Info("playout complete", "packets_sent", packetCount, "packets_received", pktCount)
}

func collectPackets(pacer *rtp2.RtpPacer) func() ([]*pionrtp.Packet, error) {
	var wg sync.WaitGroup
	var received []*pionrtp.Packet
	var failed error

	wg.Add(1)
	go func() {
		defer wg.Done()

		// throttler := timeutil.NewPeriodic(time.Second * 10)
		var lastPkt *pionrtp.Packet
		var lastPktTs utc.UTC
		var maxIpd time.Duration = 0
		for {
			p, err := pacer.Pop()
			if err != nil {
				log.Info("pacer shutdown", "err", err)
				return
			}
			now := utc.Now()
			pkt, err := rtp2.ParsePacket(p)
			if err != nil {
				failed = err
				return
			}
			received = append(received, pkt)

			if lastPkt == nil {
				log.Info("received", "seq", pkt.SequenceNumber, "ts", pkt.Timestamp)
			} else {
				maxIpd = max(maxIpd, now.Sub(lastPktTs))
				// throttler.Do(func() {
				log.Info("received", "seq", pkt.SequenceNumber,
					"ts", pkt.Timestamp,
					"ipd", duration.Spec(now.Sub(lastPktTs)).Round(),
					"max_ipd", duration.Spec(maxIpd).Round())
				// })
			}
			lastPkt = pkt
			lastPktTs = now
		}
	}()

	return func() ([]*pionrtp.Packet, error) {
		wg.Wait()
		return received, failed
	}
}

func writePackets(pacer *rtp2.RtpPacer, writer io.Writer, stripRtp bool, initialWait time.Duration) func() (int, error) {
	var wg sync.WaitGroup
	var pktCount int
	var failed error

	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			if failed != nil {
				log.Warn("playout failed", "err", failed)
			}
		}()

		time.Sleep(initialWait)
		log.Info("start reading from pacer")

		throttler := timeutil.NewPeriodic(time.Second * 10)
		var lastPkt *pionrtp.Packet
		var lastPktTs utc.UTC
		var maxIpd time.Duration = 0
		for {
			p, err := pacer.Pop()
			if err != nil {
				log.Info("pacer shutdown", "err", err)
				return
			}
			now := utc.Now()
			pkt, err := rtp2.ParsePacket(p)
			if err != nil {
				failed = err
				return
			}
			pktCount++

			if stripRtp {
				_, _ = writer.Write(pkt.Payload)
			} else {
				_, _ = writer.Write(p)
			}
			// if err != nil {
			// 	failed = err
			// 	return
			// }
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}

			if lastPkt == nil {
				log.Info("received", "seq", pkt.SequenceNumber, "ts", pkt.Timestamp)
			} else {
				maxIpd = max(maxIpd, now.Sub(lastPktTs))
				throttler.Do(func() {
					log.Info("received", "seq", pkt.SequenceNumber,
						"ts", pkt.Timestamp,
						"ipd", duration.Spec(now.Sub(lastPktTs)).Round(),
						"max_ipd", duration.Spec(maxIpd).Round())
				})
			}
			lastPkt = pkt
			lastPktTs = now
		}
	}()

	return func() (int, error) {
		wg.Wait()
		return pktCount, failed
	}
}

func pack(t *testing.T, seq, ts int) []byte {
	pkt := &pionrtp.Packet{
		Header: pionrtp.Header{
			SequenceNumber: uint16(seq),
			Timestamp:      uint32(ts),
		},
		Payload: make([]byte, 7*188),
	}
	bts, err := pkt.Marshal()
	require.NoError(t, err)
	return bts
}

func largeSource(t *testing.T) io.ReadCloser {
	files := []string{
		"/Users/luk/dev/tmp/troubleshooting/srt-rtp/hqt_7vjAXaFaYKh1jefjYA4hyzWAhvSGc7FbwAgoAvUE.rtp",
		"/Users/luk/dev/tmp/troubleshooting/srt-rtp/hqt_9Hidb6yKgjPDztVvo8ALjYvc5HhY4XGA2FQSFem6.rtp",
		"/Users/luk/dev/tmp/troubleshooting/srt-rtp/hqt_Aei6edgnYHKgHXDxqj87f4PVPVS36mmCXKm53oWM.rtp",
		"/Users/luk/dev/tmp/troubleshooting/srt-rtp/hqt_C1hZiAQQiKQdDMgcuGawvtpzahPh6zbiFJHipnrF.rtp",
		"/Users/luk/dev/tmp/troubleshooting/srt-rtp/hqt_7Zg37QmtzCLArye9eG8jgECdDBxw4gjLikzYr9kc.rtp",
	}

	var sources []io.ReadCloser
	for _, file := range files {
		source, err := os.Open(file)
		require.NoError(t, err)
		sources = append(sources, source)
	}

	return ioutil.MultiReadCloser(sources...)
}
