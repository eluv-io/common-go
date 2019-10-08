package link

import (
	"io"

	"eluvio/format/structured"
	"eluvio/format/types"
)

// ResolverFactory provides implementations for the different resolvers.
type ResolverFactory interface {

	// MetaResolver returns a MetaResolver based on the given MetaProvider.
	MetaResolver(mp MetaProvider) MetaResolver
}

// MetaResolver resolves metadata links within metadata.
type MetaResolver interface {

	// ResolveMeta resolves any metadata links in the given target structure and
	// inlines the data they reference. Links can be relative (intra-object) or
	// absolute (inter-object). If resolveFileLinks is true, then file links are
	// also resolved to their internal bundle representation.
	//
	// Returns the modified structure or an error if any of the links cannot be
	// resolved or form a circular reference.
	//
	// Note: the original structure provided to this function might be modified
	// and should not be used consequently - instead use the returned structure:
	//
	//   structure, err = resolver.ResolveMeta(structure)
	//
	// In case of an error, use neither the original (might be partially
	// modified) nor the returned (nil) data structure.
	ResolveMeta(target interface{}, resolveFileLinks bool) (interface{}, error)
}

// MetaProvider retrieves the metadata subtree at a given path of a given
// content object.
type MetaProvider interface {
	// Meta retrieves the meta data for the given hash rooted at path.
	Meta(qhash types.QHash, path structured.Path) (interface{}, error)
}

// BinaryOps defines the common functions available on the following types
// of links:
//   - ./files
//   - ./rep
//   - /qfab/hpq_XYZ
//   - /qfab/hpq_XYZ#x-y
type BinaryOps interface {

	// Size retrieves the size of the binary data referenced by the given link.
	Size(link Link) (int64, error)

	// Read reads the (binary) data referenced by the given link.
	Reader(link Link) (io.ReadCloser, error)

	// Read reads the (binary) data referenced by the given link.
	ReaderOffLen(link Link, off, length int64) (io.ReadCloser /* , *ContentRange */, error)
}

type FileOps interface {
	BinaryOps

	Meta(link Link) (interface{}, error)
}

// =============================================================================

func NewResolverFactory() ResolverFactory {
	return &resolverFactory{}
}

type resolverFactory struct {
}

func (r *resolverFactory) MetaResolver(mp MetaProvider) MetaResolver {
	return NewMetaResolver(mp)
}
