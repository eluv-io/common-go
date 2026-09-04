package testutil

import (
	"fmt"

	"github.com/stretchr/testify/assert"
)

// CollectT implements the TestingT interface and collects all errors instead of failing immediately. The errors can
// be "applied" to a real testing.T instance by calling CollectT.Copy(t).
type CollectT struct {
	errors []error
}

// Errorf collects the error.
func (c *CollectT) Errorf(format string, args ...interface{}) {
	c.errors = append(c.errors, fmt.Errorf(format, args...))
}

// FailNow panics.
func (c *CollectT) FailNow() {
	panic("Assertion failed")
}

// Reset clears the collected errors.
func (c *CollectT) Reset() {
	c.errors = nil
}

// Copy copies the collected errors to the supplied t.
func (c *CollectT) Copy(t assert.TestingT) {
	if tt, ok := t.(tHelper); ok {
		tt.Helper()
	}
	for _, err := range c.errors {
		t.Errorf("%v", err)
	}
}

type tHelper = interface {
	Helper()
}
