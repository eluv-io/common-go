package eat

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/qluvio/content-fabric/constants"
	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/sign"
	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/ethutil"
)

func (t *Token) encodeLegacy() (s string, err error) {
	legData := TokenDataLegacy{}
	legData.CopyFromTokenData(t)

	var data []byte
	data, err = json.Marshal(&legData)
	if err != nil {
		return "", errors.E("encode legacy token", errors.K.Invalid, err)
	}
	s = base64.StdEncoding.EncodeToString(data)
	if t.SigType == SigTypes.ES256K() {
		s = s + "." + base64.StdEncoding.EncodeToString([]byte(t.Signature.String()))
	}
	t.encoded = s
	return s, nil
}

func (t *Token) encodeLegacySigBytes() ([]byte, error) {
	ethAddr := ""
	if t.EthAddr != zeroAddr {
		ethAddr = t.EthAddr.Hex()
	} else if t.Subject != "" {
		ethAddr = t.Subject
	}
	if len(t.IPGeo()) == 0 {
		// maintains compatibility with older elv-master tokens that do not have
		// the IPGeo condition.
		type eat struct {
			QSpaceID   types.QSpaceID
			QLibID     types.QLibID
			EthAddr    string
			QID        types.QID
			GrantType  string
			TxRequired bool
			Expires    *big.Int
			IssuedAt   *big.Int
		}
		seat := &eat{
			QSpaceID:   t.SID,
			QLibID:     t.LID,
			EthAddr:    ethAddr,
			QID:        t.QID,
			GrantType:  string(t.Grant),
			TxRequired: false,
			Expires:    big.NewInt(t.Expires.Unix()),
			IssuedAt:   big.NewInt(t.IssuedAt.Unix()),
		}
		return rlp.EncodeToBytes(seat)
	}
	type eat struct {
		QSpaceID   types.QSpaceID
		QLibID     types.QLibID
		EthAddr    string
		QID        types.QID
		GrantType  string
		TxRequired bool
		Expires    *big.Int
		IssuedAt   *big.Int
		IPGeo      string
	}

	seat := &eat{
		QSpaceID:   t.SID,
		QLibID:     t.LID,
		EthAddr:    ethAddr,
		QID:        t.QID,
		GrantType:  string(t.Grant),
		TxRequired: false,
		Expires:    big.NewInt(t.Expires.Unix()),
		IssuedAt:   big.NewInt(t.IssuedAt.Unix()),
		IPGeo:      t.IPGeo(),
	}
	return rlp.EncodeToBytes(seat)
}

func (t *Token) decodeLegacyString(s string) (err error) {
	e := errors.Template("decode legacy auth token", errors.K.Invalid, "token_string", s)

	ts := strings.SplitN(s, ".", 2) // split token & signature
	hasSignature := len(ts) > 1

	// parse token
	var tokenBytes []byte
	tokenBytes, err = base64.StdEncoding.DecodeString(ts[0])
	t.encDetails.uncompressedTokenData = tokenBytes
	t.encDetails.uncompressedTokenDataLen = len(tokenBytes)

	legData := NewTokenDataLegacy()
	if err == nil {
		err = json.Unmarshal(tokenBytes, legData)
	}
	if err != nil {
		return e(err)
	}

	if legData.Tok != "" && legData.QID.IsValid() {
		// Backwards-compatibility for OTP API - see TokenDataLegacy
		return e.IfNotNil(t.Decode(legData.Tok))
	}

	if hasSignature {
		err = t.decodeLegacySignature(ts[1])
		if err != nil {
			return e(err)
		}
		t.TokenBytes = []byte(ts[0])
	} else {
		t.SigType = SigTypes.Unsigned()
	}

	t.encoded = s

	if isElvClientToken(legData, hasSignature) {
		// legacy client token was a single token with all data squashed
		// into it => split into two and embed one in the other
		sct := New(Types.StateChannel(), Formats.Legacy(), SigTypes.Unsigned())
		legData.CopyToTokenData(sct, sct.Type)
		if !legData.AuthSig.IsNil() {
			sct.SigType = SigTypes.ES256K()
			sct.Signature = legData.AuthSig
			sct.TokenBytes, _ = sct.encodeLegacySigBytes()

			if hasSignature {
				tokenSigAddr, err := t.Signature.SignerAddress(t.TokenBytes)
				if err != nil {
					return e(err)
				}
				embeddedSigAddr, err := sct.Signature.SignerAddress(sct.TokenBytes)
				if err != nil {
					return e(err)
				}
				if tokenSigAddr == embeddedSigAddr {
					// this is an editor-signed token!
					// keep t (and discard sct)
					t.Type = Types.EditorSigned()
					legData.CopyToTokenData(t, t.Type)
					return nil
				}
			}
		}

		t.TokenData = TokenData{
			Ctx: map[string]interface{}{},
		}
		t.AFGHPublicKey = legData.AFGHPublicKey
		t.embedLegacyStateChannelToken(sct)
		t.Type = Types.Client()
		return nil
	}

	// at this point we don't know the exact token type. Use plain, then
	// determine from additional data copied from teh legacy data
	legData.CopyToTokenData(t, Types.Plain())

	switch {
	case !hasSignature:
		t.Type = Types.Anonymous()
		if len(t.AFGHPublicKey) > 0 {
			return errors.E("decodeLegacyString", errors.K.Invalid,
				"reason", "anonymous token may not have AFGH key")
		}
	case t.HasEthTxHash():
		t.Type = Types.Tx()
	case t.QPHash != nil:
		t.Type = Types.Node()
	default:
		t.Type = Types.Plain()
	}
	if hasSignature && t.EthAddr == zeroAddr {
		return errors.E("decodeLegacyString", errors.K.Invalid,
			"reason", "invalid ethereum address",
			"addr", legData.EthAddr)
	}

	return nil
}

func (t *Token) embedLegacyStateChannelToken(sct *Token) {
	t.Embedded = sct
	// SID and LID are required on all tokens, so copy over to client token...
	t.SID = sct.SID
	t.LID = sct.LID
}

func NewTokenDataLegacy() *TokenDataLegacy {
	return &TokenDataLegacy{}
}

func (t *Token) decodeLegacySignature(sig string) (err error) {
	t.SigType = SigTypes.ES256K()
	var sigBytes []byte
	sigBytes, err = base64.StdEncoding.DecodeString(sig)
	if err == nil {
		t.Signature, err = sign.ES256K.FromString(string(sigBytes))
	}
	if err != nil {
		return errors.E("decode legacy token signature", errors.K.Invalid, err)
	}
	return nil
}

func isElvClientToken(t *TokenDataLegacy, hasSignature bool) bool {
	// required fields for state-channel token (created by elvmaster)
	if t.QSpaceID.IsNil() ||
		t.QLibID.IsNil() ||
		len(t.EthAddr) == 0 ||
		len(t.GrantType) == 0 ||
		t.Expires == 0 ||
		t.AuthSig.IsNil() {
		return false
	}

	if hasSignature {
		// if signed, we also require an afgh pk
		// return len(t.AFGHPublicKey) > 0

		return true
	}

	// if unsigned, we only accept it if the user address is not a valid eth
	// addr (e.g. a user email instead) and there is no afgh pk
	return !common.IsHexAddress(t.EthTxHash) && len(t.AFGHPublicKey) == 0
}

type TokenDataLegacy struct {
	// Client-provided
	EthAddr       string `json:"addr"`             // ethereum address of the API client
	EthTxHash     string `json:"tx_id,omitempty"`  // ethereum transaction ID (hash actually)
	QPHash        string `json:"qphash,omitempty"` // qpart hash for node 2 node
	AFGHPublicKey string `json:"afgh_pk"`          // AFGH public key

	// Common
	QSpaceID types.QSpaceID `json:"qspace_id"`         // space ID
	QLibID   types.QLibID   `json:"qlib_id,omitempty"` // lib ID

	// ElvAuthToken ==> elvmaster
	QID        types.QID              `json:"qid,omitempty"`      // content ID
	GrantType  string                 `json:"grant,omitempty"`    //
	IssuedAt   int64                  `json:"iat"`                // Issued At in seconds from 1970-01-01T00:00:00Z
	Expires    int64                  `json:"exp"`                // Expiration Time in seconds from 1970-01-01T00:00:00Z
	IPGeo      string                 `json:"ip_geo,omitempty"`   // comma separated ip geo conditions to match or empty if no such condition
	Ctx        map[string]interface{} `json:"ctx,omitempty"`      // context keys: only string or []string values! (until cbor serialization is used)
	TxRequired bool                   `json:"tx_required"`        // set to true to require full blochain (tx_id in client token)
	AuthSig    sign.Sig               `json:"auth_sig,omitempty"` // signature sign.ES256K

	// Backwards-compatibility for OTP API: wraps a new token and a qid in json
	// structure as follows:
	//	{
	//  	"tok": "NEW FORMAT",
	//  	"qid": "iq__..."
	//	}
	Tok string `json:"tok,omitempty"`
}

// CopyToTokenData copies the data from this TokenDataLegacy to the provided
// TokenData.
func (l *TokenDataLegacy) CopyToTokenData(t *Token, typ TokenType) {
	t.TokenData = TokenData{}
	t.TokenData.Ctx = map[string]interface{}{}

	t.AFGHPublicKey = l.AFGHPublicKey
	t.SID = l.QSpaceID
	t.LID = l.QLibID
	t.QID = l.QID
	t.Grant = Grant(l.GrantType)
	if l.IssuedAt != 0 {
		t.IssuedAt = utc.Unix(l.IssuedAt, 0)
	}
	if l.Expires != 0 {
		t.Expires = utc.Unix(l.Expires, 0)
	}
	t.Ctx = l.Ctx
	switch typ {
	case Types.StateChannel(), Types.EditorSigned():
		t.Subject = l.EthAddr
	default:
		// ignore the error at this point
		t.EthAddr, _ = ethutil.HexToAddress(l.EthAddr)
	}
	t.EthTxHash = common.HexToHash(l.EthTxHash)
	t.QPHash, _ = hash.FromString(l.QPHash)
	if l.IPGeo != "" {
		if t.Ctx == nil {
			t.Ctx = map[string]interface{}{}
		}
		t.Ctx[constants.ElvIPGeo] = l.IPGeo
	}
}

// CopyFromTokenData copies the data from the provided TokenData to this
// TokenDataLegacy.
func (l *TokenDataLegacy) CopyFromTokenData(t *Token) {
	if t.Type == Types.Client() {
		l.CopyFromTokenData(t.Embedded)
		l.AFGHPublicKey = t.AFGHPublicKey
		return
	}

	l.AFGHPublicKey = t.AFGHPublicKey
	l.QSpaceID = t.SID
	l.QLibID = t.LID
	l.QID = t.QID
	l.GrantType = string(t.Grant)
	if !t.IssuedAt.IsZero() {
		l.IssuedAt = t.IssuedAt.Unix()
	}
	if !t.Expires.IsZero() {
		l.Expires = t.Expires.Unix()
	}
	l.EthTxHash = t.LegacyTxID()
	l.EthAddr = t.LegacyAddr()
	l.QPHash = t.QPHash.String()
	if !t.Signature.IsNil() {
		l.AuthSig = t.Signature
	}
	l.IPGeo = t.IPGeo()
	for k, v := range t.Ctx {
		if k != constants.ElvIPGeo {
			if l.Ctx == nil {
				l.Ctx = map[string]interface{}{}
			}
			l.Ctx[k] = v
		}
	}
}

func (l *TokenDataLegacy) String() string {
	bts, _ := json.Marshal(l)
	return string(bts)
}

func LegacySign(tok string, pk *ecdsa.PrivateKey) (string, error) {
	hsh := crypto.Keccak256([]byte(tok))
	sigBytes, err := crypto.Sign(hsh, pk)
	if err != nil {
		return "", errors.E("legacy sign", err)
	}

	sig := sign.NewSig(sign.ES256K, sigBytes)
	sig = sign.NewSig(sign.ES256K, sig.EthAdjustBytes())

	return tok + "." + base64.StdEncoding.EncodeToString([]byte(sig.String())), nil
}
