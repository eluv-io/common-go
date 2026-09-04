package netutil

import (
	"encoding/json"
	"net"
	url "net/url"
	"strconv"
	"strings"

	"github.com/eluv-io/errors-go"
)

type URL struct {
	*url.URL
}

// MarshalJSON implements json.Marshaler to cope with url as string
func (u *URL) MarshalJSON() ([]byte, error) {
	s := ""
	if u.URL != nil {
		s = u.URL.String()
	}
	return json.Marshal(s)
}

// UnmarshalJSON implements json.Unmarshaler to cope with url as string
func (u *URL) UnmarshalJSON(b []byte) error {
	e := errors.Template("UnmarshalJSON", errors.K.Invalid)
	if len(b) == 0 {
		return nil
	}
	s := ""
	err := json.Unmarshal(b, &s)
	if err != nil {
		return e(err)
	}
	if len(s) == 0 {
		return nil
	}
	u.URL, err = url.Parse(s)
	if err != nil {
		return e(err)
	}
	return nil
}

func ExtractHostAndPort(fromUrl string) (host string, port int, err error) {
	var netUrl *url.URL
	netUrl, err = url.Parse(fromUrl)
	if err != nil {
		return
	}
	var portString string
	host, portString, err = net.SplitHostPort(netUrl.Host)
	if err != nil {
		return
	}
	port, err = strconv.Atoi(portString)
	return
}

func ExtractPort(fromUrl string) (port int, err error) {
	_, port, err = ExtractHostAndPort(fromUrl)
	return
}

// Join joins any number of path elements into a single path, separating them with slashes. Empty elements are ignored.
// Unlike path.Join(), the result is NOT Cleaned. However, if the argument list is empty or all its elements are empty,
// Join returns an empty string.
func Join(elem ...string) string {
	sb := strings.Builder{}
	hasEndingSlash := false
	for _, e := range elem {
		if e == "" {
			continue
		}
		if e[0] != '/' && sb.Len() > 0 && !hasEndingSlash {
			sb.WriteByte('/')
		} else if e[0] == '/' && hasEndingSlash {
			// collapse double slashes: "a/" + "/b" -> "a/b"
			e = e[1:]
		}
		hasEndingSlash = e[len(e)-1] == '/'
		sb.WriteString(e)
	}
	return sb.String()
}
