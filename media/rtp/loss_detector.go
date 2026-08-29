package rtp

import (
	"github.com/pion/rtp"

	"github.com/eluv-io/errors-go"
)

// LossDetector detects packet loss based on the sequence number in the RTP header. The zero value is ready to use.
type LossDetector struct {
	hasSeq  bool
	lastSeq uint16
}

// Next checks the sequence number of the next RTP packet. Returns an error if the sequence number is not contiguous
// with respect to the last received packet.
func (l *LossDetector) Next(packet *rtp.Packet) error {
	return l.NextSequence(packet.SequenceNumber)
}

// NextPacket checks the sequence number of the next RTP packet. Returns an error if the sequence number is not
// contiguous with respect to the last received packet.
func (l *LossDetector) NextPacket(packet *rtp.Packet) error {
	return l.Next(packet)
}

// NextSequence checks the RTP sequence number. Returns an error if the sequence number is not contiguous
// with respect to the last received sequence number.
func (l *LossDetector) NextSequence(seq uint16) error {
	defer func() {
		l.lastSeq = seq
	}()

	if !l.hasSeq {
		l.hasSeq = true
		return nil
	}
	expectedSequenceNumber := l.lastSeq + 1
	if seq != expectedSequenceNumber {
		return errors.NoTrace("LossDetector.Next", errors.K.Invalid,
			"reason", "packet loss detected",
			"expected_seq", expectedSequenceNumber,
			"new_seq", seq,
			"lost_packets", seq-expectedSequenceNumber)
	}
	return nil
}
