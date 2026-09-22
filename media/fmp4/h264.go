package fmp4

import (
	"bytes"
	"encoding/binary"

	"github.com/eluv-io/errors-go"
)

// H.264 NAL unit types relevant to remuxing (ISO/IEC 14496-10, table 7-1).
const (
	NaluIDR = 5
	NaluSEI = 6
	NaluSPS = 7
	NaluPPS = 8
	NaluAUD = 9
	NaluFIL = 12
)

// NaluType returns the nal_unit_type of a NAL unit.
func NaluType(nalu []byte) byte {
	if len(nalu) == 0 {
		return 0
	}
	return nalu[0] & 0x1f
}

// SplitAVCC splits an AVCC-formatted access unit (each NAL unit prefixed by its length in lengthSize big-endian bytes)
// into its NAL units. The returned slices alias sample. A truncated or zero-length prefix is an error, since a sample
// that does not parse would otherwise be forwarded as garbage.
func SplitAVCC(sample []byte, lengthSize int) ([][]byte, error) {
	e := errors.TemplateNoTrace("SplitAVCC", errors.K.Invalid.Default(), "length_size", lengthSize)
	if lengthSize < 1 || lengthSize > 4 {
		return nil, e("reason", "unsupported NALU length size")
	}
	var nalus [][]byte
	pos := 0
	for pos < len(sample) {
		if len(sample)-pos < lengthSize {
			return nil, e("reason", "truncated NALU length prefix", "offset", pos, "sample_size", len(sample))
		}
		var n int
		switch lengthSize {
		case 4:
			n = int(binary.BigEndian.Uint32(sample[pos:]))
		case 3:
			n = int(sample[pos])<<16 | int(sample[pos+1])<<8 | int(sample[pos+2])
		case 2:
			n = int(binary.BigEndian.Uint16(sample[pos:]))
		default:
			n = int(sample[pos])
		}
		pos += lengthSize
		if n == 0 {
			return nil, e("reason", "zero-length NALU", "offset", pos)
		}
		if n > len(sample)-pos {
			return nil, e("reason", "truncated NALU", "offset", pos, "nalu_size", n, "sample_size", len(sample))
		}
		nalus = append(nalus, sample[pos:pos+n])
		pos += n
	}
	return nalus, nil
}

// accessUnit is the result of inspecting the NAL units of one H.264 sample for remuxing.
type accessUnit struct {
	nalus          [][]byte // the sample's NAL units minus access unit delimiters and filler data
	idr            bool     // the sample contains an IDR slice
	paramSetChange bool     // the sample carries an SPS or PPS that differs from the track's
}

// inspectAccessUnit drops the NAL units an RTMP receiver does not want (AUD, filler), flags IDR pictures and detects
// in-band parameter sets that differ from those of the init segment. In-band SPS/PPS are kept in the returned NAL
// units: a receiver decodes them like any other, and a changed set is what triggers a re-announcement upstream.
func inspectAccessUnit(nalus [][]byte, sps, pps [][]byte) accessUnit {
	au := accessUnit{nalus: make([][]byte, 0, len(nalus))}
	for _, nalu := range nalus {
		switch NaluType(nalu) {
		case NaluAUD, NaluFIL:
			continue
		case NaluIDR:
			au.idr = true
		case NaluSPS:
			if !containsNalu(sps, nalu) {
				au.paramSetChange = true
			}
		case NaluPPS:
			if !containsNalu(pps, nalu) {
				au.paramSetChange = true
			}
		}
		au.nalus = append(au.nalus, nalu)
	}
	return au
}

func containsNalu(set [][]byte, nalu []byte) bool {
	for _, s := range set {
		if bytes.Equal(s, nalu) {
			return true
		}
	}
	return false
}
