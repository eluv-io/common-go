package resolvers

import (
	"encoding/json"
	"fmt"

	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
)

// foundLinks is the result of finding links in a data structure.
type foundLinks struct {
	// relative links
	rel []*lap
	// absolute links are collected per target content hash (string) for more
	// efficient resolution
	abs map[string]*absLink
}

// =============================================================================

// absLink is a collection of absolute links found pointing to the same remote
// content object, and their common root path in the remote structure.
type absLink struct {
	// the links and paths found in the same remote content object
	laps []*lap
	// the common root path of these links. Can be empty (corresponding to "/")
	rootPath structured.Path
}

// =============================================================================

// lap is a "link and path": a link and the path at which it was found in the
// source structure.
type lap struct {
	// the metadata link
	link *link.Link
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

// =============================================================================
