package memutil

import (
	"reflect"
	"sync"

	"github.com/qluvio/content-fabric/util/maputil"
)

func NewMemScanRegistry() MemScanRegistry {
	res := &registry{
		roots: map[string]interface{}{},
	}
	return res
}

// MemScanRegistry allows registration of arbitrary objects for memory scanning.
// Any registered object will be available on the memsize debug web page for
// scanning.
type MemScanRegistry interface {

	// Registers an object with the given name. Panics if the object is not a
	// non-nil pointer.
	Register(name string, obj interface{})

	// Returns a sorted list of the names of all registered objects.
	Names() []string

	// Get's the registered object with the given name.
	Get(name string) (obj interface{}, ok bool)
}

type registry struct {
	mu    sync.Mutex
	roots map[string]interface{}
}

func (r *registry) Register(name string, obj interface{}) {
	rv := reflect.ValueOf(obj)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		panic("registered object must be non-nil pointer")
	}
	r.mu.Lock()
	r.roots[name] = obj
	r.mu.Unlock()
}

func (r *registry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maputil.SortedKeys(r.roots)
}

func (r *registry) Get(name string) (obj interface{}, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	obj, ok = r.roots[name]
	return obj, ok
}
