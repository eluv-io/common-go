package mpegts

import (
	"fmt"

	"github.com/Comcast/gots/v2/packet"

	"github.com/eluv-io/errors-go"
)

// TsValidator is a simple component that can be used to validate MPEG TS packets.
type TsValidator interface {
	// Validate validates the MPEG TS packets contained in the given byte slice.
	//
	// The packet bytes (bts) can consist of a single or multiple TS packets. The method will validate one packet after the
	// other until a validation error occurs. Returns nil if all packets are valid.
	Validate(bts []byte) error
}

// NewTsValidator creates a validator for MPEG TS packets.
func NewTsValidator() TsValidator {
	return &tsValidator{cc: make(map[int]int)}
}

type tsValidator struct {
	cc map[int]int
}

func (p *tsValidator) Validate(bts []byte) (errList error) {
	errCount := 0
	packetCount := 0

	appendErr := func(err error) {
		errList = errors.Append(errList, err)
		errCount++
	}

	for ; len(bts) >= packet.PacketSize; bts = bts[packet.PacketSize:] {
		if errCount >= 20 {
			appendErr(fmt.Errorf("too many errors: %d", errCount))
			return errList
		}
		packetCount++
		pkt := packet.Packet(bts)

		err := pkt.CheckErrors()
		if err != nil {
			appendErr(fmt.Errorf("checkerr=%s ts-packet=%d", err, packetCount))
			continue
		}

		if pkt.HasPayload() {
			pid := pkt.PID()
			if pid == packet.NullPacketPid {
				// skip null packets
			} else if cc, ok := p.cc[pid]; !ok {
				p.cc[pid] = pkt.ContinuityCounter()
			} else {
				newCc := pkt.ContinuityCounter()
				p.cc[pid] = newCc
				if newCc != (cc+1)%16 {
					err = fmt.Errorf("continuity counter mismatch: expected=%02d actual=%02d ts-packet=%d pid=%d", (cc+1)%16, newCc, packetCount, pid)
					appendErr(err)
					continue
				}
			}
		}

	}
	if len(bts) > 0 {
		err := fmt.Errorf("packet too short: %d ts-packet=%d", len(bts), packetCount+1)
		appendErr(err)
	}

	return errList
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopValidator struct{}

func (n NoopValidator) Validate([]byte) error { return nil }
