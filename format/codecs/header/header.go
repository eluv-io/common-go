package header

import (
	"bytes"
	"errors"
	"io"
)

var (
	ErrHeaderInvalid = errors.New("MultiCodec header invalid")
	ErrMismatch      = errors.New("MultiCodec did not match")
	ErrVarints       = errors.New("MultiCodec varints not yet implemented")
)

// Header is the header used by MultiCodecs to encode the codec type used to create an encoding. It's format is
//   - a single byte encoding the length of the rest of the header
//   - the "path" (an identifier) of the codec. By convention it starts with a slash and contains the codec's name, e.g.
//     "/json" or "/cborV2"
//   - a terminating newline to make the header more readable when looking at the encoded data
//
// Create it with New(path)
type Header []byte

// Path returns the path of the MultiCodec header.
func (h Header) Path() string {
	return Path(h)
}

// String is an alias of Path.
func (h Header) String() string {
	return h.Path()
}

// New returns a MultiCodec header built from the given path.
func New(path string) Header {
	b, err := NewNoPanic(path)
	if err != nil {
		panic(err.Error)
	}
	return b
}

// NewNoPanic works like New but it returns error instead of calling panic
func NewNoPanic(path string) (Header, error) {
	bts := []byte(path)
	l := len(bts) + 1 // + \n
	if l >= 127 {
		return nil, ErrVarints
	}

	buf := make([]byte, l+1)
	buf[0] = byte(l)
	copy(buf[1:], bts)
	buf[l] = '\n'
	return buf, nil
}

// Path returns the MultiCodec path from header
func Path(hdr Header) string {
	hdr = hdr[1:]
	if hdr[len(hdr)-1] == '\n' {
		hdr = hdr[:len(hdr)-1]
	}
	return string(hdr)
}

// WriteHeader writes a MultiCodec header to a writer.
// It uses the given path.
func WriteHeader(w io.Writer, hdr Header) error {
	_, err := w.Write(hdr)
	return err
}

// ReadHeader reads a MultiCodec header from a reader.
// Returns the header found, or an error if the header
// mismatched.
func ReadHeader(r io.Reader) (hdr Header, err error) {
	lbuf := make([]byte, 1)
	if _, err := r.Read(lbuf); err != nil {
		return nil, err
	}

	l := int(lbuf[0])
	if l > 127 {
		return nil, ErrVarints
	}

	buf := make([]byte, l+1)
	buf[0] = lbuf[0]
	if _, err := io.ReadFull(r, buf[1:]); err != nil {
		return nil, err
	}
	if buf[l] != '\n' {
		return nil, ErrHeaderInvalid
	}
	return buf, nil
}

// ConsumeHeader reads a MultiCodec header from a reader,
// verifying it matches given header. If it does not, it returns
// ErrProtocolMismatch
func ConsumeHeader(r io.Reader, header Header) (err error) {
	actual := make([]byte, len(header))
	if _, err := io.ReadFull(r, actual); err != nil {
		return err
	}

	if !bytes.Equal(header, actual) {
		return ErrMismatch
	}
	return nil
}

// WrapHeaderReader returns a reader that first reads the given header, and then the given reader, using io.MultiReader.
// It is useful if the header has been read through, but still needed to pass to a decoder.
func WrapHeaderReader(hdr Header, r io.Reader) io.Reader {
	return io.MultiReader(bytes.NewReader(hdr), r)
}
