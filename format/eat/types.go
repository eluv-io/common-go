package eat

import (
	"github.com/eluv-io/errors-go"
)

// Types defines the different types of the auth tokens
const Types enumType = 0

type TokenType = *tokenType

type tokenType struct {
	Prefix                 string `json:"prefix"`
	Name                   string `json:"name"`
	SignatureRequired      bool   `json:"-"`
	MayRequireConfirmation bool   `json:"-"`
}

func (t *tokenType) String() string {
	return t.Name
}

func (t *tokenType) Validate() error {
	e := errors.Template("validate token type", errors.K.Invalid)
	if t == nil {
		return e("reason", "type is nil")
	}
	if t == Types.Unknown() {
		return e("type", t.Name)
	}
	return nil
}

var allTypes = []TokenType{
	{"aun", "unknown", false, false},      // 0
	{"aan", "anonymous", false, false},    // 1
	{"atx", "tx", true, false},            // 2
	{"asc", "state-channel", true, false}, // 3
	{"acl", "client", false, false},       // 4
	{"apl", "plain", true, false},         // 5
	{"aes", "editor-signed", true, true},  // 6
	{"ano", "node", true, false},          // 7
	{"asl", "signed-link", true, false},   // 8
	{"acs", "client-signed", true, true},  // 9
	{"acc", "confirmation", true, false},  // 10
}

type enumType int

// PENDING(LUK): review token types!
// 				 Should Anonymous and Plain be folded into one type (Plain), and
//				 then be differentiated by looking at SigTypes.Unsigned()?

func (enumType) Unknown() TokenType            { return allTypes[0] }
func (enumType) Anonymous() TokenType          { return allTypes[1] }  // a vanilla, unsigned token without tx
func (enumType) Tx() TokenType                 { return allTypes[2] }  // based on a blockchain transaction - aka EthAuthToken
func (enumType) StateChannel() TokenType       { return allTypes[3] }  // based on deferred blockchain tx - aka ElvAuthToken
func (enumType) Client() TokenType             { return allTypes[4] }  // a state channel token embedded in a client token - aka ElvClientToken
func (enumType) Plain() TokenType              { return allTypes[5] }  // a vanilla (signed) token without tx ==> blockchain-based permissions via HasAccess()
func (enumType) EditorSigned() TokenType       { return allTypes[6] }  // a token signed by a user who has edit access to the target content in the token
func (enumType) Node() TokenType               { return allTypes[7] }  // token for node-to-node communication
func (enumType) SignedLink() TokenType         { return allTypes[8] }  // token for signed-links (https://github.com/qluvio/proj-mgm/issues/14#issuecomment-724867064)
func (enumType) ClientSigned() TokenType       { return allTypes[9] }  // client-signed token
func (enumType) ClientConfirmation() TokenType { return allTypes[10] } // client-confirmation token (can be required by client-signed or editor-signed)

var prefixToType = map[string]*tokenType{}

func init() {
	for _, c := range allTypes {
		prefixToType[c.Prefix] = c
		if len(c.Prefix) != 3 {
			panic(errors.E("invalid type prefix definition",
				"type", c.Name,
				"prefix", c.Prefix))
		}
	}
}
