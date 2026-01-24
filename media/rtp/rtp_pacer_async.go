package rtp

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pktpool"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// MinSleepThreshold is the threshold for sleeps - below this the pacer does not actually sleep to avoid timer overhead
const MinSleepThreshold = 5 * time.Millisecond

type pacerPacket struct {
	targetTs  utc.UTC         // target wall clock time when to send the packet, calculated from RTP ts on reception of packet
	inTs      utc.UTC         // wall clock time when the packet was sent to channel
	pkt       []byte          // the actual RTP packet data (points to pooledPkt.Data)
	pooledPkt *pktpool.Packet // pooled packet that holds the data
	rtpTs     uint32          // the packet's RTP timestamp
}

func (p *RtpPacer) Push(bts []byte) error {
	if p.ctx.Err() != nil {
		return errors.E("rtpPacer.Push", errors.K.Cancelled, p.ctx.Err())
	}

	// Parse minimal header - NO ALLOCATION (stack-allocated struct)
	hdr, err := parseRtpHeaderMinimal(bts)
	if err != nil {
		return errors.E("rtpPacer.Push", errors.K.Invalid, err)
	}

	now := utc.Now()
	wait, discard := p.calculateWait(now, hdr.sequenceNumber, hdr.timestamp)
	if discard {
		log.Info("rtpPacer: time reference changed - discarding packet",
			"stream", p.stream, "now", now, "ref", p.refTime.wallClock)
		return nil
	}
	targetTs := now.Add(wait + p.initialDelay)
	p.outStats.buffered.Add(1)

	// Get packet buffer from pool
	pooledPkt := p.packetPool.GetPacket()
	pooledPkt.Data = pooledPkt.Data[:len(bts)]
	copy(pooledPkt.Data, bts)

	// Get pacerPacket from pool - NO ALLOCATION
	pp := p.pacerPacketPool.Get().(*pacerPacket)
	pp.targetTs = targetTs
	pp.inTs = now
	pp.rtpTs = hdr.timestamp
	pp.pkt = pooledPkt.Data
	pp.pooledPkt = pooledPkt

	select {
	case p.packetCh <- pp:
		return nil
	case <-p.ctx.Done():
		// Release both pools on error
		pooledPkt.Release()
		p.releasePacerPacket(pp)
		return p.ctx.Err()
	}
}

// releasePacerPacket resets and returns a pacerPacket to the pool
func (p *RtpPacer) releasePacerPacket(pp *pacerPacket) {
	if pp == nil {
		return
	}
	// Reset fields to zero values to avoid retaining references
	pp.targetTs = utc.UTC{}
	pp.inTs = utc.UTC{}
	pp.pkt = nil
	pp.pooledPkt = nil
	pp.rtpTs = 0

	p.pacerPacketPool.Put(pp)
}

func (p *RtpPacer) Pop() (bts []byte, err error) {
	// Release the previous packet buffers
	if p.lastPoppedPacket != nil {
		if p.lastPoppedPacket.pooledPkt != nil {
			p.lastPoppedPacket.pooledPkt.Release()
		}
		// Return pacerPacket struct to pool
		p.releasePacerPacket(p.lastPoppedPacket)
		p.lastPoppedPacket = nil
	}

	select {
	case pkt := <-p.packetCh:
		p.outStats.buffered.Add(-1)
		now := utc.Now()
		wait := pkt.targetTs.Sub(now)
		if wait > MinSleepThreshold {
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
			overslept := duration.Spec(utc.Now().Sub(pkt.targetTs))
			if overslept > 5*duration.Millisecond {
				p.outStats.Overslept++
				if p.outStats.MaxOverslept < overslept {
					p.outStats.MaxOverslept = overslept
				}
			}
		} else if wait < -time.Millisecond {
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
				p.outStats.Overslept = 0
				p.outStats.MaxOverslept = 0
			}
		}
		p.outStats.lastPacket = now

		// Store reference to this packet so we can release it on the next Pop() call
		p.lastPoppedPacket = pkt

		return pkt.pkt, nil
	case <-p.ctx.Done():
		return nil, p.ctx.Err()
	}
}

func (p *RtpPacer) Shutdown(err ...error) {
	p.cancel(ifutil.FirstOrDefault[error](err,
		errors.E("rtpPacer.Shutdown", errors.K.Cancelled, "reason", "pacer shutdown")))

	// Release the last popped packet
	if p.lastPoppedPacket != nil {
		if p.lastPoppedPacket.pooledPkt != nil {
			p.lastPoppedPacket.pooledPkt.Release()
		}
		p.releasePacerPacket(p.lastPoppedPacket)
		p.lastPoppedPacket = nil
	}

	// Drain and release any remaining packets in the channel
	for {
		select {
		case pkt := <-p.packetCh:
			if pkt.pooledPkt != nil {
				pkt.pooledPkt.Release()
			}
			p.releasePacerPacket(pkt)
		default:
			return
		}
	}
}

func (p *RtpPacer) ShutdownAndWait(err ...error) {
	p.cancel(ifutil.FirstOrDefault[error](err,
		errors.E("rtpPacer.ShutdownAndWait", errors.K.Cancelled, "reason", "pacer shutdown")))

	// Release the last popped packet
	if p.lastPoppedPacket != nil {
		if p.lastPoppedPacket.pooledPkt != nil {
			p.lastPoppedPacket.pooledPkt.Release()
		}
		p.releasePacerPacket(p.lastPoppedPacket)
		p.lastPoppedPacket = nil
	}

	// Drain and release any remaining packets in the channel
	for {
		select {
		case pkt := <-p.packetCh:
			if pkt.pooledPkt != nil {
				pkt.pooledPkt.Release()
			}
			p.releasePacerPacket(pkt)
		default:
			return
		}
	}
}
