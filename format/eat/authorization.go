package eat

import (
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/common-go/util/ethutil"
	"github.com/eluv-io/errors-go"
)

// NewAuthorization returns an authorization from the given token or an error.
// The optional aux is a client confirmation token.
// An error is possibly returned only in the case where the token was not encoded
func NewAuthorization(tok *Token, aux ...*Token) (*Authorization, error) {
	if tok == nil {
		return nil, errors.E("NewAuthorization", errors.K.Invalid, "reason", "token is nil")
	}
	data := tok.TokenData
	// when an embedded exists this is a state channel, hence it has precedence
	if tok.Embedded != nil {
		data = tok.Embedded.TokenData
		if len(data.AFGHPublicKey) == 0 {
			// only take it if not specified by elv master
			data.AFGHPublicKey = tok.AFGHPublicKey
		}
	}

	var cnf *Token
	if len(aux) > 0 {
		cnf = aux[0]
	}
	if cnf != nil {
		// populate data.Cnf from cnf - not required for now
		// see comment in ClientConfirmation
	}

	bearer, err := tok.OriginalBearer()
	if err != nil {
		return nil, err
	}

	return &Authorization{
		Type:      tok.Type, // used in auth, copied as 'token_type' in *Caller
		Bearer:    bearer,   // used live playout and log usage
		TokenData: data,     // used in auth, used as 'token' in CreateEvalContext
	}, nil
}

// Authorization conveys token data after a token has been verified.
type Authorization struct {
	Type      TokenType // the type of the token from which this authorization is made
	Bearer    string    // the 'original' bearer string
	TokenData           // the 'token data' from the token
}

func (a *Authorization) User() string {
	uid := a.UserId()
	if uid != nil {
		return uid.String()
	}
	return a.Subject
}

func (a *Authorization) UserId() types.UserID {
	var uid types.UserID
	switch a.Type {
	case Types.StateChannel(),
		Types.Client(),
		Types.EditorSigned(),
		Types.SignedLink():

		var err error
		uid, err = id.User.FromString(a.Subject)
		if err != nil {
			uid, _ = ethutil.AddrToID(a.Subject, id.User)
		}

	case Types.Tx(), Types.Plain(), Types.Node(), Types.ClientSigned():
		// Types.ClientConfirmation() may have an EthAddr but is not suitable for user id.
		if a.EthAddr != zeroAddr {
			uid = ethutil.AddressToID(a.EthAddr, id.User)
		}
	case Types.Anonymous():
	default:
	}
	return uid
}
