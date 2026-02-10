package byteutil

import (
	"bufio"
	"io"
)

// NewRingBuffer creates a new ring buffer with the given capacity.
func NewRingBuffer(cap int) *RingBuffer {
	return &RingBuffer{
		cap: cap,
		buf: make([]byte, 2*cap), // double the size to avoid wrapping
	}
}

// RingBuffer is a simple ring buffer implementation with a fixed capacity. The capacity is the number of bytes that can
// be written to the buffer without consuming (reading) any bytes. Instead of using modulo logic, wrapping and
// complicated byte slice copies, it uses a double-sized buffer and copies the second half to the start of the buffer
// when the first half has been read. This avoids the need for modulo logic and simplifies the implementation.
type RingBuffer struct {
	cap int
	buf []byte
	off int
	len int
}

// Write writes the given bytes to the ring buffer. It returns the number of bytes written. It returns 0 if the buffer
// is full. The buffer is not resized and will remain full until bytes are read from it.
func (r *RingBuffer) Write(bts []byte) int {
	if len(bts) == 0 {
		return 0
	}
	free := r.Free()
	if free == 0 {
		// buffer is full
		return 0
	}
	if r.off >= r.cap {
		// first half of buffer has been read -> copy second half to start
		copy(r.buf, r.buf[r.off:])
		r.off = 0
	}
	n := min(len(bts), free)
	copy(r.buf[r.off+r.len:], bts[:n])
	r.len += n
	return n
}

// Read reads bytes from the ring buffer into the given byte slice and returns the number of bytes read. It returns
// 0 if the buffer is empty.
func (r *RingBuffer) Read(bts []byte) int {
	if r.len == 0 {
		return 0
	}
	n := min(len(bts), r.len)
	copy(bts, r.buf[r.off:r.off+n])
	r.off += n
	r.len -= n
	return n
}

// ReadByte reads a single byte from the ring buffer and returns it. It returns io.EOF if the buffer is empty.
func (r *RingBuffer) ReadByte() (byte, error) {
	if r.len == 0 {
		return 0, io.EOF
	}
	b := r.buf[r.off]
	r.off++
	r.len--
	return b, nil
}

// Unread rewinds the read offset by n bytes. It returns an io.ErrShortBuffer error if n is larger than the number of
// previously read bytes that are still available in the buffer (and haven't been overwritten by a Write call).
func (r *RingBuffer) Unread(n int) error {
	if n > r.off || n > r.cap-r.len {
		return io.ErrShortBuffer
	}
	r.off -= n
	r.len += n
	return nil
}

// Len returns the number of bytes currently available for reading.
func (r *RingBuffer) Len() int {
	return r.len
}

// Free returns the number of bytes that can be written to the ring buffer without overwriting any unread bytes.
func (r *RingBuffer) Free() int {
	return r.cap - r.len
}

// Peek returns the next n bytes without advancing the read offset in the ring. The bytes stop being valid at the next
// read call. If Peek returns fewer than n bytes, it also returns an error explaining why the read is short. The error
// is [ErrBufferFull] if n is larger than the ring's buffer size.
func (r *RingBuffer) Peek(n int) ([]byte, error) {
	if n < 0 {
		return nil, bufio.ErrNegativeCount
	}

	if r.len < n {
		return r.buf[r.off : r.off+r.len], bufio.ErrBufferFull
	}

	return r.buf[r.off : r.off+n], nil
}
