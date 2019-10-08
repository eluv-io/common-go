package bytesize

import (
	"github.com/qluvio/content-fabric/errors"
	"fmt"
	"strconv"
	"strings"
)

// Spec is a bytesize specification representing a size in bytes. It can
// marshal/unmarshal to/from human readable format using units that are a
// multiple of 1024, e.g. 1MB = 1024 * 1024 bytes
type Spec uint64

const (
	B  Spec = 1
	KB      = B << 10
	MB      = KB << 10
	GB      = MB << 10
	TB      = GB << 10
	PB      = TB << 10
	EB      = PB << 10

	maxUint64 uint64 = (1 << 64) - 1
	cutoff    uint64 = maxUint64 / 10
)

var ErrBits = fmt.Errorf("capital prefix with lower-case b represents bits, not bytes")

func (b Spec) Bytes() uint64 {
	return uint64(b)
}

func (b Spec) KBytes() float64 {
	v := b / KB
	r := b % KB
	return float64(v) + float64(r)/float64(KB)
}

func (b Spec) MBytes() float64 {
	v := b / MB
	r := b % MB
	return float64(v) + float64(r)/float64(MB)
}

func (b Spec) GBytes() float64 {
	v := b / GB
	r := b % GB
	return float64(v) + float64(r)/float64(GB)
}

func (b Spec) TBytes() float64 {
	v := b / TB
	r := b % TB
	return float64(v) + float64(r)/float64(TB)
}

func (b Spec) PBytes() float64 {
	v := b / PB
	r := b % PB
	return float64(v) + float64(r)/float64(PB)
}

func (b Spec) EBytes() float64 {
	v := b / EB
	r := b % EB
	return float64(v) + float64(r)/float64(EB)
}

func (b Spec) String() string {
	switch {
	case b == 0:
		return fmt.Sprint("0B")
	case b%EB == 0:
		return fmt.Sprintf("%dEB", b/EB)
	case b%PB == 0:
		return fmt.Sprintf("%dPB", b/PB)
	case b%TB == 0:
		return fmt.Sprintf("%dTB", b/TB)
	case b%GB == 0:
		return fmt.Sprintf("%dGB", b/GB)
	case b%MB == 0:
		return fmt.Sprintf("%dMB", b/MB)
	case b%KB == 0:
		return fmt.Sprintf("%dKB", b/KB)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func (b Spec) HR() string {
	return b.HumanReadable()
}

func (b Spec) HumanReadable() string {
	switch {
	case b > EB:
		return fmt.Sprintf("%.1f EB", b.EBytes())
	case b > PB:
		return fmt.Sprintf("%.1f PB", b.PBytes())
	case b > TB:
		return fmt.Sprintf("%.1f TB", b.TBytes())
	case b > GB:
		return fmt.Sprintf("%.1f GB", b.GBytes())
	case b > MB:
		return fmt.Sprintf("%.1f MB", b.MBytes())
	case b > KB:
		return fmt.Sprintf("%.1f KB", b.KBytes())
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func (b Spec) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b *Spec) UnmarshalText(t []byte) error {
	var val uint64
	var unit string

	// copy for error message
	t0 := string(t)

	var c byte
	var i int

ParseLoop:
	for i < len(t) {
		c = t[i]
		switch {
		case '0' <= c && c <= '9':
			if val > cutoff {
				return b.overflow(t0)
			}

			c = c - '0'
			val *= 10

			if val > val+uint64(c) {
				// val+v b.overflows
				return b.overflow(t0)
			}
			val += uint64(c)
			i++

		case c == ' ':
			// ignore leading whitespace
			i++

		default:
			if i == 0 {
				return b.syntaxError(t0)
			}
			break ParseLoop
		}
	}

	unit = strings.TrimSpace(string(t[i:]))
	switch unit {
	case "Kb", "Mb", "Gb", "Tb", "Pb", "Eb":
		*b = 0
		return errors.E("unmarshal bytesize spec", errors.K.Invalid, ErrBits, "from", t0)
	}
	unit = strings.ToLower(unit)
	switch unit {
	case "", "b", "byte":
		// no conversion needed

	case "k", "kb", "kilo", "kilobyte", "kilobytes":
		if val > maxUint64/uint64(KB) {
			return b.overflow(t0)
		}
		val *= uint64(KB)

	case "m", "mb", "mega", "megabyte", "megabytes":
		if val > maxUint64/uint64(MB) {
			return b.overflow(t0)
		}
		val *= uint64(MB)

	case "g", "gb", "giga", "gigabyte", "gigabytes":
		if val > maxUint64/uint64(GB) {
			return b.overflow(t0)
		}
		val *= uint64(GB)

	case "t", "tb", "tera", "terabyte", "terabytes":
		if val > maxUint64/uint64(TB) {
			return b.overflow(t0)
		}
		val *= uint64(TB)

	case "p", "pb", "peta", "petabyte", "petabytes":
		if val > maxUint64/uint64(PB) {
			return b.overflow(t0)
		}
		val *= uint64(PB)

	case "E", "EB", "e", "eb", "eB":
		if val > maxUint64/uint64(EB) {
			return b.overflow(t0)
		}
		val *= uint64(EB)

	default:
		return b.syntaxError(t0)
	}

	*b = Spec(val)
	return nil
}

func (b *Spec) syntaxError(s string) error {
	*b = 0
	return errors.E("unmarshal bytesize spec", errors.K.Invalid, strconv.ErrSyntax, "from", s)
}

func (b *Spec) overflow(s string) error {
	*b = Spec(maxUint64)
	return errors.E("unmarshal bytesize spec", errors.K.Invalid, strconv.ErrRange, "from", s)
}

func (b *Spec) UnmarshalJSON(t []byte) error {
	if t[0] == '"' && t[len(t)-1] == '"' {
		return b.UnmarshalText(t[1 : len(t)-1])
	}
	v, err := strconv.ParseUint(string(t), 10, 64)
	*b = Spec(v)
	return err
}

// FromString parses the given bytesize string into a bytesize spec.
func FromString(s string) (Spec, error) {
	spec := Spec(0)
	err := spec.UnmarshalText([]byte(s))
	if err != nil {
		return 0, errors.E("parse", err, "bytesize_spec", s)
	}
	return spec, nil
}

// MustParse parses the given bytesize string into a bytesize spec, panicking in
// case of errors.
func MustParse(s string) Spec {
	spec, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return spec
}
