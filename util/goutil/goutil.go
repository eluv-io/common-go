package goutil

import "github.com/modern-go/gls"

// GoID returns the goroutine id of current goroutine
func GoID() int64 {
	return gls.GoID()
}
