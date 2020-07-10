package httputil

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	mc "github.com/multiformats/go-multicodec"
	cbor "github.com/multiformats/go-multicodec/cbor"
	mcjson "github.com/multiformats/go-multicodec/json"
	mux "github.com/multiformats/go-multicodec/mux"

	"github.com/qluvio/content-fabric/constants"
	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/id"
)

const customHeaderPrefix = "X-Content-Fabric-"
const customHeaderMultiCodecPrefix = "X-Content-Fabric-Mc-"

var (
	// codec used for custom headers
	customHeaderMultiCodec *mux.Multicodec
	// A regular expression to match the error returned by net/http when the
	// configured number of redirects is exhausted. This error isn't typed
	// specifically so we resort to matching on the error string.
	redirectsErrorRe = regexp.MustCompile(`stopped after \d+ redirects\z`)
	// A regular expression to match the error returned by net/http when the
	// scheme specified in the URL is invalid. This error isn't typed
	// specifically so we resort to matching on the error string.
	schemeErrorRe = regexp.MustCompile(`unsupported protocol scheme`)
)

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

// ShouldRetry returns true if a failed HTTP request should be retried, false
// otherwise.
// Adapted from DefaultRetryPolicy in github.com/hashicorp/go-retryablehttp.
//
// Params
//  * ctx: the context used in the request (or nil)
//  * statusCode: the returned HTTP status if available (if err != nil)
//  * err:        the error returned by httpClient.Do() or Get() or similar
func ShouldRetry(ctx context.Context, statusCode int, err error) bool {
	// do not retry on context.Canceled or context.DeadlineExceeded
	if ctx != nil && ctx.Err() != nil {
		return false
	}

	if err != nil {
		err = errors.GetRootCause(err)
		if v, ok := err.(*url.Error); ok {
			// Don't retry if the error was due to too many redirects.
			if redirectsErrorRe.MatchString(v.Error()) {
				return false
			}

			// Don't retry if the error was due to an invalid protocol scheme.
			if schemeErrorRe.MatchString(v.Error()) {
				return false
			}

			// Don't retry if the error was due to TLS cert verification failure.
			if _, ok := v.Err.(x509.UnknownAuthorityError); ok {
				return false
			}
		}

		// The error is likely recoverable so retry.
		return true
	}

	// Check the response code. We retry on 500-range responses to allow
	// the server time to recover, as 500's are typically not permanent
	// errors and may relate to outages on the server side. This will catch
	// invalid response codes as well, like 0 and 999.
	if statusCode == 0 || (statusCode >= 500 && statusCode != 501) {
		return true
	}

	return false
}

func GetReqNodes(headers http.Header) (map[string]bool, error) {
	reqNodes := make(map[string]bool)

	reqNodesStr, err := GetCustomHeader(headers, "Requested-Nodes")
	if err != nil {
		return nil, errors.E("invalid requested_nodes", errors.K.Invalid, err)
	}

	for _, nid := range strings.Split(reqNodesStr, ",") {
		if nid == "" {
			continue
		}
		_, err := id.QNode.FromString(nid)
		if err != nil {
			return nil, errors.E("invalid requested_nodes", errors.K.Invalid, err, "requested_nodes", reqNodesStr)
		}
		reqNodes[nid] = true
	}

	return reqNodes, nil
}

func SetReqNodes(headers http.Header, reqNodes map[string]bool) {
	rnHeader := ""
	for nid, _ := range reqNodes {
		if rnHeader != "" {
			rnHeader += ","
		}
		rnHeader += nid
	}

	if rnHeader != "" {
		SetCustomHeader(headers, "Requested-Nodes", rnHeader)
	}
}

// GetCombinedHeader returns all values of all header names as a single string,
// individual header values separated by ",".
func GetCombinedHeader(header http.Header, query url.Values, headers ...string) string {
	res := make([]string, 0)
	for _, h := range headers {
		if v, ok := header[http.CanonicalHeaderKey(h)]; ok {
			for _, s := range v {
				res = append(res, s)
			}
		}
		if v, ok := query["header-"+
			strings.ReplaceAll(strings.ToLower(h), "-", "_")]; ok {
			for _, s := range v {
				res = append(res, s)
			}
		}
	}
	return strings.Join(res, ",")
}

// GetDecryptionMode extracts the decryption mode from an
// X-Content-Fabric-Decryption-Mode header or the header-x_decryption_mode query
// parameter. The decryption mode is treated as string to prevent import cycles
// with simple.
// PENDING(LUK): move simple.DecryptionMode to format/encryption
func GetDecryptionMode(header http.Header, query url.Values, def string) (string, error) {
	res := make([]string, 0)
	if v, ok := header[http.CanonicalHeaderKey(constants.DecryptModeHeader)]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	if v, ok := query["header-x_decryption_mode"]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	dec := strings.Join(res, ",")
	if dec == "" {
		return def, nil
	}
	decs := strings.Split(dec, ",")
	if len(decs) > 1 {
		for _, s := range decs[1:] {
			if s != decs[0] {
				return "", errors.E("GetDecryptionMode",
					errors.K.Invalid,
					"reason", "multiple values (inconsistent)",
					"values", dec)
			}
		}
	}

	return decs[0], nil
}

func GetRequestSetContentDisposition(r *http.Request) (string, error) {
	if r == nil {
		return "",
			errors.E("GetRequestSetContentDisposition", errors.K.Invalid, "reason", "request is nil)")
	}
	return GetSetContentDisposition(r.Header, r.URL.Query(), "")
}

// GetSetContentDisposition extracts the 'set-content-disposition' directive from
// X-Content-Fabric-Set-Content-Disposition header or the header-x_set_content_disposition query
// parameter.
func GetSetContentDisposition(header http.Header, query url.Values, def string) (string, error) {
	res := make([]string, 0)
	if v, ok := header[http.CanonicalHeaderKey(constants.SetContentDispositionHeader)]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	if v, ok := query["header-x_set_content_disposition"]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	dec := strings.Join(res, ",")
	if dec == "" {
		return def, nil
	}
	decs := strings.Split(dec, ",")
	if len(decs) > 1 {
		for _, s := range decs[1:] {
			if s != decs[0] {
				return "", errors.E("GetSetContentDisposition",
					errors.K.Invalid,
					"reason", "multiple values (inconsistent)",
					"values", dec)
			}
		}
	}

	return decs[0], nil
}
