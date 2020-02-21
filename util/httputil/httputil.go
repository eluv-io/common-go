package httputil

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	mc "github.com/multiformats/go-multicodec"
	cbor "github.com/multiformats/go-multicodec/cbor"
	mcjson "github.com/multiformats/go-multicodec/json"
	mux "github.com/multiformats/go-multicodec/mux"

	"github.com/qluvio/content-fabric/errors"
)

const customHeaderPrefix = "X-Content-Fabric-"
const customHeaderMultiCodecPrefix = "X-Content-Fabric-Mc-"

var customHeaderMultiCodec *mux.Multicodec

func init() {
	customHeaderMultiCodec = mux.MuxMulticodec([]mc.Multicodec{
		cbor.Multicodec(),
		mcjson.Multicodec(false),
	}, mux.SelectFirst)
	customHeaderMultiCodec.Wrap = false
}

// ParseByteRange parses a (single) byte-range in the form "start-end" as
// defined for HTTP Range Request (https://tools.ietf.org/html/rfc7233)
// returns it as offset and size.
//
// Returns offset = -1 if no first byte position is specified in the range.
// Returns size = -1 if no last byte position is specified in the range.
// Returns [0, -1] if no range is specified (empty string).
func ParseByteRange(r string) (offset, size int64, err error) {
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

// ExtractCustomHeaders extracts custom HTTP headers prefixed with
// 'X-Content-Fabric-' and builds up a map with the remaining header name as
// key and the (first) header value. If multiple headers with the same name
// exist, only the first one is parsed and the rest is ignored.
// Values are first decoded with Base64 and subsequently with a multi-codec
// (which currently supports binary, cbor and json)
func ExtractCustomHeaders(headers http.Header) (m map[string]interface{}, err error) {
	m = make(map[string]interface{})
	for key, val := range headers {
		if strings.HasPrefix(key, customHeaderPrefix) {
			// only use the first value
			dec, err := base64.StdEncoding.DecodeString(val[0])
			if err != nil {
				return nil, errors.E("invalid base64 value in custom header", err, "header_name", key, "header_value", val[0])
			}
			var parsed interface{}
			var newKey string
			if strings.HasPrefix(key, customHeaderMultiCodecPrefix) {
				// it's encoded with a multicodec
				err = customHeaderMultiCodec.Decoder(bytes.NewReader(dec)).Decode(&parsed)
				if err != nil {
					return nil, errors.E("invalid value in custom header", err, "header_name", key, "header_value", val[0])
				}
				newKey = key[len(customHeaderMultiCodecPrefix):]
			} else {
				// the value is a string
				parsed = string(dec)
				newKey = key[len(customHeaderPrefix):]
			}
			m[strings.ToLower(newKey)] = parsed
		}
	}
	return m, nil
}

func GetCustomHeader(headers http.Header, key string) (string, error) {
	val, err := base64.StdEncoding.DecodeString(headers.Get(customHeaderPrefix + key))
	if err != nil {
		return "", errors.E("invalid base64 value in custom header", errors.K.Invalid, err, "header_name", key, "header_value", val)
	}

	return string(val), nil
}

func GetPlainCustomHeader(headers http.Header, key string) (string, error) {
	return headers.Get(customHeaderPrefix + key), nil
}

// SetCustomHeader sets the given key-value pairs from the provided map as
// custom headers. If the value is a string, SetCustomerHeader() is called,
// otherwise SetCustomCBORHeader() (any returned errors are ignored).
// See these functions for details.
func SetCustomHeaders(headers http.Header, m map[string]interface{}) {
	for key, val := range m {
		if s, ok := val.(string); ok {
			SetCustomHeader(headers, key, s)
		} else {
			SetCustomCBORHeader(headers, key, val)
		}
	}
}

// SetCustomHeader sets the given key value pair as a custom header, prefixing
// the key with 'X-Content-Fabric-' and encoding the string in base 64.
func SetCustomHeader(headers http.Header, key string, val string) {
	headers.Set(customHeaderPrefix+key, base64.StdEncoding.EncodeToString([]byte(val)))
}

// SetCustomCBORHeader sets the given key value pair as a custom header,
// prefixing the key with 'X-Content-Fabric-' and encoding the value in CBOR and
// then in base 64.
func SetCustomCBORHeader(headers http.Header, key string, val interface{}) error {
	var buf bytes.Buffer
	err := customHeaderMultiCodec.Encoder(&buf).Encode(val)
	if err != nil {
		return err
	}
	headers.Set(customHeaderMultiCodecPrefix+key, base64.StdEncoding.EncodeToString(buf.Bytes()))
	return nil
}

// Normalizes the given URL string into the format "scheme://host:port[/path]"
func NormalizeURL(rawURL string, defaultScheme string, defaultPort string) (string, error) {
	rawurl := rawURL
	if !strings.Contains(rawurl, "://") {
		rawurl = defaultScheme + "://" + rawurl
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", errors.E("invalid url", errors.K.Invalid, err, "url", rawURL)
	}
	if !strings.Contains(u.Host, ":") {
		u.Host = u.Host + ":" + defaultPort
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func SetRedirectedQuery(u *url.URL, v bool) {
	q := u.Query()
	s := "false"
	if v {
		s = "true"
	}
	q.Set("redirected", s)
	u.RawQuery = q.Encode()
}

func RemoveRedirectedQuery(u *url.URL) {
	q := u.Query()
	q.Del("redirected")
	u.RawQuery = q.Encode()
}
