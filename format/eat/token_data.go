package eat

import (
	"bytes"
	"encoding/binary"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"

	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/common-go/util/ethutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// TokenData is the structure containing the actual token data.
type TokenData struct {
	// Client-provided
	EthTxHash     common.Hash    `json:"txh,omitempty"` // ethereum transaction hash - stored as []byte to enable 'nil'
	EthAddr       common.Address `json:"adr,omitempty"` // ethereum address of the user who signed the token - stored as []byte to enable 'nil'
	AFGHPublicKey string         `json:"apk,omitempty"` // AFGH public key
	QPHash        types.QPHash   `json:"qph,omitempty"` // qpart hash for node 2 node

	// Common
	SID types.QSpaceID `json:"spc,omitempty"` // space ID
	LID types.QLibID   `json:"lib,omitempty"` // lib ID

	// ElvAuthToken ==> elv-master
	QID      types.QID              `json:"qid,omitempty"` // content ID
	Subject  string                 `json:"sub,omitempty"` // the entity the token was granted to
	Grant    Grant                  `json:"gra,omitempty"` // type of grant
	IssuedAt utc.UTC                `json:"iat,omitempty"` // Issued At
	Expires  utc.UTC                `json:"exp,omitempty"` // Expiration Time
	Ctx      map[string]interface{} `json:"ctx,omitempty"` // additional, arbitrary information conveyed in the token
}

// EncodeJSON encodes the token data to JSON in its optimized form.
func (t *TokenData) EncodeJSON() ([]byte, error) {
	return json.Marshal((&serData{}).copyFrom(t))
}

// DecodeJSON decodes the token data from its optimized JSON form.
func (t *TokenData) DecodeJSON(bts []byte) error {
	var data serData
	err := json.Unmarshal(bts, &data)
	if err != nil {
		return err
	}
	data.copyTo(t)
	return nil
}

// EncodeCBOR encodes the token data to CBOR in its optimized form.
func (t *TokenData) EncodeCBOR() ([]byte, error) {
	buf := &bytes.Buffer{}
	err := codecs.NewCborEncoder(buf).Encode((&serData{}).copyFrom(t))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeCBOR decodes the token data from its optimized CBOR form.
func (t *TokenData) DecodeCBOR(bts []byte) error {
	var data serData
	err := codecs.NewCborDecoder(bytes.NewReader(bts)).Decode(&data)
	if err != nil {
		return err
	}
	data.copyTo(t)
	return nil
}

var zeroHash common.Hash
var zeroAddr common.Address

func (t *TokenData) Encode() ([]byte, error) {
	e := errors.Template("Encode")
	enc := newTokenEncoder()

	sd := (&serData{}).copyFrom(t)
	enc.writeBytes(sd.EthTxHash)
	enc.writeBytes(sd.EthAddr)
	enc.writeString(t.AFGHPublicKey)
	enc.writeString(t.QPHash.String())
	enc.writeBytes(t.SID)
	enc.writeBytes(t.LID)
	enc.writeBytes(t.QID)
	enc.writeString(string(t.Grant))
	b, err := t.IssuedAt.MarshalBinary()
	if err != nil {
		return nil, e(err)
	}
	enc.writeBytes(b)
	b, err = t.Expires.MarshalBinary()
	if err != nil {
		return nil, e(err)
	}
	enc.writeBytes(b)

	err = enc.writeCbor(t.Ctx)
	if err != nil {
		return nil, e(err)
	}

	return enc.buf.Bytes(), nil
}

func (t *TokenData) Decode(bts []byte) error {
	e := errors.Template("decode token data")
	dec := newDecoder(bts)

	var b []byte
	var s string

	td := serData{}
	err := dec.readBytes(&td.EthTxHash)
	if err == nil {
		t.EthTxHash = common.BytesToHash(td.EthTxHash)
		err = dec.readBytes(&td.EthAddr)
	}
	if err == nil {
		t.EthAddr = common.BytesToAddress(td.EthAddr)
		err = dec.readString(&t.AFGHPublicKey)
	}
	if err == nil {
		err = dec.readString(&s)
		if err == nil {
			t.QPHash, err = hash.FromString(s)
		}
	}
	if err == nil {
		err = dec.readBytes((*[]byte)(&t.SID))
	}
	if err == nil {
		err = dec.readBytes((*[]byte)(&t.LID))
	}
	if err == nil {
		err = dec.readBytes((*[]byte)(&t.QID))
	}
	if err == nil {
		err = dec.readString(&s)
		if err == nil {
			t.Grant = Grant(s)
		}
	}
	if err == nil {
		err = dec.readBytes(&b)
		if err == nil {
			err = t.IssuedAt.UnmarshalBinary(b)
		}
	}
	if err == nil {
		err = dec.readBytes(&b)
		if err == nil {
			err = t.Expires.UnmarshalBinary(b)
		}
	}
	if err == nil {
		err = dec.readCbor(&t.Ctx)
	}

	return e.IfNotNil(err)
}

func (t *TokenData) IPGeo() string {
	if obj, ok := t.Ctx[ElvIPGeo]; ok {
		return obj.(string)
	}
	return ""
}

func (t *TokenData) Signer() types.UserID {
	if t.EthAddr == zeroAddr {
		return nil
	}
	return ethutil.AddressToID(t.EthAddr, id.User)
}

// -----------------------------------------------------------------------------

// serData is used for JSON/CBOR serialization
type serData struct {
	// Client-provided
	EthTxHash     []byte       `json:"txh,omitempty"` // ethereum transaction hash - stored as []byte to enable 'nil'
	EthAddr       []byte       `json:"adr,omitempty"` // ethereum address of the user - stored as []byte to enable 'nil'
	AFGHPublicKey string       `json:"apk,omitempty"` // AFGH public key
	QPHash        types.QPHash `json:"qph,omitempty"` // qpart hash for node 2 node

	// Common
	SID types.QSpaceID `json:"spc,omitempty"` // space ID
	LID types.QLibID   `json:"lib,omitempty"` // lib ID

	// ElvAuthToken ==> elvmaster
	QID      types.QID              `json:"qid,omitempty"` // content ID
	Subject  string                 `json:"sub,omitempty"` // the entity the token was granted to
	Grant    Grant                  `json:"gra,omitempty"` // type of grant
	IssuedAt int64                  `json:"iat,omitempty"` // Issued At
	Expires  int64                  `json:"exp,omitempty"` // Expiration Time
	Ctx      map[string]interface{} `json:"ctx,omitempty"` // additional, arbitrary information conveyed in the token
}

func (d *serData) copyTo(t *TokenData) *TokenData {
	t.EthTxHash = common.BytesToHash(d.EthTxHash)
	t.EthAddr = common.BytesToAddress(d.EthAddr)
	t.AFGHPublicKey = d.AFGHPublicKey
	t.QPHash = d.QPHash
	t.SID = d.SID
	t.LID = d.LID
	t.QID = d.QID
	t.Subject = d.Subject
	t.Grant = d.Grant
	if d.IssuedAt != 0 {
		t.IssuedAt = utc.UnixMilli(d.IssuedAt)
	}
	if d.Expires != 0 {
		t.Expires = utc.UnixMilli(d.Expires)
	}
	t.Ctx = d.Ctx
	return t
}

func (d *serData) copyFrom(t *TokenData) *serData {
	if t.EthTxHash != zeroHash {
		d.EthTxHash = t.EthTxHash.Bytes()
	}
	if t.EthAddr != zeroAddr {
		d.EthAddr = t.EthAddr.Bytes()
	}
	d.AFGHPublicKey = t.AFGHPublicKey
	d.QPHash = t.QPHash
	d.SID = t.SID
	d.LID = t.LID
	d.QID = t.QID
	d.Subject = t.Subject
	d.Grant = t.Grant
	if !t.IssuedAt.IsZero() {
		d.IssuedAt = t.IssuedAt.UnixMilli()
	}
	if !t.Expires.IsZero() {
		d.Expires = t.Expires.UnixMilli()
	}
	d.Ctx = t.Ctx
	return d
}

// -----------------------------------------------------------------------------

func newTokenEncoder() *tokenEncoder {
	return &tokenEncoder{
		vbuf: make([]byte, binary.MaxVarintLen64),
	}
}

type tokenEncoder struct {
	buf  bytes.Buffer
	vbuf []byte
}

func (e *tokenEncoder) writeString(s string) {
	e.writeBytes([]byte(s))
}

var vi0 = []byte{0}

func (e *tokenEncoder) writeBytes(b []byte) {
	if len(b) == 0 {
		_, _ = e.buf.Write(vi0)
	} else {
		n := binary.PutUvarint(e.vbuf, uint64(len(b)))
		_, _ = e.buf.Write(e.vbuf[:n])
		_, _ = e.buf.Write(b)
	}
}

func (e *tokenEncoder) writeCbor(v interface{}) error {
	return codecs.NewCborEncoder(&e.buf).Encode(v)
}
func newDecoder(b []byte) *tokenDecoder {
	return &tokenDecoder{buf: bytes.NewBuffer(b)}
}

type tokenDecoder struct {
	buf *bytes.Buffer
}

func (e *tokenDecoder) readString(s *string) error {
	var b []byte
	err := e.readBytes(&b)
	if err != nil {
		return err
	}
	if b == nil {
		return nil
	}
	*s = string(b)
	return nil
}

func (e *tokenDecoder) readBytes(b *[]byte) error {
	n, err := binary.ReadUvarint(e.buf)
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	bts := make([]byte, n)
	_, err = e.buf.Read(bts)
	*b = bts
	return err
}

func (e *tokenDecoder) readCbor(v interface{}) error {
	return codecs.NewCborDecoder(e.buf).Decode(v)
}
