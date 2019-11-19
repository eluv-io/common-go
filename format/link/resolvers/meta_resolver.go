package resolvers

import (
	"github.com/qluvio/content-fabric/constants"
	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/util/jsonutil"
)

func newMetaResolver(provider MetaProvider) *metaResolver {
	return &metaResolver{
		resolverFns:      resolverFns{},
		mp:               provider,
		resolveFileLinks: false,
	}
}

type metaResolver struct {
	resolverFns
	mp               MetaProvider
	resolveFileLinks bool
}

func (r *metaResolver) Transform(target interface{}) (interface{}, error) {
	return r.ResolveMeta(nil, target, r.resolveFileLinks)
}

func (r *metaResolver) EnableFileLinkResolution() MetaResolver {
	if r.resolveFileLinks {
		return r
	}
	nm := *r
	nm.resolveFileLinks = true
	return &nm
}

// ResolveMeta resolves all metadata links in the target metadata with the
// values they point to. Chained links are resolved recursively. Circular
// relative links are detected and trigger an error (circular absolute links are
// not possible since they are defined with qhashes).
//
//  * container:        the qhash of the content object that has the given
//                      target metadata. Is nil for the original CO, non-nil for
//                      any CO that was followed through an absolute link.
//  * target:           the metadata to resolve
//  * resolveFileLinks: if true, relative file links are converted to regular
//                      metadata links and resolved as well. Allows to check
//                      file links for integrity
func (r *metaResolver) ResolveMeta(container *hash.Hash, target interface{}, resolveFileLinks bool) (interface{}, error) {
	filter := func(lnk *link.Link) (*link.Link, bool) {
		if resolveFileLinks && lnk.Selector == link.S.File {
			// turn file link into a meta link
			newLnk, err := link.NewLink(lnk.Target, link.S.Meta, append(constants.BundleFilesRoot(), lnk.Path...), lnk.Off, lnk.Len)
			if err == nil {
				lnk = newLnk
			}
		}
		if lnk.IsRelative() {
			// set the container for all relative links, so they can be traced
			// back correctly
			lnk.Container = container
		}
		if lnk.Selector == link.S.Meta {
			return lnk, true
		}
		return lnk, false
	}
	var err error
	var links *foundLinks
	target, links, err = r.collectLinks(target, filter)
	if err != nil {
		return nil, errors.E("ResolveMeta", err)
	}
	// first replace absolute links, since a relative link could point to a path
	// including an absolute link...
	target, err = r.replaceAbsoluteLinks(links.abs, target)
	if err == nil {
		target, err = r.replaceRelativeLinks(links.rel, target)
	}
	return target, err
}

func (r *metaResolver) replaceRelativeLinks(laps []*lap, target interface{}) (interface{}, error) {
	if len(laps) == 0 {
		return target, nil
	}

	for len(laps) > 0 {
		idx := 0
		removed := 0
		for idx < len(laps) {
			lp := laps[idx]
			containsLink := false
			e := errors.Template("ResolveMeta", errors.K.Invalid, "link", lp.link, "path", lp.path)
			val, err := structured.Resolve(lp.link.Path, target, func(val interface{}, path structured.Path) {
				containsLink = r.isMetaLink(val)
			})
			if err == nil && !r.isMetaLink(val) {
				// make sure link path is not pointing to a parent...
				if lp.path.Contains(lp.link.Path) {
					return nil, e("reason", "circular reference")
				}

				// link path contains no links (anymore) ==> replace
				val, err = r.handleLinkProps(val, lp)
				if err == nil {
					target, err = structured.Set(target, lp.path, val)
				}
				if err != nil {
					return nil, e(err)
				}
				// remove the link - re-ordering is not a problem
				last := len(laps) - 1
				laps[idx] = laps[last]
				laps[last] = nil
				laps = laps[:last]
				removed++
				continue
			}
			if containsLink || r.isMetaLink(val) {
				// the link's path contains another link or ends in a link...
				// skip and resolve later
				idx++
				continue
			}
			return nil, e(err)
		}
		if removed == 0 {
			return nil, errors.E("resolve links", errors.K.Invalid, "reason", "circular reference", "links", laps)
		}
	}
	return target, nil
}

func (r *metaResolver) isMetaLink(val interface{}) bool {
	lnk := link.AsLink(val)
	return lnk != nil && lnk.Selector == link.S.Meta
}

func (r *metaResolver) replaceAbsoluteLinks(abs map[string]*absLink, target interface{}) (interface{}, error) {
	for _, al := range abs {
		var err error
		var val interface{}
		var referenced interface{}
		for _, lp := range al.laps {
			e := errors.Template("resolve links", "link", lp.link, "path", lp.path)
			if referenced == nil {
				// fetch metadata of target content object
				referenced, err = r.mp.Meta(lp.link.Target.String(), nil)
				if err == nil {
					// and resolve its links
					referenced, err = r.ResolveMeta(lp.link.Target, referenced, r.resolveFileLinks)
				}
				if err == nil {
					referenced, err = structured.Resolve(al.rootPath, referenced)
				}
				if err != nil {
					return nil, e(err)
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
				return nil, e(err)
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
func (r *metaResolver) handleLinkProps(linkTargetData interface{}, lp *lap) (interface{}, error) {
	if len(lp.link.Props) == 0 {
		return linkTargetData, nil
	}
	data, err := jsonutil.Clone(linkTargetData)
	if err != nil {
		return nil, err
	}
	return structured.Merge(data, nil, lp.link.Props)
}
