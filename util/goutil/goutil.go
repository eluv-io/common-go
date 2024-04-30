package goutil

import (
	"github.com/modern-go/gls"

	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/util/goutil")

// GoID returns the goroutine id of current goroutine
func GoID() int64 {
	return gls.GoID()
}

// Log logs entry info for the current goroutine and returns a function that logs exit info and which should be called
// with a defer statement.
//
// Example usage:
//
//	parentGID := goutil.GoID()
//	go func() {
//	    defer goutil.Log("testFn", parentGID, "id", id.String())()
//	    ...
//	}
//
// See also the Go() function below, which will run a goroutine and log entry and exit info in a single concise call.
func Log(name string, parentGoID int64, fields ...interface{}) func() {
	if !log.IsDebug() {
		return func() {}
	}
	if parentGoID > 0 {
		fields = append(fields, "parent_gid", parentGoID)
	}
	log.Debug("goroutine.enter "+name, fields...)
	return func() {
		log.Debug("goroutine.exit "+name, fields...)
	}
}

// Go starts a new goroutine and logs entry and exit info for that goroutine with the Log function. It automatically
// adds the parent goroutine ID.
//
// Example usage:
//
//	Go("processor", []any{"some", "context"},
//	    func() {
//	        ...
//	    },
//	)
//
// The optional deferFn is a function that is called after the exit log is written.
func Go(name string, fields []any, fn func(), deferFn ...func()) {
	deferFunc := func() {}
	if len(deferFn) > 0 && deferFn[0] != nil {
		deferFunc = deferFn[0]
	}
	if !log.IsDebug() {
		go func() {
			defer deferFunc()
			fn()
		}()
		return
	}
	parentGid := GoID()
	go func() {
		defer deferFunc()
		defer Log(name, parentGid, fields...)()
		fn()
	}()
}
