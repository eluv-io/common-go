package rtmp

import (
	"fmt"
	"strings"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"

	"github.com/eluv-io/common-go/media/fmp4"
)

// Check returns the reasons a stream with the given codec parameters may be refused or flagged by the common
// contribution services (YouTube, Facebook), gathered from their published ingest requirements. None of them stops
// Dial; they are for the operator's attention.
func Check(cfg Config, params fmp4.CodecParams) []string {
	var warnings []string
	if params.Video == nil || len(params.Video.SPS) == 0 {
		return append(warnings, "no video track: contribution services require video")
	}

	var sps h264.SPS
	if err := sps.Unmarshal(params.Video.SPS[0]); err != nil {
		warnings = append(warnings, "video SPS does not parse: "+err.Error())
	} else {
		switch sps.ProfileIdc {
		case 66, 77, 100: // baseline, main, high
		default:
			warnings = append(warnings,
				fmt.Sprintf("H.264 profile_idc %d: services expect baseline (66), main (77) or high (100)", sps.ProfileIdc))
		}
		if sps.FPS() == 0 && cfg.Meta.FrameRate == 0 {
			warnings = append(warnings, "frame rate unknown: the SPS carries no timing info and none is configured")
		}
	}

	if params.Audio == nil {
		warnings = append(warnings, "no audio track: YouTube and Facebook require an audio track")
	} else {
		var asc mpeg4audio.AudioSpecificConfig
		if err := asc.Unmarshal(params.Audio.ASC); err != nil {
			warnings = append(warnings, "AudioSpecificConfig does not parse: "+err.Error())
		} else {
			if asc.Type != mpeg4audio.ObjectTypeAACLC {
				warnings = append(warnings, fmt.Sprintf("AAC object type %d: services expect AAC-LC (2)", asc.Type))
			}
			if asc.SampleRate != 44100 && asc.SampleRate != 48000 {
				warnings = append(warnings,
					fmt.Sprintf("audio sample rate %d Hz: services expect 44100 or 48000", asc.SampleRate))
			}
			channels := asc.ChannelCount
			if channels == 0 {
				channels = int(asc.ChannelConfig)
			}
			if channels != 2 {
				warnings = append(warnings, fmt.Sprintf("%d audio channels: services expect stereo", channels))
			}
		}
	}

	if !cfg.Encrypted() && strings.Contains(strings.ToLower(cfg.URL), "facebook") {
		warnings = append(warnings, "Facebook accepts RTMPS only - use an rtmps:// URL")
	}
	return warnings
}
