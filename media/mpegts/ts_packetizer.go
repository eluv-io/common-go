package mpegts

import (
	"github.com/Comcast/gots/v2/packet"

	"github.com/eluv-io/common-go/util/byteutil"
)

// TsPacketSize is the size of an MPEG TS packet
var TsPacketSize = packet.PacketSize

// NewTsPacketizer creates a new tsPacketizer instance. It skips the incoming byte stream to the first MPEG-TS packet
// boundary, and then bundles 7 MPEG-TS packets into an output packet payload of size 7 * 188 = 1316 bytes. When wrapped
// in SRT with a header of 8 bytes, this results in a UDP packet size of 1324 bytes that fits in the regular MTU of
// 1500.
//
// Typical usage:
//
//	for {
//		n, _ := reader.Read(bts) // read bytes from a stream
//		packetizer.Write(bts[:n])
//		for packet := packetizer.Next(); len(packet) > 0; packet = packetizer.Next() {
//			// ... do something with the packet ...
//		}
//	}
//
// The exact parameter controls whether the packetizer will wait for more data to fill the target packet size. If
// exact is true, the packetizer will wait until the target packet size is reached before returning a packet. If false,
// the packetizer will return a packet as soon as it has enough data to fill a complete TS packet.
//
// The syncAlways parameter controls whether the packetizer will enforce TS packet sync at every call to Next(). If
// false, the packetizer will only enforce TS packet sync in the first call to Next().
func NewTsPacketizer(exact bool, syncMode TsSyncMode) *TsPacketizer {
	udpPacketSize := 7 * TsPacketSize
	return &TsPacketizer{
		targetPacketSize: udpPacketSize,
		exact:            exact,
		syncMode:         syncMode,
		pkt:              make([]byte, udpPacketSize),
		buf:              byteutil.NewRingBuffer(udpPacketSize),
	}
}

type TsPacketizer struct {
	targetPacketSize int                  // the size of packets to produce
	exact            bool                 // produce exactly the target packet size, even if it means waiting for more data
	syncMode         TsSyncMode           // how to sync the stream on TS packet boundaries
	buf              *byteutil.RingBuffer // internal ring buffer for packetization
	pkt              []byte               // the next packet to be returned
	remaining        []byte               // the remaining bytes from the last Write call that did not fit into the ring buffer
	synced           bool                 // initially false, set to true when TS packet boundary is found
}

// TargetPacketSize returns the target packet size in bytes: 1316.
func (p *TsPacketizer) TargetPacketSize() int {
	return p.targetPacketSize
}

// Write writes the given bytes to the internal ring buffer. It keeps a reference to unwritten bytes in bts in order to
// fill up the ring buffer as needed when Next() is called. This avoids making a copy of the bytes, but requires that
// all packets are consumed before the next Write call.
func (p *TsPacketizer) Write(bts []byte) {
	written := p.buf.Write(bts)
	p.remaining = bts[written:]
}

// Next returns the next packet.
func (p *TsPacketizer) Next() ([]byte, error) {
	switch p.syncMode {
	case TsSyncModes.Modulo():
		p.synced = true // modulo relies on the incoming bytes being already aligned to TS packet boundaries
	case TsSyncModes.Continuous():
		p.synced = false // reset to force a re-sync
	}
	skipped := 0
	for {
		p.Write(p.remaining) // no-op if remaining is empty or buf is full...

		if p.exact && p.buf.Len() < p.targetPacketSize {
			return nil, nil
		} else if p.buf.Len() < TsPacketSize {
			return nil, nil
		}

		if p.synced {
			pktLen := min(p.targetPacketSize, p.buf.Len()/TsPacketSize*TsPacketSize)
			read := p.buf.Read(p.pkt[:pktLen])
			return p.pkt[:read], nil
		}

		for {
			// find TS packet boundary
			if isSynced, err := packet.IsSynced(p.buf); err != nil {
				// need more bytes
				break
			} else if isSynced {
				// TS packet boundary found
				p.synced = true
				if skipped > 0 {
					log.Info("synced mpegts stream", "skipped_bytes", skipped)
				}
				break
			}
			_, _ = p.buf.ReadByte() // skip a byte
			skipped++
		}
	}
}
