package format

import (
	"crypto/sha256"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/token"
	. "github.com/eluv-io/common-go/format/types"
)

// Factory provides all format-related constructors and generators like codecs,
// digests, etc.
type Factory interface {
	// NewContentDigest returns a digest object for calculating content hashes.
	NewContentDigest(format hash.Format, id QID) *hash.Digest
	// NewContentPartDigest returns a digest object for calculating content hashes.
	NewContentPartDigest(format hash.Format) *hash.Digest

	// GenerateAccountID generates a new account ID
	GenerateAccountID() AccountID
	// GenerateUserID generates a new user ID
	GenerateUserID() UserID
	// GenerateQLibID generates a new content library ID
	GenerateQLibID() QLibID
	// GenerateQID generates a new content ID
	GenerateQID() QID
	// GenerateQSSID generates a new content state store ID
	GenerateQSSID() QSSID
	// GenerateUploadJobID generates a new upload job ID
	GenerateUploadJobID() UploadJobID
	// GenerateFilesJobID generates a new upload job ID
	GenerateFilesJobID() FilesJobID
	// GenerateQNodeID generates a new content node ID
	GenerateQNodeID() QNodeID
	// GenerateNetworkID generates a new network ID
	GenerateNetworkID() NetworkID

	// ParseAccountID parses the given string as an account ID
	ParseAccountID(s string) (AccountID, error)
	// ParseUserID parses the given string as a user ID
	ParseUserID(s string) (UserID, error)
	// ParseQLibID parses the given string as a content library ID
	ParseQLibID(s string) (QLibID, error)
	// ParseQID parses the given string as a content ID
	ParseQID(s string) (QID, error)
	// ParseQSSID parses the given string as a content state store ID
	ParseQSSID(s string) (QSSID, error)
	// ParseQNodeID parses the given string as a content node ID
	ParseQNodeID(s string) (QNodeID, error)
	// ParseNetworkID parses the given string as a network ID
	ParseNetworkID(s string) (NetworkID, error)

	// ParseQHash parses the given string as content hash
	ParseQHash(s string) (QHash, error)
	// ParseQWriteToken parses the string as content write token
	ParseQWriteToken(s string) (QWriteToken, error)

	// GenerateQWriteToken creates a content write token
	GenerateQWriteToken(qid QID, nid QNodeID) (QWriteToken, error)
	// GenerateQPWriteToken creates a content part write token
	GenerateQPWriteToken(scheme encryption.Scheme, flags byte) (QPWriteToken, error)
	// GenerateLROHandle creates a handle for long running bitcode operations
	GenerateLROHandle(nid QNodeID) (LROHandle, error)

	// ParseQPWriteToken parses the given string as content part write token
	ParseQPWriteToken(s string) (QPWriteToken, error)
	// ParseQPHash parses the given string as content part hash. Returns nil
	// if the string is empty invalid.
	ParseQPHash(s string) (QPHash, error)
	// ParseQPLHash parses the given string as live content part hash
	ParseQPLHash(s string) (QPHash, error)

	// ParseQType parses the given string as content type
	ParseQType(s string) (QType, error)

	// ParseQSpaceID parses the given string as a content space ID
	ParseQSpaceID(s string) (QSpaceID, error)

	// ParseUploadJobID parses the given string as an upload job ID
	ParseUploadJobID(s string) (UploadJobID, error)
	// ParseFilesJobID parses the given string as an upload job ID
	ParseFilesJobID(s string) (FilesJobID, error)

	// NewMetadataCodec returns the codec for serializing metadata
	NewMetadataCodec() codecs.MultiCodec
}

// NewFactory creates a new format factory.
func NewFactory() Factory {
	return &factory{}
}

// Factory is the factory for all format-related generators like codecs,
// digests, etc.
type factory struct{}

// NewContentDigest returns a digest object for calculating content hashes.
func (f *factory) NewContentDigest(format hash.Format, id QID) *hash.Digest {
	return hash.NewDigest(sha256.New(), hash.Type{hash.Q, format}).WithID(id)
}

// NewContentPartDigest returns a digest object for calculating content hashes.
func (f *factory) NewContentPartDigest(format hash.Format) *hash.Digest {
	return hash.NewDigest(sha256.New(), hash.Type{hash.QPart, format})
}

// GenerateAccountID generates a new account ID
func (f *factory) GenerateAccountID() AccountID {
	return id.Generate(id.Account)
}

// ParseAccountID parses the given string as an account ID
func (f *factory) ParseAccountID(s string) (AccountID, error) {
	return id.Account.FromString(s)
}

// GenerateUserID generates a new user ID
func (f *factory) GenerateUserID() UserID {
	return id.Generate(id.User)
}

// ParseUserID parses the given string as a user ID
func (f *factory) ParseUserID(s string) (UserID, error) {
	return id.User.FromString(s)
}

// GenerateQLibID generates a new content library ID
func (f *factory) GenerateQLibID() QLibID {
	return id.Generate(id.QLib)
}

// ParseQLibID parses the given string as a content library ID
func (f *factory) ParseQLibID(s string) (QLibID, error) {
	return id.QLib.FromString(s)
}

// GenerateQID generates a new content ID
func (f *factory) GenerateQID() QID {
	return id.Generate(id.Q)
}

// ParseQID parses the given string as a content ID
func (f *factory) ParseQID(s string) (QID, error) {
	return id.Q.FromString(s)
}

// GenerateQSSID generates a new content ID
func (f *factory) GenerateQSSID() QSSID {
	return id.Generate(id.QStateStore)
}

// ParseQSSID parses the given string as a content state store ID
func (f *factory) ParseQSSID(s string) (QSSID, error) {
	return id.QStateStore.FromString(s)
}

// GenerateUploadJobID generates a upload job ID
func (f *factory) GenerateUploadJobID() UploadJobID {
	return id.Generate(id.QFileUpload)
}

// ParseUploadJobID parses the given string as an upload job ID
func (f *factory) ParseUploadJobID(s string) (UploadJobID, error) {
	return id.QFileUpload.FromString(s)
}

// GenerateFilesJobID generates a files job ID
func (f *factory) GenerateFilesJobID() FilesJobID {
	return id.Generate(id.QFilesJob)
}

// ParseFilesJobID parses the given string as a files job ID
func (f *factory) ParseFilesJobID(s string) (FilesJobID, error) {
	return id.QFilesJob.FromString(s)
}

// GenerateQNodeID generates a content node ID
func (f *factory) GenerateQNodeID() QNodeID {
	return id.Generate(id.QNode)
}

// ParseQNodeID parses the given string as a content node ID
func (f *factory) ParseQNodeID(s string) (QNodeID, error) {
	return id.QNode.FromString(s)
}

// GenerateNetworkID generates a network ID
func (f *factory) GenerateNetworkID() NetworkID {
	return id.Generate(id.Network)
}

// ParseNetworkID parses the given string as a network ID
func (f *factory) ParseNetworkID(s string) (NetworkID, error) {
	return id.Network.FromString(s)
}

// ParseQHash parses the given string as content hash
func (f *factory) ParseQHash(s string) (QHash, error) {
	return hash.Q.FromString(s)
}

// ParseQWriteToken parses the string as content write token
func (f *factory) ParseQWriteToken(s string) (QWriteToken, error) {
	return token.FromString(s)
}

// GenerateQWriteToken creates a content part write token
func (f *factory) GenerateQWriteToken(qid QID, nid QNodeID) (QWriteToken, error) {
	return token.NewObject(token.QWrite, qid, nid)
}

// GenerateQPWriteToken creates a content part write token
func (f *factory) GenerateQPWriteToken(scheme encryption.Scheme, flags byte) (QPWriteToken, error) {
	return token.NewPart(token.QPartWrite, scheme, flags)
}

// GenerateLROHandle creates a handle for long running bitcode operations
func (f *factory) GenerateLROHandle(nid QNodeID) (LROHandle, error) {
	return token.NewLRO(token.LRO, nid)
}

// ParseQPWriteToken parse the given string as content part write token
func (f *factory) ParseQPWriteToken(s string) (QPWriteToken, error) {
	return token.FromString(s)
}

// ParseQPHash parses the given string as content part hash
func (f *factory) ParseQPHash(s string) (QPHash, error) {
	h, err := hash.FromString(s)
	if err != nil {
		return nil, err
	} else if h.IsNil() {
		return nil, nil
	}
	switch h.Type.Code {
	case hash.QPart, hash.QPartLive, hash.QPartLiveTransient:
		return h, nil
	default:
		return nil, errors.E("parse hash", errors.K.Invalid, "reason", "invalid code", "hash", s)
	}
}

// ParseQPLHash parses the given string as live content part hash
func (f *factory) ParseQPLHash(s string) (QPHash, error) {
	h, err := hash.FromString(s)
	if err != nil {
		return nil, err
	} else if h.IsNil() {
		return nil, nil
	}
	switch h.Type.Code {
	case hash.QPartLive, hash.QPartLiveTransient:
		return h, nil
	case hash.QPart:
		return nil, errors.E("parse live hash", errors.K.Invalid, "reason", "hash not live", "hash", s)
	default:
		return nil, errors.E("parse live hash", errors.K.Invalid, "reason", "invalid code", "hash", s)
	}
}

// ParseQType parses the string as content type
func (f *factory) ParseQType(s string) (QType, error) {
	return hash.Q.FromString(s)
}

// NewMetadataCodec returns the codec for serializing metadata
func (f *factory) NewMetadataCodec() codecs.MultiCodec {
	return codecs.NewCborCodec()
}

// ParseQSpaceID parses the string as content space ID
func (f *factory) ParseQSpaceID(s string) (QSpaceID, error) {
	return id.QSpace.FromString(s)
}
