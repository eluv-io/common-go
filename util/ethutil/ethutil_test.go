package ethutil

import (
	"github.com/qluvio/content-fabric/format/id"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"math/big"
	"math/rand"
	"testing"
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

}

func TestNullKMDAddress(t *testing.T) {
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
