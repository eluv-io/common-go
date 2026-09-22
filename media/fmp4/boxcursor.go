package fmp4

import (
	"encoding/binary"

	"github.com/eluv-io/errors-go"
)

// errShortBox is the accumulated error of a boxCursor that was asked for more bytes than its box holds. It is a
// package value so that the failure path allocates nothing either.
var errShortBox = errors.NoTrace("boxCursor", errors.K.Invalid, "reason", "read beyond the end of the box")

// boxCursor reads big-endian fields out of one box body held in memory. It mirrors mp4ff's bits.FixedSliceReader,
// including the accumulated-error style, but is a value type: mp4ff's reader is pointer-constructed and has no
// Reset, so using it per fragment would allocate. Once the mp4ff fork gains FixedSliceReader.Reset, this type can go
// and the decoders below switch to it unchanged.
type boxCursor struct {
	b   []byte
	pos int
	err error
}

func newBoxCursor(b []byte) boxCursor {
	return boxCursor{b: b}
}

// AccError returns the first error accumulated by the read methods, nil if every read was within bounds.
func (c *boxCursor) AccError() error {
	return c.err
}

// NrRemainingBytes returns the number of bytes not yet read.
func (c *boxCursor) NrRemainingBytes() int {
	return len(c.b) - c.pos
}

// Pos returns the read position within the box body.
func (c *boxCursor) Pos() int {
	return c.pos
}

func (c *boxCursor) fail() {
	if c.err == nil {
		c.err = errShortBox
	}
	c.pos = len(c.b)
}

func (c *boxCursor) ReadUint8() byte {
	if c.NrRemainingBytes() < 1 {
		c.fail()
		return 0
	}
	v := c.b[c.pos]
	c.pos++
	return v
}

func (c *boxCursor) ReadUint16() uint16 {
	if c.NrRemainingBytes() < 2 {
		c.fail()
		return 0
	}
	v := binary.BigEndian.Uint16(c.b[c.pos:])
	c.pos += 2
	return v
}

func (c *boxCursor) ReadUint24() uint32 {
	if c.NrRemainingBytes() < 3 {
		c.fail()
		return 0
	}
	v := uint32(c.b[c.pos])<<16 | uint32(c.b[c.pos+1])<<8 | uint32(c.b[c.pos+2])
	c.pos += 3
	return v
}

func (c *boxCursor) ReadUint32() uint32 {
	if c.NrRemainingBytes() < 4 {
		c.fail()
		return 0
	}
	v := binary.BigEndian.Uint32(c.b[c.pos:])
	c.pos += 4
	return v
}

func (c *boxCursor) ReadInt32() int32 {
	return int32(c.ReadUint32())
}

func (c *boxCursor) ReadUint64() uint64 {
	if c.NrRemainingBytes() < 8 {
		c.fail()
		return 0
	}
	v := binary.BigEndian.Uint64(c.b[c.pos:])
	c.pos += 8
	return v
}

// SkipBytes advances the cursor by n bytes.
func (c *boxCursor) SkipBytes(n int) {
	if n < 0 || c.NrRemainingBytes() < n {
		c.fail()
		return
	}
	c.pos += n
}

// Sub returns a cursor over the next n bytes and advances past them.
func (c *boxCursor) Sub(n int) boxCursor {
	if n < 0 || c.NrRemainingBytes() < n {
		c.fail()
		return boxCursor{}
	}
	sub := boxCursor{b: c.b[c.pos : c.pos+n]}
	c.pos += n
	return sub
}
