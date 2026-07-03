package mpegts

import (
	"encoding/binary"

	"github.com/eluv-io/errors-go"
)

// AtsTimestampLen is the size in bytes of the arrival-timestamp prefix on an ATS-TS (Arrival-Time-Stamped MPEG-TS)
// value: an int64 nanoseconds-since-Unix-epoch value encoded big-endian, followed by the raw MPEG-TS packets of a
// single received datagram. It mirrors avpipe's broadcastproto/tlv.AtsTimestampLen (the value of a TlvTypeAtsTs blob);
// the TLV framing itself is handled by the transport/packetizer layer, so consumers here receive only the value part.
const AtsTimestampLen = 8

// ParseAtsTs splits an ATS-TS value into its arrival timestamp (nanoseconds since the Unix epoch) and the raw MPEG-TS
// payload. It returns an error if the data is too short to contain the timestamp prefix. See AtsTimestampLen.
func ParseAtsTs(bts []byte) (arrivalNs int64, payload []byte, err error) {
	if len(bts) < AtsTimestampLen {
		return 0, nil, errors.NoTrace("ParseAtsTs", errors.K.Invalid,
			"reason", "data too short for arrival timestamp", "len", len(bts))
	}
	return int64(binary.BigEndian.Uint64(bts[:AtsTimestampLen])), bts[AtsTimestampLen:], nil
}
