package ctxutil

import (
	"context"

	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/eluvio/util/ctxutil")

var current = NewStack()

func Current() ContextStack {
	return current
}

func SetCurrent(c ContextStack) {
	current = c
}

// Ctx returns the current context.
func Ctx() context.Context {
	return current.Ctx()
}
