package ioutil

import (
	"io"
	"io/ioutil"
	"sync"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/log"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

func NewReadSeekCloser(fnRead func(p []byte) (int, error), fnSeek func(offset int64, whence int) (int64, error), fnClose func() error) ReadSeekCloser {
	return &readSeekCloser{
		fnRead:  fnRead,
		fnSeek:  fnSeek,
		fnClose: fnClose,
	}
}

type readSeekCloser struct {
	fnRead  func(p []byte) (int, error)
	fnSeek  func(offset int64, whence int) (int64, error)
	fnClose func() error
}

func (r *readSeekCloser) Read(buf []byte) (int, error) {
	return r.fnRead(buf)
}

func (r *readSeekCloser) Seek(offset int64, whence int) (int64, error) {
	return r.fnSeek(offset, whence)
}

func (r *readSeekCloser) Close() error {
	return r.fnClose()
}

// TrackedCloser tracks calls to its Close() method and returns immediately if
// it was already called.
type TrackedCloser interface {
	Close() error
	IsClosed() bool
}

// NewTrackedCloser returns a closer that tracks calls to its Close() method
// and returns immediately if it was already called.
func NewTrackedCloser(target io.Closer) TrackedCloser {
	return &trackedCloser{target: target}
}

type trackedCloser struct {
	target io.Closer
	closed bool
	mutex  sync.Mutex
}

func (t *trackedCloser) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.target.Close()
}

func (t *trackedCloser) IsClosed() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.closed
}

// closeCloser lets us catch close errors when deferred
func CloseCloser(c io.Closer, l *log.Log) {
	if c != nil {
		err := c.Close()
		if err != nil && l != nil {
			l.Error("close error", "err", err)
		}
	}
}

// ReadAtAtMin reads from r into buf until it has read at least min bytes.
// It returns the number of bytes copied and an error if fewer bytes were read.
// The error is io.EOF only if no bytes were read.
// If an EOF happens after reading fewer than min bytes, ReadAtAtMin returns
// io.ErrUnexpectedEOF.
//
// If min is greater than the length of buf, ReadAtAtMin returns io.ErrShortBuffer.
// On return, n >= min if and only if err == nil.
func ReadAtAtMin(r io.ReaderAt, off int64, buf []byte, min int) (n int, err error) {
	if len(buf) < min {
		return 0, io.ErrShortBuffer
	}
	for n < min && err == nil {
		var nn int
		nn, err = r.ReadAt(buf[n:], off+int64(n))
		n += nn
	}
	if n >= min {
		err = nil
	} else if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

// ReadAtFull reads exactly len(buf) bytes from r into buf.
// It returns the number of bytes copied and an error if fewer bytes were read.
// The error is EOF only if no bytes were read.
// If an EOF happens after reading some but not all the bytes,
// ReadFull returns ErrUnexpectedEOF.
// On return, n == len(buf) if and only if err == nil.
func ReadAtFull(r io.ReaderAt, off int64, buf []byte) (n int, err error) {
	return ReadAtAtMin(r, off, buf, len(buf))
}

// Consume discards the rest of the reader and closes it.
// http.Client requires each response body to be fully read before closing.
func Consume(r io.ReadCloser) error {
	if r == nil {
		return nil
	}

	_, err := io.Copy(ioutil.Discard, r)
	if err == nil {
		err = r.Close()
	}

	return err
}

type nopWriteCloser struct {
	io.Writer
}

func (*nopWriteCloser) Close() error { return nil }

// NopWriteCloser returns a WriteCloser with a no-op Close method wrapping
// the provided Writer w.
func NopWriteCloser(w io.Writer) io.WriteCloser {
	return &nopWriteCloser{w}
}

type nopWriter struct{}

func (*nopWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (*nopWriter) Close() error { return nil }

// NopWriter returns a no-op writer that discards all bytes that are written.
func NopWriter() io.WriteCloser {
	return &nopWriter{}
}

// CopyBuffer copies all bytes from src to dst using the given copy buffer.
// Unlike io.CopyBuffer, which may use WriteTo() and ReadFrom() methods of the
// provided writer and reader, it will always use the copy buffer.
func CopyBuffer(dst io.Writer, src io.Reader, copyBuf []byte) (int, error) {
	total := 0
	for {
		read, err := src.Read(copyBuf)
		if read > 0 {
			written, err2 := dst.Write(copyBuf[:read])
			total += written
			if err2 != nil {
				return total, err2
			}
		} else if err == nil {
			return total, errors.E("invalid 0-byte read")
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}

// Skip reads and discards n bytes from the given reader. Returns the actual
// number of bytes read and/or an error.
func Skip(r io.Reader, n int64) (int64, error) {
	return io.CopyN(ioutil.Discard, r, n)
}

// Whence returns a string description of io.Seek* constants
func Whence(whence int) string {
	switch whence {
	case io.SeekStart:
		return "start"
	case io.SeekCurrent:
		return "current"
	case io.SeekEnd:
		return "end"
	default:
		return ""
	}
}
