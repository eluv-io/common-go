package bytesize

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/eluv-io/errors-go"
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
var ErrInvalidFraction = fmt.Errorf("invalid fraction")

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

// HR returns a human readable form, followed by the precise byte count in
// parenthesis: "5.3 MB (5341321)"
func (b Spec) HR() string {
	return b.HumanReadable() + " (" + b.String() + ")"
}

func (b Spec) HumanReadable() string {
	switch {
	case b > EB:
		return fmt.Sprintf("%.1fEB", b.EBytes())
	case b > PB:
		return fmt.Sprintf("%.1fPB", b.PBytes())
	case b > TB:
		return fmt.Sprintf("%.1fTB", b.TBytes())
	case b > GB:
		return fmt.Sprintf("%.1fGB", b.GBytes())
	case b > MB:
		return fmt.Sprintf("%.1fMB", b.MBytes())
	case b > KB:
		return fmt.Sprintf("%.1fKB", b.KBytes())
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func (b Spec) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b *Spec) UnmarshalText(t []byte) error {
	res, err := unmarshalText(t)
	*b = res
	return err
}

func unmarshalText(t []byte) (Spec, error) {
	// copy for error message
	t0 := string(t)

StartOver:
	spec := Spec(0)
	b := &spec
	var val uint64
	var fraction uint64
	var fractionDiv uint64 = 1
	var parseFraction bool
	var unit string
	var unitMul uint64 = 1

	var i int

ParseLoop:
	for ; i < len(t); i++ {
		switch c := t[i]; {
		case '0' <= c && c <= '9':
			c = c - '0'
			if !parseFraction {
				if val > cutoff { // val*10 overflows
					return spec, b.overflow(t0)
				}
				val *= 10
				if val > val+uint64(c) { // val+c overflows
					return spec, b.overflow(t0)
				}
				val += uint64(c)
			} else {
				fraction *= 10
				fraction += uint64(c)
				fractionDiv *= 10
			}
		case c == '.':
			if parseFraction {
				return spec, b.syntaxError(t0)
			}
			parseFraction = true

		case c == ' ':
			// ignore whitespace

		default:
			if i == 0 {
				return spec, b.syntaxError(t0)
			}
			break ParseLoop
		}
	}

	unit = strings.TrimSpace(string(t[i:]))
	switch unit {
	case "Kb", "Mb", "Gb", "Tb", "Pb", "Eb":
		*b = 0
		return spec, errors.E("unmarshal bytesize spec", errors.K.Invalid, ErrBits, "from", t0)
	}
	unit = strings.ToLower(unit)

	switch unit {
	case "", "b", "byte":
		// no conversion needed

	case "k", "kb", "kilo", "kilobyte", "kilobytes":
		unitMul = uint64(KB)
	case "m", "mb", "mega", "megabyte", "megabytes":
		unitMul = uint64(MB)
	case "g", "gb", "giga", "gigabyte", "gigabytes":
		unitMul = uint64(GB)
	case "t", "tb", "tera", "terabyte", "terabytes":
		unitMul = uint64(TB)
	case "p", "pb", "peta", "petabyte", "petabytes":
		unitMul = uint64(PB)
	case "E", "EB", "e", "eb", "eB":
		unitMul = uint64(EB)
	default:
		idxPrecise := bytes.IndexRune(t, '(')
		if idxPrecise >= 0 && t[len(t)-1] == ')' {
			val = 0
			unit = ""
			i = 0
			t = t[idxPrecise+1 : len(t)-1]
			goto StartOver
		}
		return spec, b.syntaxError(t0)
	}

	// if fraction > unitMul {
	// 	return spec, errors.E("unmarshal bytesize spec", errors.K.Invalid, ErrInvalidFraction, "from", t0)
	// }

	if val > maxUint64/unitMul {
		return spec, b.overflow(t0)
	}
	val = val * unitMul

	if parseFraction {
		val += uint64(float64(unitMul) / float64(fractionDiv) * float64(fraction))
	}

	*b = Spec(val)
	return spec, nil
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

// FromString is an alias for Parse()
func FromString(s string) (Spec, error) {
	return Parse(s)
}

// Parse parses the given bytesize string into a bytesize spec.
func Parse(s string) (Spec, error) {
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
