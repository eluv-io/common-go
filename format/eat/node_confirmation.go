package eat

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// NodeConfirmationBearerHash returns the hash that binds a node confirmation to the bearer token it accompanies: the
// hex-encoded SHA-256 of the bearer token's string.
func NodeConfirmationBearerHash(bearer string) string {
	sum := sha256.Sum256([]byte(bearer))
	return hex.EncodeToString(sum[:])
}

// NodeConfirmationBuilder builds a node confirmation: a node's statement that it forwards the given bearer token. The
// node signs it with its own key.
type NodeConfirmationBuilder struct {
	*signer
}

// NewNodeConfirmation creates a node confirmation for the bearer token, valid for the given duration from now.
func NewNodeConfirmation(bearer string, validity time.Duration) *NodeConfirmationBuilder {
	token := New(Types.NodeConfirmation(), defaultFormat)
	token.IssuedAt = utc.Now()
	token.Expires = token.IssuedAt.Add(validity)
	token.Ctx = map[string]any{"elv": map[string]any{"bearer": NodeConfirmationBearerHash(bearer)}}
	return &NodeConfirmationBuilder{newSigner(token)}
}

func (b *NodeConfirmationBuilder) Sign(pk *ecdsa.PrivateKey) Encoder {
	return b.signer.Sign(pk)
}

// ValidateNodeConfirmation validates the node confirmation conf for this token. The confirmation must be of the node
// confirmation type, within its validity, bound to this token's original bearer string, and signed by an address that
// isActiveNode accepts. The caller decides which tokens need a node confirmation and which addresses are nodes.
func (t *Token) ValidateNodeConfirmation(
	conf *Token,
	maxValidity, timeSkew time.Duration,
	isActiveNode func(addr common.Address) (bool, error)) error {

	e := errors.Template("ValidateNodeConfirmation", errors.K.Permission, "type", t.Type)

	if conf == nil {
		return e("reason", "a node confirmation is required")
	}
	if conf.Type != Types.NodeConfirmation() {
		return e("reason", "invalid node confirmation type", "confirmation_type", conf.Type)
	}
	err := conf.VerifySignature()
	if err != nil {
		return e(err)
	}
	err = conf.VerifyTimes(maxValidity, timeSkew)
	if err != nil {
		return e(err)
	}

	bearer, err := t.OriginalBearer()
	if err != nil {
		return e(err)
	}
	if structured.Wrap(conf.Ctx).Get("elv", "bearer").String() != NodeConfirmationBearerHash(bearer) {
		return e("reason", "node confirmation not bound to the token")
	}

	signer, err := conf.SignerAddress()
	if err != nil {
		return e(err)
	}
	active, err := isActiveNode(signer)
	if err != nil {
		return e(err)
	}
	if !active {
		return e("reason", "node confirmation not signed by an active node", "signer", signer.Hex())
	}
	return nil
}
