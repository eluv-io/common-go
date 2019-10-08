package link

import (
	"encoding/json"
	"fmt"

	"eluvio/constants"
	"eluvio/errors"
	"eluvio/format/structured"
	"eluvio/util/jsonutil"
)

func NewMetaResolver(mp MetaProvider) MetaResolver {
	return &resolver{
		mp: mp,
	}
}

type resolver struct {
	mp MetaProvider
}

type foundLinks struct {
	// relative links
	rel []*lap
	// absolute links are collected per target content hash (string) for more
	// efficient resolution
	abs map[string]*absLink
}

// absLink is a collection of absolute links found pointing to the same remote
// content object, and their common root path in the remote metadata.
type absLink struct {
	// the links and paths found in the same remote content object
	laps []*lap
	// the common root path of these links. Can be empty (corresponding to "/")
	rootPath structured.Path
}

// lap is a "link and path": a link and the path at which it was found in the
// metadata.
type lap struct {
	// the metadata link
	link *Link
	// the path at which the link was found
	path structured.Path
}

func (l lap) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	m["link"] = l.link
	m["path"] = l.path
	return json.Marshal(m)
}

func (l lap) String() string {
	return fmt.Sprintf("link [%s] path [%s]", l.link, l.path)
}

func (r *resolver) ResolveMeta(target interface{}, resolveFileLinks bool) (interface{}, error) {
	var err error
	links := r.findMetaLinks(target, resolveFileLinks)

	// first replace absolute links, since a relative link could point to a path
	// including an absolute link...
	target, err = r.replaceAbsoluteLinks(links.abs, target, resolveFileLinks)
	if err == nil {
		target, err = r.replaceRelativeLinks(links.rel, target)
	}
	return target, err
}

func (r *resolver) findMetaLinks(target interface{}, resolveFileLinks bool) *foundLinks {
	links := &foundLinks{
		abs: make(map[string]*absLink),
	}

	structured.Visit(target, false, func(path structured.Path, val interface{}) bool {
		var lnk *Link
		switch l := val.(type) {
		case *Link:
			lnk = l
		case Link:
			lnk = &l
		default:
			return true
		}
		if resolveFileLinks && lnk.Selector == S.File {
			// turn file link into a meta link
			newLnk, err := NewLink(lnk.Target, S.Meta, append(constants.BundleFilesRoot(), lnk.Path...), lnk.Off, lnk.Len)
			if err == nil {
				lnk = newLnk
			}
		}
		if lnk.Selector == S.Meta {
			if lnk.IsAbsolute() {
				hsh := lnk.Target.String()
				al, found := links.abs[hsh]
				if !found {
					al = &absLink{}
				}
				al.laps = append(al.laps, &lap{
					path: path,
					link: lnk,
				})
				al.rootPath = al.rootPath.CommonRoot(lnk.Path)
				links.abs[hsh] = al
			} else {
				links.rel = append(links.rel, &lap{
					path: path,
					link: lnk,
				})
			}
		}
		return true
	})
	return links
}

func (r *resolver) replaceRelativeLinks(laps []*lap, target interface{}) (interface{}, error) {
	if len(laps) == 0 {
		return target, nil
	}

	for len(laps) > 0 {
		idx := 0
		removed := 0
		for idx < len(laps) {
			lp := laps[idx]
			containsLink := false
			val, err := structured.Resolve(lp.link.Path, target, func(val interface{}, path structured.Path) {
				containsLink = IsLink(val)
			})
			if err == nil && !IsLink(val) {
				// make sure link path is not pointing to a parent...
				if lp.path.Contains(lp.link.Path) {
					return nil, errors.E("resolve links", errors.K.Invalid, "reason", "circular reference", "link", lp.link, "path", lp.path)
				}

				// link path contains no links (anymore) ==> replace
				val, err = r.handleLinkProps(val, lp)
				if err != nil {
					return nil, errors.E("resolve links", err, "link", lp.link, "path", lp.path)
				}
				target, err = structured.Set(target, lp.path, val)
				// remove the link - re-ordering is not a problem
				last := len(laps) - 1
				laps[idx] = laps[last]
				laps[last] = nil
				laps = laps[:last]
				removed++
				continue
			}
			if containsLink || IsLink(val) {
				// the link's path contains another link or ends in a link...
				// skip and resolve later
				idx++
				continue
			}
			return nil, errors.E("resolve links", errors.K.Invalid, err, "link", lp.link, "path", lp.path)
		}
		if removed == 0 {
			return nil, errors.E("resolve links", errors.K.Invalid, "reason", "circular reference", "links", laps)
		}
	}
	return target, nil
}

func (r *resolver) replaceAbsoluteLinks(abs map[string]*absLink, target interface{}, resolveFileLinks bool) (interface{}, error) {
	for _, al := range abs {
		var err error
		var val interface{}
		var referenced interface{}
		for _, lp := range al.laps {
			e := errors.Template("resolve links", errors.K.Invalid, "link", lp.link, "path", lp.path)
			if referenced == nil {
				// fetch metadata of target content object
				referenced, err = r.mp.Meta(lp.link.Target, al.rootPath)
				if err == nil {
					// and resolve its links
				}
				referenced, err = r.ResolveMeta(referenced, resolveFileLinks)
				if err != nil {
					return nil, e().Cause(err)
				}
			}
			val, err = structured.Resolve(lp.link.Path[len(al.rootPath):], referenced)
			if err == nil {
				val, err = r.handleLinkProps(val, lp)
				if err == nil {
					target, err = structured.Set(target, lp.path, val)
				}
			}
			if err != nil {
				return nil, e().Cause(err)
			}
		}
	}
	return target, nil
}

// handleLinkProps makes a copy of the link's target structure, merges the
// link properties into it and returns the merged structure.
// The copy is necessary so that multiple links pointing to the same target with
// different link props don't clash.
// Returns the original target structure if there are no link props.
func (r *resolver) handleLinkProps(linkTargetData interface{}, lp *lap) (interface{}, error) {
	if len(lp.link.Props) == 0 {
		return linkTargetData, nil
	}
	data, err := jsonutil.Clone(linkTargetData)
	if err != nil {
		return nil, err
	}
	return structured.Merge(data, nil, lp.link.Props)
}
