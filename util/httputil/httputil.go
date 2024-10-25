package httputil

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin/binding"
	mc "github.com/multiformats/go-multicodec"
	cbor "github.com/multiformats/go-multicodec/cbor"
	mcjson "github.com/multiformats/go-multicodec/json"
	mux "github.com/multiformats/go-multicodec/mux"
	"golang.org/x/text/encoding/charmap"

	"github.com/eluv-io/common-go/format/id"
	eioutil "github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
)

const (
	DecryptModeHeader            = "X-Content-Fabric-Decryption-Mode"         // HTTP header for specifying mode for decryption
	SetContentDispositionHeader  = "X-Content-Fabric-Set-Content-Disposition" // HTTP header for directive to return a Content-Disposition header
	customHeaderPrefix           = "X-Content-Fabric-"
	customHeaderMultiCodecPrefix = "X-Content-Fabric-Mc-"
	customQueryPrefix            = "header-x_"
)

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

// GetBytesRange extracts "byte range" as verbatim string from an
// HTTP Range Request (https://tools.ietf.org/html/rfc7233)
//
// The spec defines range requests with a special header "Range: bytes=x-y". In
// addition, this function also attempts to extract the range from a URL query:
// "...?bytes=x-y".
//
// Only "byte" ranges are supported (the spec allows for additional types).
// The returned string is whatever follows the "bytes=" prefix.
//
// Returns the URL query definition if both query and header are specified.
// Returns an empty string "" if no byte range is specified.
// Returns an error in case of an invalid "Range" header.
func GetBytesRange(request *http.Request) (string, error) {
	query := request.URL.Query()["bytes"]
	if len(query) > 0 {
		return query[0], nil
	}

	header := request.Header.Get("Range")
	if len(header) > 0 {
		if strings.HasPrefix(header, "bytes=") {
			return strings.TrimPrefix(header, "bytes="), nil
		} else {
			return "", errors.E("byte range", errors.K.Invalid, "range_header", header)
		}
	}

	return "", nil
}

// ParseByteRange parses a (single) [Byte Range] in the form "start-end" as
// defined for HTTP Range Request returns it as offset and size.
//
// Returns offset = -1 if no first byte position is specified in the range.
// Returns size = -1 if no last byte position is specified in the range.
// Returns [0, -1] if no range is specified (empty string).
//
// [Byte Range]: https://tools.ietf.org/html/rfc7233#section-2.1
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

func GetCustomQuery(values url.Values, key string) (string, error) {
	val, err := base64.StdEncoding.DecodeString(values.Get(customQueryPrefix + key))
	if err != nil {
		return "", errors.E("invalid base64 value in custom query", errors.K.Invalid, err, "query_key", key, "query_value", val)
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

func SetRefererQuery(u *url.URL, r string) {
	q := u.Query()
	q.Set("header-referer", r)
	u.RawQuery = q.Encode()
}

// ShouldRetry returns true if a failed HTTP request should be retried, false
// otherwise.
// Adapted from DefaultRetryPolicy in github.com/hashicorp/go-retryablehttp.
//
// Params
//   - ctx: the context used in the request (or nil)
//   - statusCode: the returned HTTP status if available (if err != nil)
//   - err:        the error returned by httpClient.Do() or Get() or similar
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

// Unmarshal unmarshals the body of the HTTP request, using the correct JSON or
// XML unmarshaler depending on the "Content-Type" header of the request (JSON
// per default). The data is unmarshaled into generic maps and slices.
func Unmarshal(reqBody io.ReadCloser, reqHeader http.Header) (interface{}, error) {
	if reqBody == nil {
		// this should not happen with a real server request, but will in unit
		// tests...
		return nil, nil
	}
	body, err := ioutil.ReadAll(reqBody)
	if err != nil {
		return nil, errors.E("read request body", errors.K.IO, err)
	}
	if len(body) == 0 {
		return nil, nil
	}
	ct := reqHeader.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, binding.MIMEXML):
		return defaultUnmarshaler.XML(body)
	case strings.HasPrefix(ct, binding.MIMEJSON) || ct == "":
		return defaultUnmarshaler.JSON(body)
	default:
		return nil, errors.E("unmarshal request", errors.K.Invalid, "reason", "unacceptable content type", "content_type", ct)
	}
}

// UnmarshalTo unmarshals the body of the HTTP request into the given target go
// structure, using the correct JSON or XML unmarshaler depending on the
// "Content-Type" header of the request (JSON per default).
//
// Returns without error if the body is empty and "required" is false.
func UnmarshalTo(reqBody io.Reader, reqHeader http.Header, target interface{}, required bool) error {
	var body []byte
	var err error
	if reqBody != nil {
		body, err = ioutil.ReadAll(reqBody)
		if err != nil {
			return errors.E("read request body", errors.K.IO, err)
		}
	}
	if len(body) == 0 && !required {
		return nil
	}
	ct := reqHeader.Get("Content-Type")
	switch {
	case strings.HasPrefix(ct, "application/xml"):
		// Convert XML to JSON
		s, err := defaultUnmarshaler.XML(body)
		if err == nil {
			buf := &bytes.Buffer{}
			err = defaultMarshaler.JSON(buf, s)
			body = buf.Bytes()
		}
		if err != nil {
			return err
		}
	case strings.HasPrefix(ct, "application/json") || ct == "":
	default:
		return errors.E("unmarshal request", errors.K.Invalid, "reason", "unacceptable content type", "content_type", ct)
	}
	err = json.Unmarshal(body, target)
	if err != nil {
		return errors.E("unmarshal request", errors.K.Invalid, err, "content_type", ct)
	}
	return nil
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
func GetCombinedHeader(header http.Header, query url.Values, queryFirst bool, headers ...string) string {
	res := make([]string, 0)
	hval := make([]string, 0)
	qval := make([]string, 0)
	for _, h := range headers {
		hval = hval[:0]
		if v, ok := header[http.CanonicalHeaderKey(h)]; ok {
			for _, s := range v {
				hval = append(hval, s)
			}
		}
		qval = qval[:0]
		if v, ok := query["header-"+
			strings.ReplaceAll(strings.ToLower(h), "-", "_")]; ok {
			for _, s := range v {
				qval = append(qval, s)
			}
		}
		if queryFirst {
			res = append(append(res, qval...), hval...)
		} else {
			res = append(append(res, hval...), qval...)
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
	if v, ok := header[http.CanonicalHeaderKey(DecryptModeHeader)]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	if v, ok := query["header-x_decryption_mode"]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	if len(res) == 0 {
		return def, nil
	}
	if len(res) > 1 {
		for _, s := range res[1:] {
			if s != res[0] {
				return "", errors.E("GetDecryptionMode",
					errors.K.Invalid,
					"reason", "multiple values (inconsistent)",
					"values", strings.Join(res, ","))
			}
		}
	}

	return res[0], nil
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
	if v, ok := header[http.CanonicalHeaderKey(SetContentDispositionHeader)]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	if v, ok := query["header-x_set_content_disposition"]; ok {
		for _, s := range v {
			res = append(res, s)
		}
	}
	if len(res) == 0 {
		return def, nil
	}
	if len(res) > 1 {
		for _, s := range res[1:] {
			if s != res[0] {
				return "", errors.E("GetSetContentDisposition",
					errors.K.Invalid,
					"reason", "multiple values (inconsistent)",
					"values", strings.Join(res, ","))
			}
		}
	}

	return res[0], nil
}

// ClientIP returns the client IP address for the given HTTP request. By default
// this is the IP address portion of the request's RemoteAddr (ip:port). If the
// request contains X-Forwarded-For or X-Real-IP headers (usually set by a
// reverse-proxy in front of the HTTP server, e.g. nginx), then the client IP is
// extracted from those headers.
//
// If an optional acceptHeadersFrom function is provided and refuses the
// RemoteAddr, then the above headers are ignored and the IP address from
// RemoteAddr is returned.
func ClientIP(r *http.Request, acceptHeadersFrom ...func(remoteAddr string) bool) string {
	if len(acceptHeadersFrom) > 0 && acceptHeadersFrom[0] != nil && !acceptHeadersFrom[0](r.RemoteAddr) {
		return strings.Split(r.RemoteAddr, ":")[0]
	}
	for _, headerName := range []string{"X-Forwarded-For", "X-Real-IP"} {
		header := r.Header.Get(headerName)
		if header == "" {
			continue
		}
		// header may have multiple values separated by comma
		// -> https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/X-Forwarded-For
		vals := strings.SplitN(header, ",", 2)
		// client IP is the first value
		return strings.TrimSpace(vals[0])
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

// ParseServerError tries parsing an error response from a fabric API call and
// returns the result as error.
func ParseServerError(body io.ReadCloser, httpStatusCode int) error {
	defer func() {
		_ = eioutil.Consume(body)
	}()

	var e errors.TemplateFn
	switch httpStatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		e = errors.TemplateNoTrace(errors.K.Permission, "status", httpStatusCode)
	case http.StatusNotFound:
		e = errors.TemplateNoTrace(errors.K.NotFound, "status", httpStatusCode)
	default:
		e = errors.TemplateNoTrace(errors.K.Internal, "status", httpStatusCode)
	}

	resp, err := ioutil.ReadAll(io.LimitReader(body, 1_000_000))
	if err != nil || len(resp) == 0 {
		return e(err, "body", string(resp))
	}
	list, err := errors.UnmarshalJsonErrorList(resp)
	if err != nil {
		return e("body", string(resp))
	}
	return e(list.ErrorOrNil())
}

// SetContentDisposition sets the Content-Disposition header as a filename attachment.
//
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Disposition
// See https://datatracker.ietf.org/doc/html/rfc5987
func SetContentDisposition(header http.Header, filename string) {
	if isoName, err := charmap.ISO8859_1.NewEncoder().String(filename); err == nil {
		header.Add("Content-Disposition", "attachment; filename=\""+isoName+"\"")
	} else {
		// we could always add this variant - multiple Content-Disposition headers are allowed
		header.Add("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename)+"")
	}
}
