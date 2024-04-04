package httputil

import (
	"fmt"
	"io"

	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
)

type ContentRange struct {
	off, len, totalSize    int64
	adaptedOff, adaptedLen int64
}

func (c *ContentRange) GetAdaptedOff() int64 {
	return c.adaptedOff
}

func (c *ContentRange) GetAdaptedEndOff() int64 {
	if c.adaptedLen == 0 {
		return c.adaptedOff
	}
	return c.adaptedOff + c.adaptedLen - 1
}

func (c *ContentRange) GetAdaptedLen() int64 {
	return c.adaptedLen
}

func (c *ContentRange) IsPartial() bool {
	return c.adaptedOff > 0 || c.adaptedLen != c.totalSize
}

func (c *ContentRange) AsHeader() string {
	if c.adaptedOff < 0 {
		return fmt.Sprintf("bytes */%d", c.totalSize)
	}
	return fmt.Sprintf("bytes %d-%d/%d", c.adaptedOff, c.GetAdaptedEndOff(), c.totalSize)
}

func (c *ContentRange) TotalSize() int64 {
	return c.totalSize
}

// --------------------------------------------------------------------------------------------------------------------

// AdaptRange adapts offset and length received in HTTP header (or query) as described in RFC 7233, section 4
// (https://tools.ietf.org/html/rfc7233#section-4) given the actual total size of the content.
//
// A content range object is always returned, even in case of error, in which case the AsHeader() method will return
// "*/TotalBytes" as needed in the HTTP response.
func AdaptRange(off, len, totalLen int64) (*ContentRange, error) {
	var err error = nil
	realOff := off
	realLen := len
	if off < 0 && len < 0 {
		err = errors.E("adapt-byte-range", errors.K.Invalid, "reason", "negative offset and length")
	} else if off < 0 {
		realOff = totalLen - len
	} else if len < 0 {
		realLen = totalLen - off
		if realLen < 0 {
			err = errors.E("adapt-byte-range", errors.K.Invalid, "reason", "offset larger than total length",
				"offset", off, "length", len, "total_length", totalLen)
		}
	}
	if err == nil {
		if realOff+realLen > totalLen {
			realLen = totalLen - realOff
		}
		if realOff < 0 || (realOff > totalLen && realOff > 0) {
			err = errors.E("adapt-byte-range", errors.K.Invalid, "reason", "invalid offset result",
				"offset", off, "length", len, "total_length", totalLen)
		}
	}
	if err != nil {
		realOff = -1
		realLen = -1
	}

	return &ContentRange{
		off:        off,
		len:        len,
		totalSize:  totalLen,
		adaptedOff: realOff,
		adaptedLen: realLen,
	}, err
}

// FullRange returns a content range with the given total size and an adapted
// offset of 0 and an adapted len equal to the total size.
func FullRange(totalSize int64) *ContentRange {
	return &ContentRange{
		off:        0,
		len:        -1,
		totalSize:  totalSize,
		adaptedOff: 0,
		adaptedLen: totalSize,
	}
}

// ToRangeReader adapts off and siz to the given totalSize, positions the reader
// at the adapted offset and returns the adapted ContentRange.
func ToRangeReader(reader ioutil.ReadSeekCloser, off, siz, totalSize int64, err error) (ioutil.ReadSeekCloser, *ContentRange, error) {
	if err != nil {
		return nil, nil, err
	}
	contentRange, err := AdaptRange(off, siz, totalSize)
	if err != nil {
		_ = reader.Close()
		return nil, contentRange, err
	}
	_, err = reader.Seek(contentRange.GetAdaptedOff(), io.SeekStart)
	if err != nil {
		_ = reader.Close()
		return nil, contentRange, err
	}
	return reader, contentRange, nil
}
