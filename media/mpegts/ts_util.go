package mpegts

import (
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/Comcast/gots/v2/pes"
)

const (
	// Maximum value for PCR base (33 bits @ 90 kHz)
	maxPCRBase = (1 << 33) - 1
	// Maximum value for PCR extension (9 bits @ 27 MHz)
	maxPCRExt = (1 << 9) - 1
	// MaxPCR is the maximum value for PCR value (42 bits)
	MaxPCR = maxPCRBase*300 + maxPCRExt

	MaxPTS = (1 << 33) - 1
)

// ExtractPCR extracts the PCR (Program Clock Reference) from a given TS packet, if available.
// Returns
//	- pid: the PID of the packet
//	- pcr: the PCR value
//	- ok: true if the packet contains a PCR, false otherwise
func ExtractPCR(pkt *packet.Packet) (pcr uint64, ok bool) {
	af, err := pkt.AdaptationField()
	if err != nil {
		return
	}

	pcr, err = af.PCR()
	if err != nil {
		return
	}

	return pcr, true
}

// ExtractPTS extracts the PTS and DTS from a given TS packet, if available.
// Returns
//	- pid: the PID of the packet
//	- pts: the presentation timestamp PTS of the packet
//	- dts: the decoding timestamp DTS of the packet. Is set to the PTS if no specific DTS is present.
//	- ok: true if the packet contains a PTS / DTS, false otherwise
func ExtractPTS(pkt *packet.Packet) (pts, dts uint64, ok bool) {
	// Check for Payload Unit Start Indicator (PUSI)
	if !pkt.PayloadUnitStartIndicator() {
		return
	}

	// Parse PES packet from the TS packet's payload
	payload, err := pkt.Payload()
	if err != nil {
		return
	}

	pesPacket, err := pes.NewPESHeader(payload)
	if err != nil {
		return
	}

	if pesPacket.HasPTS() { // presentation time stamp
		pts = pesPacket.PTS()
		// default DTS to PTS according to
		// https://www.etsi.org/deliver/etsi_ts/101100_101199/101154/01.09.01_60/ts_101154v010901p.pdf
		dts = pts
		ok = true
	}

	if pesPacket.HasDTS() { // decoding time stamp
		dts = pesPacket.DTS()
	}

	return
}

func PcrDiff(p1, p2 uint64) time.Duration {
	var diff uint64
	if p2 >= p1 {
		diff = p2 - p1
	} else {
		diff = MaxPCR - p1 + p2 + 1
	}
	return PcrToDuration(diff)
}

func PcrToDuration(diff uint64) time.Duration {
	// PCR is in 27 mHz units, i.e. 1 tick = 1/27000000 s
	return time.Duration(diff) * time.Microsecond / 27
}

func PtsToDuration(diff uint64) time.Duration {
	// PTS/DTS is in 90 kHz units, i.e. 1 tick = 1/90000 s
	return time.Duration(diff) * 100 * time.Microsecond / 9
}
