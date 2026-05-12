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

func (l *LossDetector) Next(packet *rtp.Packet) error {
	defer func() {
		l.lastSeq = packet.SequenceNumber
	}()

	if !l.hasSeq {
		l.hasSeq = true
		return nil
	}
	expectedSequenceNumber := l.lastSeq + 1
	if packet.SequenceNumber != expectedSequenceNumber {
		return errors.NoTrace("LossDetector.Next", errors.K.Invalid,
			"reason", "packet loss detected",
			"expected_seq", expectedSequenceNumber,
			"new_seq", packet.SequenceNumber,
			"lost_packets", packet.SequenceNumber-expectedSequenceNumber)
	}
	return nil
}
