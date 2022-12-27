package link

import (
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/errors-go"
)

// Extra are additional link features. They are stored in "." in the JSON
// representation:
//
//	{
//		"/": "./meta/blub",
//		".": {
//			"auto_update": {
//				"tag": "latest"
//			},
//			"container": "hq__AAA"
//		},
//		"other-link-prop1": "zonk",
//		"other-link-prop2": "zink"
//	}
type Extra struct {
	// NOTE: DO NOT CHANGE FIELD TYPES, THEIR ORDER OR REMOVE ANY FIELDS SINCE STRUCT IS CBOR-ENCODED AS ARRAY!
	_ struct{} `cbor:",toarray"` // encode struct as array

	// The auto-update
	AutoUpdate *AutoUpdate `json:"auto_update,omitempty"`
	// The hash or write token of the content object in which a relative link is defined. This is a temporary attribute
	// used in link resolution. It is only marshalled to JSON, but not stored in CBOR.
	Container string `json:"container,omitempty" cbor:"-"`
	// An error if this link could not be resolved during link resolution. This a temporary attribute, but is also
	// marshalled when set.
	ResolutionError error `json:"resolution_error,omitempty" cbor:"-"`
	// The authorization for a "signed link": the token contains the signature of a user that is editor of the the link
	// target. See https://github.com/qluvio/proj-mgm/issues/14#issuecomment-724867064
	Authorization string `json:"authorization,omitempty"`
	// EnforceAuth determines whether the link's target path should be explicitly authorized. False per default, which
	// means that links from public meta to private meta are interpreted as implicitly permitted. If true, then the
	// link's target path is authorized as if accessed directly through /meta/path.
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
			!e.EnforceAuth
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

// func (e *Extra) MarshalCBOR(m map[string]interface{}) {
// 	if e.IsEmpty() {
// 		return
// 	}
// 	if e.AutoUpdate != nil {
// 		m["eau"] = e.AutoUpdate.MarshalMap()
// 	}
// 	if e.Authorization != "" {
// 		m["ea"] = e.Authorization
// 	}
// 	if e.EnforceAuth {
// 		m["eea"] = e.EnforceAuth
// 	}
// 	// container and resolution error are not stored in CBOR!
// }
//
// func (e *Extra) UnmarshalCBOR(m map[string]interface{}) {
// 	au := m["eau"].(map[string]interface{})
// 	if au != nil {
// 		e.AutoUpdate = &AutoUpdate{}
// 		e.AutoUpdate.UnmarshalMap(au)
// 	}
// 	e.Authorization, _ = m["authorization"].(string)
// 	e.EnforceAuth, _ = m["enforce_auth"].(bool)
// }

func (e *Extra) UnmarshalValue(val *structured.Value) error {
	err := val.Decode(e)
	if err != nil {
		return errors.NoTrace("extra.UnmarshalValue", err)
	}
	return nil
}

func (e *Extra) UnmarshalValueAndRemove(extra *structured.Value) error {
	err := e.UnmarshalValue(extra)
	if err != nil {
		return err
	}
	extra.Delete("auto_update")
	extra.Delete("container")
	extra.Delete("resolution_error")
	extra.Delete("authorization")
	extra.Delete("enforce_auth")
	return nil
}

// AutoUpdate is the structure for auto-update information
type AutoUpdate struct {
	// NOTE: DO NOT CHANGE FIELD TYPES, THEIR ORDER OR REMOVE ANY FIELDS SINCE STRUCT IS ENCODED AS ARRAY!
	_   struct{} `cbor:",toarray"` // encode struct as array
	Tag string   `json:"tag"`
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
