package types

import (
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
)

// ZeroTQID is the zero value of TQID (meaning "no tqid")
var ZeroTQID = TQID{}

func ToTQID(qid id.ID) TQID {
	return TQID{id.Decompose(qid)}
}

func NewTQID(qid QID, tid TenantID) TQID {
	return TQID{id.Compose(id.TQ, qid.Bytes(), tid.Bytes())}
}

// TQID is the type of content IDs with embedded tenant ID
type TQID struct {
	id.Composed
}

func (t TQID) TenantID() TenantID {
	return t.Embedded()
}

func (t TQID) Equal(other TQID) bool {
	return t.Composed.Equal(other.Composed)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ZeroTLID is the zero value of TLID (meaning "no tlid")
var ZeroTLID = TLID{}

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
	return t.Composed.Equal(other.Composed)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type TQHash struct {
	QHash
	tid TenantID
}

func ToTQHash(qhash QHash) TQHash {
	res := TQHash{QHash: qhash}
	if !qhash.IsNil() {
		res.tid = ToTQID(qhash.ID).TenantID()
	}
	return res
}

func (t TQHash) TenantID() TenantID {
	return t.tid
}

func (t TQHash) String() string {
	if t.QHash == nil {
		return ""
	}
	return t.QHash.String()
}

func (t TQHash) MarshalText() ([]byte, error) {
	if t.QHash == nil {
		return nil, nil
	}
	return t.QHash.MarshalText()
}

func (t *TQHash) UnmarshalText(bts []byte) error {
	var qh hash.Hash

	err := qh.UnmarshalText(bts)
	if err != nil {
		return err
	}

	tqh := ToTQHash(&qh)
	*t = tqh
	return nil
}
