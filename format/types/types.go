package types

import (
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/format/token"
)

// QSpaceID is the type of content space IDs
type QSpaceID = id.ID

// AccountID is the type of account IDs
type AccountID = id.ID

// UserID is the type of user IDs
type UserID = id.ID

// QLibID is the type of content library IDs
type QLibID = id.ID

// QID is the type of content IDs
type QID = id.ID

// QSSID is the type of content state store IDs
type QSSID = id.ID

// UploadJobID is the type of upload jobs IDs
type UploadJobID = id.ID

// FilesJobID is the type of files jobs IDs
type FilesJobID = id.ID

// QNodeID is the type of content node IDs
type QNodeID = id.ID

// NetworkID is the type of eluvio network IDs
type NetworkID = id.ID

// KmsID is the type of eluvio network IDs
type KmsID = id.ID

// QHash is the type of a content hash
type QHash = *hash.Hash

// QPHash is the type of content part hash
type QPHash = *hash.Hash

// QType is the type of a content. Since the content type is a bitcode module
// that is stored as a regular content object, the QType is in fact a content
// hash (QHash)
type QType = *hash.Hash

// QWriteToken is a token needed for writing to a content object
type QWriteToken = token.Token

// QReadToken is a token needed for reading from a content object
type QReadToken = token.Token

// QPWriteToken is a token needed for writing to a content part
type QPWriteToken = token.Token

// QPReadToken is a token needed for reading from a content part
type QPReadToken = token.Token

// Attributes is the type of content attributes
type Attributes struct{}

// MDValue is the type for metadata
type MDValue string

// KeyIterator is an iterator for metadata keys
type KeyIterator interface {
}

// MetaData is arbitrary, json-like meta-data
type MetaData interface{}

// SDPath is a path that references a sub structure within structured data.
type SDPath = structured.Path

// QIDSet is the full set of identifiers of a content object including the ID of
// the content library ID it belongs to, the content ID and the content version
// hash.
type QIDSet struct {
	QLibID QLibID
	QID    QID
	QHash  QHash
}
