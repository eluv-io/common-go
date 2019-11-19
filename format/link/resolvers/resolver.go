package resolvers

import (
	"github.com/qluvio/content-fabric/format/structured"
)

// MetaResolver resolves metadata links within metadata.
// Its Transform() function resolves any metadata links in the given target
// structure and inlines the data they reference. Links can be relative
// (intra-object) or absolute (inter-object).
//
// Returns the modified structure or an error if any of the links cannot be
// resolved or form a circular reference.
//
// Note: the original structure provided to this function might be modified
// and should not be used subsequently - instead use the returned structure:
//
//   structure, err = resolver.Transform(structure)
//
// In case of an error, use neither the original (might be partially
// modified) nor the returned (nil) data structure.
type MetaResolver interface {
	structured.Transformer

	// EnableFileLinkResolution returns a metaResolver with file link resolution
	// enabled: file links are also resolved to their internal bundle
	// representation.
	EnableFileLinkResolution() MetaResolver
}

// MetaProvider retrieves the metadata subtree at a given path of a given
// content object.
type MetaProvider interface {
	// Meta retrieves the meta data for the given hash rooted at path.
	Meta(qhot string, path structured.Path) (interface{}, error)
}

// Creates a new metaResolver
func NewMetaResolver(mp MetaProvider) MetaResolver {
	return newMetaResolver(mp)
}
