package mpegts

import (
	"time"

	"github.com/Comcast/gots/v2/packet"

	"github.com/eluv-io/common-go/media/transport/rtp"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
)

// NewTsPacer creates a pacer that can be used to playout MPEG TS packets at the correct rate.
func NewTsPacer() *TsPacer {
	return &TsPacer{
		stripRtp:     false,
		usePCR:       true,
		pid2start:    make(map[int]*streamStart),
		logThrottler: timeutil.NewPeriodic(10 * time.Second),
		stream:       "n/a",
	}
}

// TsPacer is a simple component that can be used to playout MPEG TS packets at the correct rate. It offers a single
// Wait() method that should be called before each packet is sent out. The Wait() method will block until the correct
// time to send the packet has elapsed.
//
// By default, the pacer uses PCR (Program Clock Reference) to calculate the wait time. It marks the current wall clock
// time when Wait() is called the first time. On subsequent calls, it calculates the wait time based on the difference
// between the PCR of the packet and the PCR of the first packet in the stream.
//
// Instead of the PCR the tracer can also use DTS (Decoding Time Stamp) and/or PTS (Presentation Time Stamp). Enable
// this by calling the WithDtsPts() method.
type TsPacer struct {
	initialDelay time.Duration        // initial delay before sending the first packet
	stripRtp     bool                 // whether to strip the RTP header before sending the packet
	usePCR       bool                 // whether to use PCR for pacing or DTS/PTS
	pid2start    map[int]*streamStart // map of stream start times, keyed by PIDs
	logThrottler timeutil.Periodic    // prevent spamming the log with errors
	stream       string               // stream name
}

func (p *TsPacer) SetDelay(delay time.Duration) {
	p.initialDelay = delay
}

func (p *TsPacer) WithDelay(delay time.Duration) *TsPacer {
	p.initialDelay = delay
	return p
}

func (p *TsPacer) WithStripRtp(stripRtp bool) *TsPacer {
	p.stripRtp = stripRtp
	return p
}

func (p *TsPacer) WithDtsPts() *TsPacer {
	p.usePCR = false
	return p
}

func (p *TsPacer) WithStream(stream string) *TsPacer {
	p.stream = stream
	return p
}

func (p *TsPacer) Wait(bts []byte) {
	done := false
	if p.stripRtp {
		var err error
		bts, err = rtp.StripHeader(bts)
		if err != nil {
			p.logThrottler.Do(func() {
				log.Info("tsPacer: failed to strip RTP header", errors.ClearStacktrace(err), "stream", p.stream)
			})
			return
		}
	}
	if p.initialDelay > 0 {
		time.Sleep(p.initialDelay)
		p.initialDelay = 0
	}
	for len(bts) > packet.PacketSize && !done {
		pkt := packet.Packet(bts)
		err := pkt.CheckErrors()
		if err != nil {
			p.logThrottler.Do(func() {
				log.Info("tsPacer: TS packet error", errors.ClearStacktrace(err), "stream", p.stream)
			})
			return
		}
		if p.usePCR {
			done = p.waitPcr(&pkt)
		} else {
			done = p.waitPts(&pkt)
		}

		bts = bts[packet.PacketSize:]
	}
}

func (p *TsPacer) waitPcr(pkt *packet.Packet) bool {
	pcr, hasPcr := ExtractPCR(pkt)
	if !hasPcr {
		return false
	}

	pid := pkt.PID()
	if start, ok := p.pid2start[pid]; !ok {
		log.Info("start pcr", "pid", pid, "pcr", pcr, "stream", p.stream)
		p.pid2start[pid] = &streamStart{
			ts:    pcr,
			watch: timeutil.StartWatch(),
		}
	} else {
		if pcr < start.ts {
			// PCR wrapped around. Reset the start reference and don't worry about calculating the wait time.
			start.ts = pcr
			start.watch.Reset()
			return false
		}

		tsDiff := pcr - start.ts

		// PCR is in 27mHz units, i.e. 1 tick = 1/27000000 s
		dur := time.Duration(tsDiff) * time.Microsecond / 27
		wdur := start.watch.Duration()
		wait := dur - wdur
		// log.Info("wait", "pid", pid, "start_ts", start.ts, "pcr", pcr, "diff", tsDiff, "dur", dur, "watch_dur", wdur, "wait", wait)
		if wait > 0 {
			if log.IsTrace() {
				log.Trace("tsPacer: wait",
					"stream", p.stream,
					"pid", pid,
					"start_ts", start.ts,
					"pcr", pcr,
					"diff", tsDiff,
					"dur", dur,
					"watch_dur", wdur,
					"wait", wait)
			}
			time.Sleep(wait)
		} else if wait < -20*time.Millisecond {
			if log.IsInfo() {
				log.Info("tsPacer: no wait - packet delayed",
					"stream", p.stream,
					"pid", pid,
					"start_ts", start.ts,
					"pcr", pcr,
					"diff", tsDiff,
					"dur", dur,
					"watch_dur", wdur,
					"wait", wait)
			}
		}
	}

	return true
}

func (p *TsPacer) waitPts(pkt *packet.Packet) bool {
	pts, dts, hasPts := ExtractPTS(pkt)
	if !hasPts {
		return false
	}

	pid := pkt.PID()
	if start, ok := p.pid2start[pid]; !ok {
		log.Info("tsPacer: start pts", "stream", p.stream, "pid", pid, "pts", pts, "dts", dts)
		p.pid2start[pid] = &streamStart{
			ts:    dts,
			watch: timeutil.StartWatch(),
		}
	} else {
		if dts < start.ts {
			// TS wrapped around. Reset the start reference and don't worry about calculating the wait time.
			start.ts = dts
			start.watch.Reset()
			return false
		}

		// PTS/DTS is in 90kHz units, i.e. 1 tick = 1/90000 s
		tsDiff := dts - start.ts
		dur := time.Duration(tsDiff) * 100 * time.Microsecond / 9
		wdur := start.watch.Duration()
		wait := dur - wdur
		if wait > 0 {
			if log.IsTrace() {
				log.Trace("tsPacer: wait",
					"stream", p.stream,
					"pid", pid,
					"start_ts", start.ts,
					"pts", pts,
					"dts", dts,
					"diff", tsDiff,
					"dur", dur,
					"watch_dur", wdur,
					"wait", wait)
			}
			time.Sleep(wait)
		} else if wait < -20*time.Millisecond {
			if log.IsInfo() {
				log.Info("tsPacer: no wait - packet delayed",
					"stream", p.stream,
					"pid", pid,
					"start_ts", start.ts,
					"pts", pts,
					"dts", dts,
					"diff", tsDiff,
					"dur", dur,
					"watch_dur", wdur,
					"wait", wait)
			}
		}
	}

	return true
}

type streamStart struct {
	ts    uint64
	watch *timeutil.StopWatch
}
