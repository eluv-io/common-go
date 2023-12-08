package goutil

import (
	"github.com/modern-go/gls"

	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/eluvio/util/goutil")

// GoID returns the goroutine id of current goroutine
func GoID() int64 {
	return gls.GoID()
}

// Log logs entry and exit info for the current goroutine
// Example usage:
//     defer goutil.Log("testFn", parentGID, "id", id.String())()
func Log(name string, parentGoID int64, fields ...interface{}) func() {
	if parentGoID > 0 {
		fields = append(fields, "parent_gid", parentGoID)
	}
	log.Debug("goroutine.enter "+name, fields...)
	return func() {
		log.Debug("goroutine.exit "+name, fields...)
	}
}
