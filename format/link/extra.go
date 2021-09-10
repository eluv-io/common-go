package link

import (
	"github.com/qluvio/content-fabric/format/structured"
)

// Extra are additional link features. They are stored in "." in the JSON
// representation:
//  {
//  	"/": "./meta/blub",
//  	".": {
//  		"auto_update": {
//  			"tag": "latest"
//  		},
//  		"container": "hq__AAA"
//  	},
//  	"other-link-prop1": "zonk",
//  	"other-link-prop2": "zink"
//  }
type Extra struct {
	// The auto-update
	AutoUpdate *AutoUpdate `json:"auto_update,omitempty"`
	// The hash or write token of the content object in which a relative link is
	// defined. This is a temporary attribute used in link resolution, but is
	// also marshalled when set.
	Container string `json:"container,omitempty"`
	// An error if this link could not be resolved during link resolution. This
	// a temporary attribute, but is also marshalled when set.
	ResolutionError error `json:"resolution_error,omitempty"`
	// The authorization for a "signed link": the token contains the signature
	// of a user that is editor of the the link target.
	// See https://github.com/qluvio/proj-mgm/issues/14#issuecomment-724867064
	Authorization string `json:"authorization,omitempty"`
	// EnforceAuth determines whether the link's target path should be
	// explicitly authorized. False per default, which means that links from
	// public meta to private meta are interpreted as implicitly permitted. If
	// true, then the link's target path is authorized as if accessed directly
	// through /meta/path.
	EnforceAuth bool `json:"enforce_auth,omitempty"`
}

func (e *Extra) MarshalMap() map[string]interface{} {
	if e.IsEmpty() {
		return nil
	}
	m := make(map[string]interface{})
	if e.AutoUpdate != nil {
		m["auto_update"] = e.AutoUpdate.MarshalMap()
	}
	if e.Container != "" {
		m["container"] = e.Container
	}
	if e.ResolutionError != nil {
		m["resolution_error"] = e.ResolutionError
	}
	if e.Authorization != "" {
		m["authorization"] = e.Authorization
	}
	if e.EnforceAuth {
		m["enforce_auth"] = e.EnforceAuth
	}
	return m
}

func (e *Extra) IsEmpty() bool {
	return e == nil ||
		e.Container == "" &&
			e.AutoUpdate == nil &&
			e.ResolutionError == nil &&
			e.Authorization == "" &&
			e.EnforceAuth == false
}

func (e *Extra) UnmarshalMap(m map[string]interface{}) {
	au := structured.Wrap(m).Get("auto_update").Map(nil)
	if au != nil {
		e.AutoUpdate = &AutoUpdate{}
		e.AutoUpdate.UnmarshalMap(au)
	}
	e.Authorization, _ = m["authorization"].(string)
	e.EnforceAuth, _ = m["enforce_auth"].(bool)
}

// AutoUpdate is the structure for auto-update information
type AutoUpdate struct {
	Tag string `json:"tag"`
}

func (a *AutoUpdate) MarshalMap() map[string]interface{} {
	m := make(map[string]interface{})
	if a.Tag != "" {
		m["tag"] = a.Tag
	}
	return m
}

func (a *AutoUpdate) UnmarshalMap(m map[string]interface{}) {
	a.Tag = structured.Wrap(m).Get("tag").String()
}

func (a *AutoUpdate) Clone() *AutoUpdate {
	if a == nil {
		return a
	}
	clone := *a
	return &clone
}
