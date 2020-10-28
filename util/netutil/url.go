package netutil

import (
	"encoding/json"
	"net/url"

	"github.com/qluvio/content-fabric/errors"
)

type URL struct {
	*url.URL
}

// MarshalJSON implements json.Marshaler to cope with url as string otherwise
func (u *URL) MarshalJSON() ([]byte, error) {
	s := ""
	if u.URL != nil {
		s = u.URL.String()
	}
	return json.Marshal(s)
}

// MarshalJSON implements json.Unmarshaler to cope with url as string
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
