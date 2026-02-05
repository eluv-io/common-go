package httputil

import (
	"fmt"
	"io"

	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
)

const KindRangeNotSatisfiable = errors.Kind("range not satisfiable")

type ContentRange struct {
	Off, Len, TotalLen     int64
	AdaptedOff, AdaptedLen int64
}

func (c *ContentRange) GetAdaptedOff() int64 {
	return c.AdaptedOff
}

func (c *ContentRange) GetAdaptedEndOff() int64 {
	if c.AdaptedLen == 0 {
		return c.AdaptedOff
	} else if c.AdaptedLen < 0 {
		return c.AdaptedLen
	}
	return c.AdaptedOff + c.AdaptedLen - 1
}

func (c *ContentRange) GetAdaptedLen() int64 {
	return c.AdaptedLen
}

func (c *ContentRange) IsPartial() bool {
	return c.AdaptedOff > 0 || (c.TotalLen >= 0 && c.AdaptedLen != c.TotalLen)
}

func (c *ContentRange) AsHeader() string {
	var brange, blength string
	if c.AdaptedOff < 0 {
		brange = "*"
	} else if c.AdaptedLen < 0 {
		brange = fmt.Sprintf("%d-", c.AdaptedOff)
	} else {
		brange = fmt.Sprintf("%d-%d", c.AdaptedOff, c.GetAdaptedEndOff())
	}
	if c.TotalLen >= 0 {
		blength = fmt.Sprintf("/%d", c.TotalLen)
	}
	return brange + blength
}

func (c *ContentRange) TotalSize() int64 {
	return c.TotalLen
}

// --------------------------------------------------------------------------------------------------------------------

// AdaptRange adapts offset and length of a [Byte Range] received in an HTTP Range header (or query) according to the
// instructions in [RFC 7233, section 4] given the actual total size of the content.
//
// See httputil.ParseByteRange() for details on offset and len.
//
// Returns an error if offset and/or len are invalid. A content range object is always returned, even in case of error,
// in which case the AsHeader() method will return "*/TotalBytes" as needed in the HTTP response.
//
// [Byte Range]: https://tools.ietf.org/html/rfc7233#section-2.1
// [RFC 7233, section 4]: https://tools.ietf.org/html/rfc7233#section-4
func AdaptRange(off, len, totalLen int64) (*ContentRange, error) {
	e := errors.Template("adapt-byte-range", KindRangeNotSatisfiable, "offset", off, "length", len, "total_length", totalLen)
	var err error = nil
	realOff := off
	if realOff < -1 {
		realOff = -1
	}
	realLen := len
	if realLen < -1 {
		realLen = -1
	}
	if off < 0 && len < 0 {
		err = e("reason", "negative offset and length")
	} else if off < 0 && totalLen < 0 {
		err = e("reason", "negative offset and total_length")
	} else if off < 0 {
		realOff = totalLen - len
		if realOff < 0 {
			err = e("reason", "length larger than total_length")
		}
	} else if len < 0 && totalLen >= 0 {
		realLen = totalLen - off
		if realLen < 0 {
			err = e("reason", "offset larger than total_length")
		}
	}
	if err == nil && totalLen >= 0 {
		if realOff > totalLen {
			err = e("reason", "offset larger than total_length")
		} else if realOff+realLen > totalLen {
			realLen = totalLen - realOff
		}
	}
	if err != nil {
		realOff = -1
		realLen = -1
	}

	return &ContentRange{
		Off:        off,
		Len:        len,
		TotalLen:   totalLen,
		AdaptedOff: realOff,
		AdaptedLen: realLen,
	}, err
}

// FullRange returns a content range with the given total size and an adapted
// offset of 0 and an adapted len equal to the total size.
func FullRange(totalSize int64) *ContentRange {
	return &ContentRange{
		Off:        0,
		Len:        -1,
		TotalLen:   totalSize,
		AdaptedOff: 0,
		AdaptedLen: totalSize,
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
