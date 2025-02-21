package hash

import (
	"crypto/sha256"
	"hash"
	"io"

	ei "github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/preamble"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// Digest encapsulates a message digest function which produces a specific type of Hash
type Digest struct {
	hash.Hash
	preamble  *preamble.Sizer
	htype     Type
	id        ei.ID
	size      int64
	psize     int64
	storageId uint
}

// make sure Digest implements the Hash interface
var _ hash.Hash = (*Digest)(nil)

// NewDigest creates a new digest. Does not support live part hashes
func NewDigest(h hash.Hash, t Type) *Digest {
	return &Digest{Hash: h, preamble: preamble.NewSizer(), htype: t}
}

func (d *Digest) WithPreamble(preambleSize int64) *Digest {
	if d.htype.Code == QPart {
		if preambleSize > 0 {
			d.psize = preambleSize
		} else {
			// Calculate preamble size
			var err error
			d.psize, err = d.preamble.Size()
			if err != nil {
				// Should not happen
				log.Warn("invalid hash", "error", err)
			}
		}
	} else {
		// Should not happen
		log.Warn("invalid hash", "error", "preamble not applicable", "code", d.htype.Code)
	}
	return d
}

func (d *Digest) WithID(i ei.ID) *Digest {
	if d.htype.Code == Q {
		d.id = i
	}
	return d
}

func (d *Digest) WithStorageId(sc uint) *Digest {
	d.storageId = sc
	return d
}

func (d *Digest) Write(p []byte) (int, error) {
	n, err := d.Hash.Write(p)
	if err == nil && d.htype.Code == QPart {
		n2, err2 := d.preamble.Write(p)
		if err2 != nil || n2 != n {
			// Should not happen
			log.Warn("invalid hash", "error", err, "n", n, "n2", n2)
		}
	}
	d.size += int64(n)
	return n, err
}

// AsHash finalizes the digest calculation using all the bytes that were previously written to this digest object and
// return the result as a Hash.
func (d *Digest) AsHash() *Hash {
	b := d.Hash.Sum(nil)
	var h *Hash
	var err error
	if d.htype.Code == Q {
		h, err = NewObject(d.htype, b, d.size, d.id)
	} else {
		h, err = NewPart(d.htype, b, d.size, d.psize)
	}
	if err != nil {
		// errors must be caught by unit tests!
		log.Fatal("invalid hash", "error", err)
	}
	return h
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

	return digest.AsHash(), nil
}
