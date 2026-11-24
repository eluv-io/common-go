package preamble

import (
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/eluv-io/errors-go"
)

// defaultFormat is the format of a preamble written without one.
const defaultFormat = "/raw"

// liveEncMode returns the encoding mode of LiveFormat preambles, which encodes deterministically. The codecs package
// cannot be used here: it imports this package through format/hash.
var liveEncMode = sync.OnceValues(func() (cbor.EncMode, error) {
	return cbor.CoreDetEncOptions().EncMode()
})

// LiveFormat is the preamble format of a live part that carries settings for the fabric, such as timeouts. The part
// writer's own preamble is nested inside it.
const LiveFormat = "/fabric-live"

// LiveVersion is the version of the LiveFormat encoding.
const LiveVersion = 1

// Live is the content of a LiveFormat preamble.
//
// Fields are encoded by number and only when set. A reader ignores fields it does not know, so a new field may only add
// information that an older reader can do without.
type Live struct {
	Version  uint          `cbor:"1,keyasint"`
	Timeouts *LiveTimeouts `cbor:"2,keyasint,omitempty"`
	// Format is the format of the part writer's own preamble.
	Format string `cbor:"3,keyasint,omitempty"`
	// Data is the part writer's own preamble.
	Data []byte `cbor:"4,keyasint,omitempty"`
}

// LiveTimeouts are the timeouts of a live part. A zero value means the node's configured timeout applies.
type LiveTimeouts struct {
	// Init is the longest wait for the first byte of the part's data.
	Init time.Duration `cbor:"1,keyasint,omitempty"`
	// Idle is the longest wait for more data once data has started.
	Idle time.Duration `cbor:"2,keyasint,omitempty"`
	// Transfer is the longest time a transfer of the whole part to another node may take.
	Transfer time.Duration `cbor:"3,keyasint,omitempty"`
}

// IsZero returns whether no timeout is set.
func (t *LiveTimeouts) IsZero() bool {
	return t == nil || *t == LiveTimeouts{}
}

// EncodeLive returns the preamble data and format of a live part with the given timeouts and the part writer's own
// preamble. Without timeouts, it returns the writer's preamble as is, so parts without settings keep their format. The
// writer's format is normalized and validated as by Write in both cases.
func EncodeLive(timeouts *LiveTimeouts, data []byte, format string) ([]byte, string, error) {
	e := errors.Template("preamble.EncodeLive", errors.K.Invalid)
	format, err := normalizeFormat(format)
	if err != nil {
		return nil, "", e(err)
	}
	if timeouts.IsZero() {
		return data, format, nil
	}
	if timeouts.Init < 0 || timeouts.Idle < 0 || timeouts.Transfer < 0 {
		return nil, "", e("reason", "negative timeout", "timeouts", *timeouts)
	}
	mode, err := liveEncMode()
	if err != nil {
		return nil, "", e(err)
	}
	enc, err := mode.Marshal(&Live{
		Version:  LiveVersion,
		Timeouts: timeouts,
		Format:   format,
		Data:     data,
	})
	if err != nil {
		return nil, "", e(err)
	}
	return enc, LiveFormat, nil
}

// DecodeLive decodes the preamble of a live part. A preamble of another format is returned as the part writer's own
// preamble, without timeouts.
func DecodeLive(data []byte, format string) (*Live, error) {
	if format != LiveFormat {
		return &Live{Format: format, Data: data}, nil
	}
	l := &Live{}
	if err := cbor.Unmarshal(data, l); err != nil {
		return nil, errors.E("preamble.DecodeLive", errors.K.Invalid, err)
	}
	if l.Format == "" {
		l.Format = defaultFormat
	}
	return l, nil
}
