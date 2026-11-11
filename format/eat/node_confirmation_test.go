package eat_test

import (
	"crypto/ecdsa"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/eat"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

func TestNodeConfirmation(t *testing.T) {
	now := utc.UnixMilli(1700000000000)
	defer utc.MockNow(now)()

	newKey := func() *ecdsa.PrivateKey {
		pk, err := crypto.GenerateKey()
		require.NoError(t, err)
		return pk
	}
	nodeKey, otherKey, editorKey := newKey(), newKey(), newKey()
	nodeAddr := crypto.PubkeyToAddress(nodeKey.PublicKey)
	isNode := func(addr common.Address) (bool, error) { return addr == nodeAddr, nil }

	// bearer returns a signed-link token as a node receives it
	bearer := func() *eat.Token {
		s := eat.NewSignedLink(id.Generate(id.QSpace), nil, id.Generate(id.Q), "./meta/public", id.Generate(id.Q)).
			Sign(editorKey).
			MustEncode()
		tok, err := eat.FromString(s)
		require.NoError(t, err)
		return tok
	}
	// confirmation returns a node confirmation for tok, signed with pk, as a node receives it
	confirmation := func(pk *ecdsa.PrivateKey, tok *eat.Token) *eat.Token {
		b, err := tok.OriginalBearer()
		require.NoError(t, err)
		s := eat.NewNodeConfirmation(b, 30*time.Second).Sign(pk).MustEncode()
		require.Equal(t, eat.Types.NodeConfirmation().Prefix, s[:3])
		conf, err := eat.FromString(s)
		require.NoError(t, err)
		return conf
	}

	tok := bearer()
	validate := func(conf *eat.Token, isActive func(common.Address) (bool, error)) error {
		return tok.ValidateNodeConfirmation(conf, time.Minute, time.Minute, isActive)
	}

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, validate(confirmation(nodeKey, tok), isNode))
	})

	tests := []struct {
		name     string
		conf     *eat.Token
		isActive func(common.Address) (bool, error)
	}{
		{"missing", nil, isNode},
		{"signed by another key", confirmation(otherKey, tok), isNode},
		{"bound to another token", confirmation(nodeKey, bearer()), isNode},
		{"node lookup fails", confirmation(nodeKey, tok),
			func(common.Address) (bool, error) { return false, errors.E("lookup", errors.K.Unavailable) }},
		{"client confirmation",
			eat.Must(eat.NewClientConfirmation(now).Sign(nodeKey).Token()), isNode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, validate(tt.conf, tt.isActive))
		})
	}

	t.Run("expired", func(t *testing.T) {
		conf := confirmation(nodeKey, tok)
		defer utc.MockNow(now.Add(time.Minute))()
		require.Error(t, validate(conf, isNode))
	})

	t.Run("beyond the validity cap", func(t *testing.T) {
		b, err := tok.OriginalBearer()
		require.NoError(t, err)
		conf, err := eat.FromString(eat.NewNodeConfirmation(b, time.Hour).Sign(nodeKey).MustEncode())
		require.NoError(t, err)
		defer utc.MockNow(now.Add(2 * time.Minute))()
		require.Error(t, validate(conf, isNode))
	})
}

// TestNodeConfirmationTypeBound checks that the node confirmation type is part of the signature: re-prefixed in either
// direction between node confirmation and client confirmation, a token no longer verifies.
func TestNodeConfirmationTypeBound(t *testing.T) {
	pk, err := crypto.GenerateKey()
	require.NoError(t, err)

	verifyAs := func(s string, to eat.TokenType) error {
		tok, err := eat.FromString(to.Prefix + s[3:])
		if err != nil {
			return err
		}
		return tok.VerifySignature()
	}

	nodeConf := eat.NewNodeConfirmation("bearer", 30*time.Second).Sign(pk).MustEncode()
	require.NoError(t, verifyAs(nodeConf, eat.Types.NodeConfirmation()))
	require.Error(t, verifyAs(nodeConf, eat.Types.ClientConfirmation()))

	clientConf := eat.NewClientConfirmation(utc.Now()).
		WithCtx(map[string]any{"elv": map[string]any{"bearer": eat.NodeConfirmationBearerHash("bearer")}}).
		Sign(pk).
		MustEncode()
	require.NoError(t, verifyAs(clientConf, eat.Types.ClientConfirmation()))
	require.Error(t, verifyAs(clientConf, eat.Types.NodeConfirmation()))
}
