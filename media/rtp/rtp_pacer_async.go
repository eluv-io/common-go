package rtp

import (
	"time"

	"github.com/pion/rtp"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

type pacerPacket struct {
	targetTs utc.UTC // target wall clock time when to send the packet, calculated from RTP ts on reception of packet
	inTs     utc.UTC // wall clock time when the packet was sent to channel
	pkt      []byte  // the actual RTP packet
	rtpTs    uint32  // the packet's RTP timestamp
}

func (p *RtpPacer) Push(bts []byte) error {
	if p.ctx.Err() != nil {
		return errors.E("rtpPacer.Push", errors.K.Cancelled, p.ctx.Err())
	}

	pkt, err := ParsePacket(bts)
	if err != nil {
		return errors.E("rtpPacer.Push", errors.K.Invalid, err)
	}
	now := utc.Now()
	wait, discard := p.calculateWait(now, pkt.SequenceNumber, pkt.Timestamp)
	if discard {
		log.Info("rtpPacer: time reference changed - discarding packet", "stream", p.stream, "now", now, "ref", p.refTime.wallClock)
		return nil
	}
	targetTs := now.Add(wait + p.initialDelay)
	p.outStats.buffered.Add(1)

	select {
	case p.packetCh <- p.newPacerPacket(bts, now, targetTs, pkt):
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *RtpPacer) newPacerPacket(bts []byte, now, targetTs utc.UTC, pkt *rtp.Packet) *pacerPacket {
	pp := &pacerPacket{targetTs: targetTs, inTs: now, rtpTs: pkt.Timestamp}

	// PENDING(LUK): do not copy packets here - instead handle copies outside of the pacer and re-use buffers!
	pp.pkt = make([]byte, len(bts))
	copy(pp.pkt, bts)

	return pp
}

func (p *RtpPacer) Pop() (bts []byte, err error) {
	select {
	case pkt := <-p.packetCh:
		p.outStats.buffered.Add(-1)
		now := utc.Now()
		wait := pkt.targetTs.Sub(now)
		if wait > 0 {
			time.Sleep(wait)
			// below code creates a new timer instance for each packet that we need to wait for...
			//
			// select {
			// case <-time.After(wait):
			// 	// PENDING(LUK): this triggers an error even if the packetCh is not drained...
			// 	// case <-p.ctx.Done():
			// 	// 	return nil, p.ctx.Err()
			// }
			p.outStats.Sleeps++
			overslept := utc.Now().Sub(pkt.targetTs)
			if overslept > 5*time.Millisecond {
				p.outStats.OverSlept++
				if p.outStats.MaxOverslept < overslept {
					p.outStats.MaxOverslept = overslept
				}
			}
		} else if wait < time.Millisecond {
			p.outStats.DelayedPackets++
		}
		if !p.outStats.lastPacket.IsZero() {
			// we ignore the first wait value with the above if statement, but it is always "initiaDelay" anyway

			newWait := p.outStats.wait.UpdateNow(now, duration.Spec(wait))
			if newWait {
				p.outStats.WaitLast = p.outStats.wait.Previous
			}
			newIPD := p.outStats.ipd.UpdateNow(now, duration.Spec(now.Sub(p.outStats.lastPacket)))
			if newIPD {
				p.outStats.IPDLast = p.outStats.ipd.Previous
			}
			newCHD := p.outStats.chd.UpdateNow(now, duration.Spec(now.Sub(pkt.inTs)))
			if newCHD {
				p.outStats.CHDLast = p.outStats.chd.Previous
			}
			if newWait || newIPD || newCHD { // they should be in sync
				p.outStats.BufferedPackets = p.outStats.buffered.Load()
				log.Debug("rtpPacer: out statistics", "stream", p.stream, "ipd", jsonutil.Stringer(&p.outStats))
				p.outStats.DelayedPackets = 0
				p.outStats.Sleeps = 0
				p.outStats.OverSlept = 0
				p.outStats.MaxOverslept = 0
			}
		}
		p.outStats.lastPacket = now
		return pkt.pkt, nil
	case <-p.ctx.Done():
		return nil, p.ctx.Err()
	}
}

func (p *RtpPacer) Shutdown(err ...error) {
	p.cancel(ifutil.FirstOrDefault[error](err, errors.E("rtpPacer.Shutdown", errors.K.Cancelled, "reason", "pacer shutdown")))
}

func (p *RtpPacer) ShutdownAndWait(err ...error) {
	p.cancel(ifutil.FirstOrDefault[error](err, errors.E("rtpPacer.ShutdownAndWait", errors.K.Cancelled, "reason", "pacer shutdown")))
}
