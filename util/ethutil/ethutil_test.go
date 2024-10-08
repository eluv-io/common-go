package ethutil

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/keys"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestAddrXXID(t *testing.T) {
	a := "0x90f8bf6a479f320ead074411a4b0e7944ea8c9c1"
	ispc := "ispc329GX6UVyuWzwPzqDHm5shxfNgrc"
	ispcId, err := id.FromString(ispc)
	require.NoError(t, err)

	idx, err := AddrToID(a, id.QSpace)
	require.NoError(t, err)

	require.Equal(t, ispcId, idx)

	ok, err := AddrEqualsID(a, id.QSpace, ispc)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = AddrEqualsID(a, id.QSpace, "ispc329GX6UVyuWzwPzqDHm5shxfNgr")
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = AddrEqualsID("0x90f8bf6a479f320ead074411a4b0e7944ea8c9cz",
		id.QSpace, ispc)
	require.Error(t, err)

}

func TestAddrToFromID(t *testing.T) {

	ispc := "ispc329GX6UVyuWzwPzqDHm5shxfNgrc"
	ispcId, err := id.FromString(ispc)
	require.NoError(t, err)

	addr0, err := IDStringToAddress(ispc)
	require.NoError(t, err)
	addr := IDToAddress(ispcId)
	require.EqualValues(t, addr0, addr)

	id1, err := AddrToID(addr.String(), id.QSpace)
	require.Equal(t, ispcId, id1)

	id2 := AddressToID(addr, id.QSpace)
	require.Equal(t, ispcId, id2)

	addr = IDToAddress(nil)
	require.Equal(t, common.Address{}, addr)
}

func TestNullKMSAddress(t *testing.T) {
	nullAddress := common.BigToAddress(big.NewInt(0))

	nullId := id.NewID(id.KMS, nullAddress.Bytes())
	nullIdStr := nullId.String()
	require.Equal(t, nullIdStr, "ikms11111111111111111111")

	nullId = id.NewID(id.KMS, common.Address{}.Bytes())
	nullIdStr = nullId.String()
	require.Equal(t, nullIdStr, "ikms11111111111111111111")
}

func TestAddressEqualsID(t *testing.T) {
	var nid id.ID
	b := AddressEqualsID(common.Address{}, nid)
	require.False(t, b)

	bb := make([]byte, 20)
	rand.Read(bb)

	addr := common.BytesToAddress(bb)
	nid = id.NewID(id.Q, bb)
	b = AddressEqualsID(addr, nid)
	require.True(t, b)
}

func TestToPublicKeyAndID(t *testing.T) {
	k, err := crypto.GenerateKey()
	require.NoError(t, err)

	compressedPubBytes := crypto.CompressPubkey(&k.PublicKey)
	compressedPubString := hexutil.Encode(compressedPubBytes)
	//fmt.Println("compressedPubString", compressedPubString)
	compDec, err := hexutil.Decode(compressedPubString)
	require.NoError(t, err)
	decCompPub, err := crypto.DecompressPubkey(compDec)
	require.NoError(t, err)
	require.Equal(t, &k.PublicKey, decCompPub)

	kid, nid := ToPublicKeyAndID(k, id.User)
	kid2, err := keys.UserPublicKey.FromString(kid.String())
	require.NoError(t, err)
	decPub, err := crypto.DecompressPubkey(kid2.Bytes())
	require.NoError(t, err)
	require.Equal(t, &k.PublicKey, decPub)
	require.Equal(t, crypto.PubkeyToAddress(*decPub), IDToAddress(nid))
	fmt.Println("user_id", nid)
	fmt.Println("address", crypto.PubkeyToAddress(*decPub).String())
}

func TestHexToAddress(t *testing.T) {
	type testCase struct {
		val     string
		expFail bool
	}
	for _, tcase := range []*testCase{
		{val: "0x65419c9f653703ed7fb6cc636cf9fda6cc024e2e"},
		{val: "65419c9f653703ed7fb6cc636cf9fda6cc024e2e"},
		{val: ""},
		{val: "bob@example.com", expFail: true},
		{val: "foo", expFail: true},
	} {
		address, err := HexToAddress(tcase.val)
		if tcase.expFail {
			require.Error(t, err, "val [%s]", tcase.val)
			continue
		}
		require.NoError(t, err, "val [%s]", tcase.val)
		exp := common.HexToAddress(tcase.val)
		require.Equal(t, exp, address)
		if len(tcase.val) == 0 {
			zeroAddr := common.Address{}
			require.Equal(t, zeroAddr, address)
		}
	}

}
