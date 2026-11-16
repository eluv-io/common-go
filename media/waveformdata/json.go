package waveformdata

import (
	"encoding/json"

	"github.com/eluv-io/errors-go"
)

// jsonWaveform is the JSON form produced by audiowaveform --output-format json and read by waveform-data.js.
type jsonWaveform struct {
	Version         int     `json:"version"`
	Channels        int     `json:"channels"`
	SampleRate      int     `json:"sample_rate"`
	SamplesPerPixel int     `json:"samples_per_pixel"`
	Bits            int     `json:"bits"`
	Length          int     `json:"length"`
	Data            []int16 `json:"data"`
}

// MarshalJSON emits the JSON form. The version is always 2, as with Encode.
func (w *Waveform) MarshalJSON() ([]byte, error) {
	if err := w.Validate(); err != nil {
		return nil, errors.E("waveformdata.MarshalJSON", err)
	}
	data := w.Data
	if data == nil {
		data = []int16{}
	}
	return json.Marshal(jsonWaveform{
		Version:         Version,
		Channels:        w.Channels,
		SampleRate:      w.SampleRate,
		SamplesPerPixel: w.SamplesPerPixel,
		Bits:            w.Bits,
		Length:          w.Length(),
		Data:            data,
	})
}

// UnmarshalJSON parses the JSON form and checks that the data length matches the header.
func (w *Waveform) UnmarshalJSON(b []byte) error {
	e := errors.Template("waveformdata.UnmarshalJSON", errors.K.Invalid)
	var j jsonWaveform
	if err := json.Unmarshal(b, &j); err != nil {
		return e(err)
	}
	res := Waveform{
		Version:         j.Version,
		Channels:        j.Channels,
		SampleRate:      j.SampleRate,
		SamplesPerPixel: j.SamplesPerPixel,
		Bits:            j.Bits,
		Data:            j.Data,
	}
	if err := res.Validate(); err != nil {
		return e(err)
	}
	if res.Length() != j.Length {
		return e("reason", "length does not match data", "length", j.Length, "data_buckets", res.Length())
	}
	*w = res
	return nil
}
