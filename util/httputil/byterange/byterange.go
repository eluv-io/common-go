package byterange

import (
	"strconv"
	"strings"

	"github.com/eluv-io/errors-go"
)

// Parse parses a (single) [Byte Range] in the form "start-end" as
// defined for HTTP Range Request returns it as offset and size.
//
// Returns offset = -1 if no first byte position is specified in the range.
// Returns size = -1 if no last byte position is specified in the range.
// Returns [0, -1] if no range is specified (empty string).
//
// [Byte Range]: https://tools.ietf.org/html/rfc7233#section-2.1
func Parse(r string) (offset, size int64, err error) {
	if r == "" {
		return 0, -1, nil
	}

	var first, last int64 = -1, -1
	ends := strings.Split(r, "-")
	if len(ends) != 2 {
		return 0, 0, errors.E("byte range", errors.K.Invalid, "bytes", r)
	}

	if ends[0] != "" {
		first, err = strconv.ParseInt(ends[0], 10, 64)
		if err != nil {
			return 0, 0, errors.E("byte range", errors.K.Invalid, err, "bytes", r)
		}
	}

	if ends[1] != "" {
		last, err = strconv.ParseInt(ends[1], 10, 64)
		if err != nil {
			return 0, 0, errors.E("byte range", errors.K.Invalid, err, "bytes", r)
		} else if last < first {
			return 0, 0, errors.E("byte range", errors.K.Invalid, errors.Str("first > last"), "bytes", r)
		}
	}

	switch {
	case first == -1 && last == -1:
		return 0, -1, nil
	case first == -1:
		return first, last, nil
	case last == -1:
		return first, last, nil
	default:
		return first, last - first + 1, nil
	}
}
