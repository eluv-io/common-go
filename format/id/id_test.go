package id

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tid = ID(append([]byte{1}, []byte{0, 1, 2, 3, 4, 5, 6}...))

const expIDString = "iacc1W7LcTy7"

func TestGenerate(t *testing.T) {
	generated := Generate(User)
	assert.NoError(t, generated.AssertCode(User))

	idString := generated.String()
	assert.Equal(t, "iusr", idString[:4])

	idFromString, err := FromString(idString)
	assert.NoError(t, err)
	assert.NoError(t, idFromString.AssertCode(User))

	assert.Equal(t, generated, idFromString)
}

func TestStringConversion(t *testing.T) {
	idString := tid.String()
	assert.Equal(t, expIDString, idString)

	idFromString, err := FromString(idString)
	assert.NoError(t, err)
	assert.NoError(t, idFromString.AssertCode(Account))

	assert.Equal(t, tid, idFromString)
	assert.Equal(t, idString, fmt.Sprint(tid))
	assert.Equal(t, idString, fmt.Sprintf("%v", tid))
	assert.Equal(t, "blub"+idString, fmt.Sprintf("blub%s", tid))
}

func TestInvalidStringConversions(t *testing.T) {
	tests := []struct {
		id string
	}{
		{id: ""},
		{id: "blub"},
		{id: "ilib"},
		{id: "ilib00001111"},
		{id: "ilib "},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			id, err := FromString(test.id)
			assert.Error(t, err)
			assert.Nil(t, id)
		})
	}
}

func TestCodeFromStringInvalid(t *testing.T) {
	require.Equal(t, len(codeToPrefix), len(codeToName))
	for k, v := range codeToName {
		_, err := Code(k).FromString("invalid-id")
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), v))
		require.True(t, strings.Contains(err.Error(), "invalid-id"))
	}
}

func TestJSON(t *testing.T) {
	b, err := json.Marshal(tid)
	assert.NoError(t, err)
	assert.Equal(t, "\""+expIDString+"\"", string(b))

	var unmarshalled ID
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, tid, unmarshalled)
}

type Wrapper struct {
	ID ID
}

func TestWrappedJSON(t *testing.T) {
	s := Wrapper{
		ID: tid,
	}
	b, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), expIDString)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, s, unmarshalled)
}

func TestEqualsFromString(t *testing.T) {
	s, err := FormatId("abcde0", QSpace)
	require.NoError(t, err)
	id1, err := FromString(s)
	require.NoError(t, err)

	//ispczi1u
	require.True(t, id1.Is(id1.String()))
	s2 := "ispcZi1U"
	_, err = FromString(s2)
	require.NoError(t, err)
	require.False(t, id1.Is(s2))
}
