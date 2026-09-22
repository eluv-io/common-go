// Package fmp4 reads fragmented MP4 (CMAF style) init and media segments into elementary access units, for remuxing
// into containers that carry raw H.264 and AAC frames such as FLV/RTMP.
package fmp4

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Eyevinn/mp4ff/aac"
	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/eluv-io/errors-go"
)

// Kind is the media type of a track.
type Kind uint8

const (
	KindUnknown Kind = iota
	KindVideo
	KindAudio
)

func (k Kind) String() string {
	switch k {
	case KindVideo:
		return "video"
	case KindAudio:
		return "audio"
	default:
		return "unknown"
	}
}

// Codec is the codec of a track, limited to what the remux targets accept.
type Codec uint8

const (
	CodecUnsupported Codec = iota
	CodecH264
	CodecAAC
)

func (c Codec) String() string {
	switch c {
	case CodecH264:
		return "h264"
	case CodecAAC:
		return "aac"
	default:
		return "unsupported"
	}
}

// ErrUnsupportedCodec is the cause of a ParseInit error when a track uses a codec other than H.264 or AAC.
var ErrUnsupportedCodec = errors.Str("unsupported codec")

// TrackInfo describes one track of an init segment.
type TrackInfo struct {
	TrackID   uint32
	Kind      Kind
	Codec     Codec
	Codecs    string       // RFC 6381 codec string, e.g. "avc1.640028" or "mp4a.40.2"
	Timescale uint32       // ticks per second of the track's timestamps
	Trex      *mp4.TrexBox // default sample values, needed to interpret fragments whose tfhd omits them
	// EditMediaTime is the media time the presentation starts at, taken from the first non-empty edit list entry.
	// Sample timestamps have it subtracted so they are presentation-relative. 0 when there is no edit list.
	EditMediaTime int64

	// H.264
	SPS, PPS       [][]byte
	NALULengthSize int // bytes of the NAL unit length prefix in samples; the AVC file format in use always writes 4
	Width, Height  int

	// AAC
	ASC        []byte // raw AudioSpecificConfig, the payload of the AAC sequence header
	ObjectType int    // AAC audio object type, 2 for AAC-LC
	SampleRate int
	Channels   int
}

// CodecEqual reports whether o carries the same codec configuration as t: the same SPS/PPS or the same
// AudioSpecificConfig. A change means a receiver has to be re-announced.
func (t *TrackInfo) CodecEqual(o *TrackInfo) bool {
	if t == nil || o == nil {
		return t == o
	}
	if t.Codec != o.Codec {
		return false
	}
	switch t.Codec {
	case CodecH264:
		return nalusEqual(t.SPS, o.SPS) && nalusEqual(t.PPS, o.PPS)
	case CodecAAC:
		return bytes.Equal(t.ASC, o.ASC)
	default:
		return false
	}
}

func nalusEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// VideoParams is the codec configuration of an H.264 track, as a sink needs it.
type VideoParams struct {
	Codecs        string
	SPS, PPS      [][]byte
	Width, Height int
}

// AudioParams is the codec configuration of an AAC track, as a sink needs it.
type AudioParams struct {
	Codecs     string
	ASC        []byte
	ObjectType int
	SampleRate int
	Channels   int
}

// CodecParams is the codec configuration a sink publishes: the video track and, optionally, the audio track.
type CodecParams struct {
	Video *VideoParams
	Audio *AudioParams
}

// Equal reports whether both configurations are the same.
func (p CodecParams) Equal(o CodecParams) bool {
	switch {
	case (p.Video == nil) != (o.Video == nil), (p.Audio == nil) != (o.Audio == nil):
		return false
	}
	if p.Video != nil && !(nalusEqual(p.Video.SPS, o.Video.SPS) && nalusEqual(p.Video.PPS, o.Video.PPS)) {
		return false
	}
	if p.Audio != nil && !bytes.Equal(p.Audio.ASC, o.Audio.ASC) {
		return false
	}
	return true
}

// VideoParams returns the track's video configuration, or nil for a non-H.264 track.
func (t *TrackInfo) VideoParams() *VideoParams {
	if t == nil || t.Codec != CodecH264 {
		return nil
	}
	return &VideoParams{Codecs: t.Codecs, SPS: t.SPS, PPS: t.PPS, Width: t.Width, Height: t.Height}
}

// AudioParams returns the track's audio configuration, or nil for a non-AAC track.
func (t *TrackInfo) AudioParams() *AudioParams {
	if t == nil || t.Codec != CodecAAC {
		return nil
	}
	return &AudioParams{Codecs: t.Codecs, ASC: t.ASC, ObjectType: t.ObjectType, SampleRate: t.SampleRate,
		Channels: t.Channels}
}

// Init is a parsed init segment.
type Init struct {
	Tracks []TrackInfo
}

// Track returns the first track of the given kind, or nil.
func (i *Init) Track(kind Kind) *TrackInfo {
	for idx := range i.Tracks {
		if i.Tracks[idx].Kind == kind {
			return &i.Tracks[idx]
		}
	}
	return nil
}

// ParseInit parses an init segment (ftyp + moov). Every track must be H.264 or AAC; any other codec fails with
// ErrUnsupportedCodec as the cause, since the remux targets cannot carry it and a silent skip would leave a track
// behind.
func ParseInit(r io.Reader) (*Init, error) {
	e := errors.Template("fmp4.ParseInit", errors.K.Invalid.Default())

	f, err := mp4.DecodeFile(r)
	if err != nil {
		return nil, e(err, "reason", "failed to decode init segment")
	}
	moov := f.Moov
	if f.Init != nil && f.Init.Moov != nil {
		moov = f.Init.Moov
	}
	if moov == nil {
		return nil, e("reason", "no moov box in init segment")
	}
	if len(moov.Traks) == 0 {
		return nil, e("reason", "no tracks in init segment")
	}

	init := &Init{}
	for _, trak := range moov.Traks {
		track, err := parseTrack(trak, moov.Mvex)
		if err != nil {
			return nil, e(err)
		}
		init.Tracks = append(init.Tracks, track)
	}
	return init, nil
}

func parseTrack(trak *mp4.TrakBox, mvex *mp4.MvexBox) (TrackInfo, error) {
	var t TrackInfo
	if trak.Tkhd == nil || trak.Mdia == nil || trak.Mdia.Mdhd == nil || trak.Mdia.Minf == nil ||
		trak.Mdia.Minf.Stbl == nil || trak.Mdia.Minf.Stbl.Stsd == nil {
		return t, errors.E("parseTrack", errors.K.Invalid, "reason", "incomplete trak box")
	}
	t.TrackID = trak.Tkhd.TrackID
	t.Timescale = trak.Mdia.Mdhd.Timescale
	e := errors.Template("parseTrack", errors.K.Invalid.Default(), "track_id", t.TrackID)
	if t.Timescale == 0 {
		return t, e("reason", "zero timescale")
	}

	if mvex != nil {
		for _, trex := range mvex.Trexs {
			if trex.TrackID == t.TrackID {
				t.Trex = trex
				break
			}
		}
	}
	if trak.Edts != nil {
		for _, elst := range trak.Edts.Elst {
			for _, entry := range elst.Entries {
				if entry.MediaTime >= 0 {
					t.EditMediaTime = entry.MediaTime
					break
				}
			}
		}
	}

	stsd := trak.Mdia.Minf.Stbl.Stsd
	switch {
	case stsd.AvcX != nil:
		t.Kind = KindVideo
		if err := parseAvc(&t, stsd.AvcX); err != nil {
			return t, e(err)
		}
	case stsd.Mp4a != nil:
		t.Kind = KindAudio
		if err := parseMp4a(&t, stsd.Mp4a); err != nil {
			return t, e(err)
		}
	default:
		typ := "unknown"
		switch {
		case stsd.HvcX != nil:
			t.Kind = KindVideo
			typ = stsd.HvcX.Type()
		case stsd.VvcX != nil, stsd.VpXX != nil, stsd.Encv != nil:
			t.Kind = KindVideo
			typ = firstSampleEntryType(stsd)
		case stsd.Opus != nil, stsd.MhXX != nil, stsd.Enca != nil:
			t.Kind = KindAudio
			typ = firstSampleEntryType(stsd)
		default:
			typ = firstSampleEntryType(stsd)
		}
		t.Codecs = typ
		return t, e(ErrUnsupportedCodec, "sample_entry", typ)
	}
	return t, nil
}

func firstSampleEntryType(stsd *mp4.StsdBox) string {
	if len(stsd.Children) > 0 {
		return stsd.Children[0].Type()
	}
	return "unknown"
}

func parseAvc(t *TrackInfo, se *mp4.VisualSampleEntryBox) error {
	e := errors.Template("parseAvc", errors.K.Invalid.Default())
	if se.AvcC == nil {
		return e("reason", "avcC box missing")
	}
	if len(se.AvcC.SPSnalus) == 0 || len(se.AvcC.PPSnalus) == 0 {
		return e("reason", "avcC without SPS or PPS")
	}
	t.Codec = CodecH264
	t.SPS = cloneNalus(se.AvcC.SPSnalus)
	t.PPS = cloneNalus(se.AvcC.PPSnalus)
	// mp4ff's decoder configuration record does not expose lengthSizeMinusOne; every AVC file format writer in use
	// (ffmpeg included) writes 4-byte prefixes, which mp4ff's own AVCC helpers assume as well.
	t.NALULengthSize = 4
	t.Width, t.Height = int(se.Width), int(se.Height)
	sps, err := avc.ParseSPSNALUnit(t.SPS[0], false)
	if err != nil {
		return e(err, "reason", "failed to parse SPS")
	}
	if sps.Width > 0 && sps.Height > 0 {
		t.Width, t.Height = int(sps.Width), int(sps.Height)
	}
	t.Codecs = avc.CodecString(se.Type(), sps)
	return nil
}

func parseMp4a(t *TrackInfo, se *mp4.AudioSampleEntryBox) error {
	e := errors.Template("parseMp4a", errors.K.Invalid.Default())
	if se.Esds == nil || se.Esds.DecConfigDescriptor == nil || se.Esds.DecConfigDescriptor.DecSpecificInfo == nil {
		return e("reason", "esds box without decoder specific info")
	}
	ascBytes := se.Esds.DecConfigDescriptor.DecSpecificInfo.DecConfig
	asc, err := aac.DecodeAudioSpecificConfig(bytes.NewReader(ascBytes))
	if err != nil {
		return e(err, "reason", "failed to decode AudioSpecificConfig")
	}
	t.Codec = CodecAAC
	t.ASC = append([]byte(nil), ascBytes...)
	t.ObjectType = int(asc.ObjectType)
	t.Codecs = fmt.Sprintf("mp4a.40.%d", asc.ObjectType)
	t.SampleRate = asc.SamplingFrequency
	if asc.SBRPresentFlag && asc.ExtensionFrequency > 0 {
		t.SampleRate = asc.ExtensionFrequency
	}
	t.Channels = channelsFromConfig(asc.ChannelConfiguration)
	if t.Channels == 0 {
		t.Channels = int(se.ChannelCount)
	}
	return nil
}

// channelsFromConfig maps an AAC channel configuration (ISO/IEC 14496-3 table 1.19) to a channel count; 0 means the
// configuration does not state it.
func channelsFromConfig(cfg byte) int {
	switch {
	case cfg >= 1 && cfg <= 6:
		return int(cfg)
	case cfg == 7:
		return 8
	default:
		return 0
	}
}

func cloneNalus(in [][]byte) [][]byte {
	out := make([][]byte, len(in))
	for i, n := range in {
		out[i] = append([]byte(nil), n...)
	}
	return out
}
