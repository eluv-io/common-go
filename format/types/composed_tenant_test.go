package types_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/types"
)

func ExampleTQHash_String() {
	fmt.Println("nil: [" + types.ToTQHash(nil).String() + "]")

	h := hash.MustParse("htq_sSrYCbHxw2ycsSE3k4J9AHgsxBZZ5g1ZYrRDN8ZYZvnQ1wSzPPJTxu3tx2UH7N5wqais1RiceTZ")
	fmt.Println(types.ToTQHash(h).String())

	// Output:
	//
	// nil: []
	// htq_sSrYCbHxw2ycsSE3k4J9AHgsxBZZ5g1ZYrRDN8ZYZvnQ1wSzPPJTxu3tx2UH7N5wqais1RiceTZ
}

func TestTQHash_MarshalJSON(t *testing.T) {
	hashes := []types.TQHash{
		{},
		generateTQHash(),
		generateTQHash(),
	}

	for _, tqh := range hashes {
		t.Run(fmt.Sprint(tqh), func(t *testing.T) {
			bts, err := json.Marshal(tqh)
			require.NoError(t, err)

			fmt.Println(string(bts))

			var res types.TQHash
			err = json.Unmarshal(bts, &res)
			require.NoError(t, err)

			require.Equal(t, tqh.TenantID(), res.TenantID())
		})
	}

}

func generateTQHash() types.TQHash {
	tid := id.Generate(id.Tenant)
	qid := id.Generate(id.Q)
	tqid := types.NewTQID(qid, tid)

	d := hash.NewDigest(sha256.New(), hash.Type{
		Code:   hash.TQ,
		Format: hash.Unencrypted,
	}).WithID(tqid.ID())
	_, _ = d.Write([]byte("blub"))
	qh := d.AsHash()

	tqh := types.ToTQHash(qh)
	return tqh
}
