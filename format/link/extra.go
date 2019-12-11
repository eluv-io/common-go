package link

import "github.com/qluvio/content-fabric/format/structured"

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
	return m
}

func (e *Extra) IsEmpty() bool {
	return e == nil || e.Container == "" && e.AutoUpdate == nil
}

func (e *Extra) UnmarshalMap(m map[string]interface{}) {
	au := structured.Wrap(m).Get("auto_update").Map(nil)
	if au != nil {
		e.AutoUpdate = &AutoUpdate{}
		e.AutoUpdate.UnmarshalMap(au)
	}
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
