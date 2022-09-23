package types

import "github.com/eluv-io/common-go/format/id"

func ToTQID(qid id.ID) TQID {
	return TQID{id.Decompose(qid)}
}

func NewTQID(lid QID, tid TenantID) TQID {
	return TQID{id.Compose(id.TQ, lid.Bytes(), tid.Bytes())}
}

// TQID is the type of content IDs with embedded tenant ID
type TQID struct {
	id.Composed
}

func (t TQID) TenantID() TenantID {
	return t.Embedded()
}

func (t TQID) Equal(other TQID) bool {
	return t.ID().Equal(other.ID()) && t.Embedded().Equal(other.Embedded())
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func ToTLID(lid id.ID) TLID {
	return TLID{id.Decompose(lid)}
}

func NewTLID(lid QLibID, tid TenantID) TLID {
	return TLID{id.Compose(id.TLib, lid.Bytes(), tid.Bytes())}
}

// TLID is the type of library IDs with embedded tenant ID
type TLID struct {
	id.Composed
}

func (t TLID) TenantID() TenantID {
	return t.Embedded()
}

func (t TLID) Equal(other TLID) bool {
	return t.ID().Equal(other.ID()) && t.Embedded().Equal(other.Embedded())
}
