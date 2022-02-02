package preamble

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"

	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/common-go/util/stringutil"
	"github.com/eluv-io/errors-go"
	"github.com/multiformats/go-multicodec"
)

// Write writes the given preamble to the specified writer
// A preamble is stored at the beginning of a part in the following format:
//     [varint][header][data]
// where varint is the length of the header and data, header is a multiformat header describing the
// format of the data, and data is the user-specified preamble data.
func Write(w io.Writer, preambleData []byte, preambleFormat ...string) (int64, error) {
	preambleFmt := ""
	if len(preambleFormat) > 0 {
		preambleFmt = strings.TrimSpace(preambleFormat[0])
	}
	if preambleFmt == "" {
		preambleFmt = "/raw"
	} else if !strings.HasPrefix(preambleFmt, "/") {
		preambleFmt = "/" + preambleFmt
	}
	if !isFormat(preambleFmt) {
		return 0, errors.E("preamble write", errors.K.Invalid, "reason", "invalid preamble format", "format", preambleFmt)
	}
	header := multicodec.Header([]byte(preambleFmt))
	sz := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(sz, uint64(len(header)+len(preambleData)))
	mr := io.MultiReader(bytes.NewReader(sz[:n]), bytes.NewReader(header), bytes.NewReader(preambleData))
	preambleSize, err := io.Copy(w, mr)
	if err != nil {
		return 0, errors.E("preamble write", errors.K.IO, err)
	}
	return preambleSize, nil
}

// Read reads the preamble (if present) from the specified reader
// This implementation will restore the read offset of the reader if noSeek is false. If sizeLimit
// is specified, it will error if the stored varint size (not preambleSize) is greater than sizeLimit.
// Note: it is technically possible this implementation returns a gibberish "preamble" without error
// when a preamble was not originally written, but this would be highly (?) unlikely. As such, it
// is preferable that Read() is only called on data that is guaranteed to have a preamble.
func Read(r io.ReadSeeker, noSeek bool, sizeLimit ...int64) (preambleData []byte, preambleFormat string, preambleSize int64, err error) {
	origOff, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		err = errors.E("preamble read", err)
		return
	}
	if !noSeek {
		// Return to original read offset after finishing
		defer func() {
			_, err2 := r.Seek(origOff, io.SeekStart)
			if err == nil && err2 != nil {
				err = errors.E("preamble read", err2)
			}
		}()
		// Seek to beginning and read preamble
		_, err = r.Seek(0, io.SeekStart)
		if err != nil {
			err = errors.E("preamble read", err)
			return
		}
	} else {
		// Read offset should be at beginning
		if origOff != 0 {
			err = errors.E("preamble read", errors.K.Invalid, "reason", "read offset not zero")
			return
		}
		// Return to original read offset if error
		defer func() {
			if err != nil {
				_, _ = r.Seek(origOff, io.SeekStart)
			}
		}()
	}
	sl := int64(math.MaxInt64)
	if len(sizeLimit) > 0 {
		sl = sizeLimit[0]
	}
	sz, err := binary.ReadUvarint(ioutil.NewByteReader(r))
	if err != nil || sz == 0 || sz > uint64(sl) {
		err = errors.E("preamble read", errors.K.NotExist, err, "reason", "preamble size not found", "size", sz, "size_limit", sl)
		return
	}
	data := make([]byte, int(sz))
	_, err = io.ReadFull(r, data)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		err = errors.E("preamble read", errors.K.NotExist, err, "reason", "preamble data not found")
		return
	} else if err != nil {
		err = errors.E("preamble read", errors.K.IO, err)
		return
	}
	buf := bytes.NewBuffer(data)
	header, err := multicodec.ReadHeader(buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF || err == multicodec.ErrVarints || err == multicodec.ErrHeaderInvalid {
		err = errors.E("preamble read", errors.K.NotExist, err, "reason", "preamble header not found")
		return
	} else if err != nil {
		err = errors.E("preamble read", errors.K.IO, err)
		return
	}
	preambleFormat = string(multicodec.HeaderPath(header))
	if !isFormat(preambleFormat) {
		err = errors.E("preamble read", errors.K.NotExist, "reason", "preamble header not found")
		return
	}
	preambleSize, err = r.Seek(0, io.SeekCurrent)
	if err != nil {
		err = errors.E("preamble read", err)
		return
	}
	preambleData = buf.Bytes() // Remaining bytes
	return
}

// Seek sets the seek offset of the specified seeker
// If a preamble is present, the requested seek offset will be adjusted to avoid the preamble.
// currOffset, offset, whence, and the return value are relative to the end of the preamble, i.e.
// excluding the preamble if present.
func Seek(s io.Seeker, preambleSize int64, dataSize int64, currOffset int64, offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		offset += currOffset
		whence = io.SeekStart
	} else if whence == io.SeekEnd && dataSize >= 0 {
		offset += dataSize
		whence = io.SeekStart
	}
	if whence == io.SeekStart {
		if offset == currOffset {
			// Nothing to do
			return offset, nil
		} else if offset < 0 || (dataSize >= 0 && offset > dataSize) {
			return 0, errors.E("preamble seek", errors.K.Invalid, "reason", "out of bounds")
		}
		// Adjust offset to avoid the preamble
		offset += preambleSize
	}
	// Record offset just in case need to revert
	revertOff, err := s.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, errors.E("preamble seek", err)
	}
	off, err := s.Seek(offset, whence)
	if err != nil {
		return 0, errors.E("preamble seek", err)
	} else if off < preambleSize {
		// New offset is in the preamble, which is not allowed; revert offset
		_, err = s.Seek(revertOff, io.SeekStart)
		if err != nil {
			return 0, errors.E("preamble seek", err)
		}
		return 0, errors.E("preamble seek", errors.K.Invalid, "reason", "out of bounds")
	}
	return off - preambleSize, nil
}

///////////////////////////////////////////////////////////////////////////////

// Sizer provides an io.Writer that calculates the size of preamble data written to it
// It is expected that Sizer.Size() is only called after the full preamble data has been written to
// the Sizer. In the typical case, since the preamble size is not known beforehand (hence the use
// of Sizer), the full part data is written to the Sizer. Behavior is undefined if data without a
// preamble is written to the Sizer.
// Note: This implementation of Sizer only records the first MaxVarintLen64 bytes, from which the
// preamble size can be determined.
type Sizer struct {
	buf []byte
}

var _ io.Writer = (*Sizer)(nil)

func NewSizer() *Sizer {
	return &Sizer{buf: make([]byte, 0, binary.MaxVarintLen64)}
}

func (s *Sizer) Write(p []byte) (int, error) {
	// Fill buf and discard the rest
	n := cap(s.buf) - len(s.buf)
	if n > 0 {
		if len(p) < n {
			n = len(p)
		}
		s.buf = append(s.buf, p[:n]...)
	}
	return len(p), nil
}

func (s *Sizer) Size() (int64, error) {
	// Preamble size is the sum of the varint length and the varint itself
	x, n := binary.Uvarint(s.buf)
	if n <= 0 {
		return 0, errors.E("preamble size", errors.K.Invalid, "reason", "invalid uvarint", "n", n)
	}
	return int64(x) + int64(n), nil
}

///////////////////////////////////////////////////////////////////////////////

const formatSymbols = "abcdefghijklmnopqrstuvwxyz1234567890-_"

func isFormat(s string) bool {
	return len(s) > 1 && strings.HasPrefix(s, "/") &&
		stringutil.MatchRunes(s[1:], func(r rune) bool { return strings.ContainsRune(formatSymbols, r) })
}
