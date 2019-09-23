package format

import (
	"github.com/eluv-io/inject-go"
)

// NewModule returns the bindings for the daemon package
func NewModule() inject.Module {
	m := inject.NewModule()
	m.BindSingletonConstructor(NewFactory)
	return m
}
