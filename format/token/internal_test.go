package token

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodeFromStringInvalid(t *testing.T) {
	require.Equal(t, len(codeToPrefix), len(codeToName))
	for k, v := range codeToName {
		_, err := Code(k).FromString("invalid-id")
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), v))
		require.True(t, strings.Contains(err.Error(), "invalid-id"))
	}
}

// ClearString is exposed to tests in order to clear 's' the string representation
// of the token
func (t *Token) ClearString() string {
	return t.makeString()
}
