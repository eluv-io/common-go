package link

import "github.com/eluv-io/inject-go"

// NewModule returns the bindings for the link package
func NewModule() inject.Module {
	m := inject.NewModule()
	m.BindSingletonConstructor(NewResolverFactory)
	return m
}
