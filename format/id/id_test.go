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

	assert.True(t, generated.Equal(idFromString))
	assert.False(t, generated.Equal(nil))
	var nilID ID
	//noinspection GoNilness
	assert.False(t, nilID.Equal(generated))
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

func TestEquivalent(t *testing.T) {
	id1 := Generate(User)                                      // iusr7zNaN4pwUHNuCHDpawHLEz
	id2 := append(Generate(QNode)[:codeLen], id1[codeLen:]...) // inod7zNaN4pwUHNuCHDpawHLEz

	require.True(t, id1.Equivalent(id1))
	require.True(t, id1.Equivalent(id2))
	require.True(t, id2.Equivalent(id1))
}

func ExampleID_Explain() {
	qid := NewID(Q, []byte{1, 2, 3, 4})
	tid := NewID(Tenant, []byte{99})
	fmt.Println(qid.Explain())
	fmt.Println(tid.Explain())

	composed := Embed(qid, tid)
	fmt.Println(composed.Explain())
	fmt.Println(composed.ID().Explain())

	// Output:
	//
	// iq__2VfUX content 0x01020304 (4 bytes)
	// iten2i tenant 0x63 (1 bytes)
	// itq_h42CL8T content with embedded tenant 0x016301020304 (6 bytes)
	//   primary : iq__2VfUX content 0x01020304 (4 bytes)
	//   embedded: iten2i tenant 0x63 (1 bytes)
	// itq_h42CL8T content with embedded tenant 0x016301020304 (6 bytes)
	//   primary : iq__2VfUX content 0x01020304 (4 bytes)
	//   embedded: iten2i tenant 0x63 (1 bytes)
}

func TestIsCompatible(t *testing.T) {
	tests := []struct {
		c1         Code
		c2         Code
		compatible bool
	}{
		{Q, Q, true},
		{Q, TQ, true},
		{QLib, QLib, true},
		{QLib, TLib, true},
		{Tenant, Tenant, true},
		{Q, QLib, false},
		{Q, TLib, false},
		{Q, Tenant, false},
		{TLib, QNode, false},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.c1, test.c2, test.compatible), func(t *testing.T) {
			require.Equal(t, test.compatible, test.c1.IsCompatible(test.c2))
			require.Equal(t, test.compatible, test.c2.IsCompatible(test.c1))
		})
	}
}
