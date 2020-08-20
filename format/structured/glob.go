package structured

import (
	"encoding/json"
	"strconv"

	"github.com/qluvio/content-fabric/util/jsonutil"

	"github.com/qluvio/content-fabric/log"
)

const wildcard = "*"

// FilterGlob filters the given target according to the provided "select" and
// "remove" paths. Only elements at "select" paths are included in the result
// and further reduced by "remove" paths. Hence removal takes precedence in case
// of conflicting select and remove paths.
//
// "select" and "remove" paths may contain wildcards '*' in place of path
// segments, e.g. /a/*/b or /a/*/*/b/*/c. A wildcard therefore represents all
// keys in a map or all indices in a slice.
//
// Partial path segments (e.g. /a/prefix*/*suffix) are not supported. Neither is
// the single character wildcard '?'.
//
// If no select paths are specified, a select "/" is used (i.e. selecting the
// full target) and then applying the removals. If neither select nor remove
// paths are specified, the target is returned unchanged.
//
// The original target data structure is never modified. Instead, new maps or
// slices are created and populated as need. However, any unchanged map or slice
// is not copied and referenced directly.
//
// This function tries to optimize the filtering in the following ways: first it
// analyzes the select/remove paths and creates a filter tree, collapsing and
// pruning paths as much as possible. Then it traverses the target structure
// along the filter tree structure. Wildcards in the filter tree are expanded by
// iterating through the corresponding elements in the target structure.
//
// See unit tests for examples.
func FilterGlob(target interface{}, selectPaths, removePaths []Path) interface{} {
	if len(selectPaths) == 0 && len(removePaths) == 0 {
		return target
	}

	filter := createFilter(selectPaths, removePaths)
	if log.IsDebug() {
		log.Debug("filter.glob", "select", selectPaths, "remove", removePaths, "filters", filter)
	}
	res := filter.Filter(target)
	return res
}

func createFilter(selectPaths, removePaths []Path) *globFilter {
	res := &globFilter{
		typ: typVoid,
	}

	if len(selectPaths) == 0 {
		res.typ = typSelect
	}

	for _, path := range selectPaths {
		curr := res
		pathLen := len(path)
		if pathLen == 0 {
			res.typ = typSelect
		}
		for segIdx, seg := range path {
			typ := typVoid
			if segIdx+1 == pathLen {
				typ = typSelect
			}
			curr = curr.Add(seg, typ)
		}
	}

	for _, path := range removePaths {
		curr := res
		pathLen := len(path)
		if pathLen == 0 {
			res.typ = typRemove
			res.children = nil
			break
		}
		for segIdx, seg := range path {
			typ := typVoid
			if segIdx+1 == pathLen {
				typ = typRemove
			}
			curr = curr.Add(seg, typ)
		}
	}

	return res
}

type filterType string

const (
	// the node must be selected
	typSelect = filterType("select")
	// the node must be removed
	typRemove = filterType("remove")
	// path segments to the first select or remove node
	typVoid = filterType("void")
)

// globFilter is a node in the filter tree built from select/remove paths.
type globFilter struct {
	seg      string // just needed by unit test for simpler filter construction
	typ      filterType
	children map[string]*globFilter
}

func (f *globFilter) MarshalJSON() ([]byte, error) {
	type alias struct {
		Typ      filterType             `json:"type"`
		Children map[string]*globFilter `json:"sub,omitempty"`
	}
	return json.Marshal(&alias{
		Typ:      f.typ,
		Children: f.children,
	})
}

func (f *globFilter) String() string {
	return jsonutil.MarshalCompactString(f)
}

func (f *globFilter) Add(seg string, typ filterType) *globFilter {
	isWildcard := seg == wildcard
	if isWildcard && typ != typVoid {
		f.children = nil
	} else if child, has := f.children[seg]; has {
		if typ != typVoid {
			child.typ = typ
			child.children = nil
		}
		return child
	}
	if typ == typVoid {
		typ = f.typ
	}
	return f.AddChild(&globFilter{
		seg: seg,
		typ: typ,
	})
}

func (f *globFilter) AddChild(child *globFilter) *globFilter {
	if f.children == nil {
		f.children = map[string]*globFilter{}
	}
	f.children[child.seg] = child
	return child
}

func (f *globFilter) Filter(target interface{}) interface{} {
	res, _ := f.filter(target, false)
	return res
}

func (f *globFilter) filter(target interface{}, selectAll bool) (interface{}, bool) {
	if f.typ == typRemove {
		return nil, false
	}
	if f.children == nil {
		return target, true
	}

	selectAll = selectAll || f.typ == typSelect

	node := dereference(target)

	switch t := node.(type) {
	case map[string]interface{}:
		res := make(map[string]interface{}, len(t))
		wcf, hasWildcard := f.children[wildcard]
		if hasWildcard {
			selectAll = selectAll || wcf.typ == typSelect
		}
		if hasWildcard || selectAll {
			for k, v := range t {
				retainWC := true
				if hasWildcard {
					if nv, retain := wcf.filter(v, selectAll); retain {
						retainWC = true
						v = nv
					} else {
						retainWC = false
					}
				}
				if child, found := f.children[k]; found {
					if nv, retain := child.filter(v, selectAll); retain {
						res[k] = nv
					}
					continue
				}
				if retainWC {
					res[k] = v
				}
			}
		} else {
			// no wildcard
			for k, child := range f.children {
				if v, has := t[k]; has {
					if nv, retain := child.filter(v, selectAll); retain {
						res[k] = nv
					}
				}
			}
		}
		if len(res) > 0 {
			return res, true
		}
		return nil, false
	case []interface{}:
		res := make([]interface{}, 0, len(t))
		wcf, hasWildcard := f.children[wildcard]
		if hasWildcard {
			selectAll = selectAll || wcf.typ == typSelect
		}
		var retain bool
		if hasWildcard || selectAll {
			for i, v := range t {
				if hasWildcard {
					v, retain = wcf.filter(v, selectAll)
					if !retain {
						continue
					}
				}
				if child, found := f.children[strconv.Itoa(i)]; found {
					v, retain = child.filter(v, selectAll)
					if !retain {
						continue
					}
				}
				res = append(res, v)
			}
		} else {
			// no wildcard
			for k, child := range f.children {
				i, err := strconv.Atoi(k)
				if err != nil || i >= len(t) {
					continue
				}
				v := t[i]
				v, retain = child.filter(v, selectAll)
				if retain {
					res = append(res, v)
				}
			}
		}
		if len(res) > 0 {
			return res, true
		}
		return nil, false
	default:
		return target, selectAll
	}
}
