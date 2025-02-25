package hash

import (
	"crypto/sha256"
	"hash"
	"io"

	"github.com/eluv-io/common-go/format/preamble"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// Digest encapsulates a message digest function which produces a specific type of Hash
type Digest struct {
	hash.Hash
	preamble  *preamble.Sizer
	format    Format
	storageId uint
	size      uint64
	psize     uint64
	err       error
}

// make sure Digest implements the Hash interface
var _ hash.Hash = (*Digest)(nil)

// NewDigest creates a new digest. Does not support live part hashes
func NewDigest(h hash.Hash, t Type) *Digest {
	return &Digest{Hash: h, preamble: preamble.NewSizer(), format: t.Format}
}

// NewBuilder creates a new digest, which is essentially a builder for hashes.
func NewBuilder() *Digest {
	return &Digest{Hash: sha256.New(), preamble: preamble.NewSizer(), format: Unencrypted}
}

func (d *Digest) WithPreamble(preambleSize int64) *Digest {
	if preambleSize > 0 {
		d.psize = uint64(preambleSize)
	} else {
		// Calculate preamble size
		psize, err := d.preamble.Size()
		if err != nil {
			d.err = errors.Append(d.err, err)
		} else {
			d.psize = uint64(psize)
		}
	}
	return d
}

func (d *Digest) WithStorageId(storageId uint) *Digest {
	d.storageId = storageId
	return d
}

func (d *Digest) WithFormat(format Format) *Digest {
	d.format = format
	return d
}

func (d *Digest) Write(p []byte) (int, error) {
	n, err := d.Hash.Write(p)
	if err == nil {
		n2, err2 := d.preamble.Write(p[:n])
		if err2 != nil || n2 != n {
			d.err = errors.Append(d.err, errors.NoTrace("digest.Write", errors.K.IO, err, "n", n, "n2", n2))
		}
	}
	d.size += uint64(n)
	return n, err
}

// AsHash finalizes the digest calculation using all the bytes that were previously written to this digest object and
// return the result as a Hash.
//
// Deprecated: use BuildHash()
func (d *Digest) AsHash() *Hash {
	h, err := d.BuildHash()
	if err != nil {
		// errors must be caught by unit tests!
		log.Fatal("invalid hash", "error", err)
	}
	return h
}

// BuildHash finalizes the digest calculation using all the bytes that were previously written to this digest object and
// returns the resulting part Hash.
func (d *Digest) BuildHash() (*Hash, error) {
	b, err := d.Finalize()
	if err != nil {
		return nil, errors.E("digest.BuildHash", errors.K.Invalid.Default(), err)
	}

	return newPart(d.format, b, d.size, d.psize, d.storageId)
}

// Finalize finalizes the digest calculation using all the bytes that were previously written to this digest object and
// returns the resulting digest.
func (d *Digest) Finalize() ([]byte, error) {
	if d.err != nil {
		return nil, errors.E("digest.Finalize", errors.K.Invalid, d.err)
	}

	return d.Hash.Sum(nil), nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func CalcHash(reader io.ReadSeeker, size ...int64) (*Hash, error) {
	digest := NewDigest(sha256.New(), Type{QPart, Unencrypted})

	// Check for preamble
	var preambleSize int64
	var err error
	if len(size) > 0 {
		_, _, preambleSize, err = preamble.Read(reader, false, size[0])
	} else {
		_, _, preambleSize, err = preamble.Read(reader, false)
	}
	if errors.IsNotExist(err) {
		preambleSize = 0
	} else if err != nil {
		return nil, err
	}

	buf := make([]byte, 128*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			_, err = digest.Write(buf[:n])
			if err != nil {
				return nil, err
			}
		}
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return nil, err
		}
	}

	if preambleSize > 0 {
		digest = digest.WithPreamble(preambleSize)
	}

	return digest.BuildHash()
}
