package media

import (
	"time"

	"github.com/eluv-io/utc-go"
)

// Transformer is an interface for transforming packets before sending them.
type Transformer interface {
	// Transform transforms the given packet and returns the transformed packet or nil if the packet should be dropped.
	Transform(bts []byte) ([]byte, error)
}

// Packetizer is an interface reading packets from a byte stream and aggregating them into network packet payloads.
//
// Use the packetizer as follows:
//
//	buf := make([]byte, 4096)
//	r := NewReader(...)
//	packetizer := NewPacketizer(...)
//	for {
//	  n, err := r.Read(buf)
//	  packetizer.Write(buf[:n])
//	  for packet := packetizer.Next(); packet != nil; packet = packetizer.Next() {
//	    sendPacket(packet)
//	  }
//	}
type Packetizer interface {

	// Write adds the given bytes to the packetizer.
	Write(bts []byte)

	// Next returns the next packet payload or nil if no packet is available.
	Next() ([]byte, error)

	// TargetPacketSize returns the target packet size.
	TargetPacketSize() int
}

// Pacer is an interface for controlling the rate of packet sending.
type Pacer interface {

	// Wait blocks until the correct time to send the given packet has elapsed. The wait time is calculated based on
	// timing references (e.g. RTP timestamps or MPEG-TS Program Clock References).
	Wait(bts []byte)

	// CalculateWait calculates the wait time for the given packet based on the current time.
	CalculateWait(now utc.UTC, bts []byte) time.Duration

	// SetDelay configures the pacer to wait for the given delay before sending the first packet. This allows
	// smoothening jitter on the first packets. The default is 0.
	SetDelay(delay time.Duration)
}

// AsyncPacer is an asynchronous pacer using Push/Pop instead of the synchronous Wait method.
type AsyncPacer interface {
	Pacer

	// Push pushes the given packet to the pacer. Use pop to retrieve the packet at the correct send time. Returns an
	// error if the packet is invalid or the pacer was shutdown.
	Push(bts []byte) error

	// Pop returns the next packet when it's ready to be sent. If no packet is available, it blocks until the next
	// packet is available. Returns an error if the pacer was shutdown.
	Pop() (bts []byte, err error)

	// Shutdown terminates the pacer and unblocks any pending Pop calls. If an error is provided, it is returned to all
	// pending or future Push/Pop calls.
	Shutdown(err ...error)
}
