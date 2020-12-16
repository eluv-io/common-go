package netutil

import (
	"net/url"
	"strings"

	"github.com/qluvio/content-fabric/errors"
)

// FindLocator returns true if the provided list of locators contains the
// candidate locator, false otherwise.
// Returns false if the candidate locator is malformed.
func FindLocator(locators []string, candidate string) bool {
	c, err := NormalizeLocator(candidate)
	if err != nil {
		return false
	}
	for _, locator := range locators {
		loc, err := NormalizeLocator(locator)
		if err != nil {
			continue
		}
		if c == loc {
			return true
		}
	}
	return false
}

// NormalizeLocator normalizes the given locator to its standards form:
//   http://node-1.contentfabric.net:80/
//   https://node-1.contentfabric.net:443/
func NormalizeLocator(locator string) (string, error) {
	u, err := url.Parse(locator)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" {
		return "", errors.E("invalid locator", errors.K.Invalid, err, "locator", locator)
	}
	if u.Port() == "" {
		switch u.Scheme {
		case "http":
			u.Host += ":80"
		case "https":
			u.Host += ":443"
		}
	}
	if u.Path != "/" && !strings.HasSuffix(u.Path, "/") {
		u.Path = u.Path + "/"
	}
	return u.String(), nil
}
