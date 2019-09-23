package link_test

import (
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/format"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/util/byteutil"

	"github.com/stretchr/testify/require"
)

func TestLinkBuilder(t *testing.T) {
	tests := []struct {
		builder   *link.Builder
		expString string
		expProps  map[string]interface{}
	}{
		{
			builder:   link.NewBuilder().Selector(link.S.Meta).Path(structured.Path{"public", "description"}),
			expString: "./meta/public/description",
		},
		{
			builder:   link.NewBuilder().Selector(link.S.Meta).P("public", "name"),
			expString: "./meta/public/name",
		},
		{
			builder:   link.NewBuilder().Target(qhash).Selector(link.S.Meta).P("public", "name"),
			expString: fmt.Sprintf("/qfab/%v/meta/public/name", qhash),
		},
		{
			builder:   link.NewBuilder().Target(qphash).Off(20).Len(100),
			expString: fmt.Sprintf("/qfab/%v#20-119", qphash),
		},
		{
			builder: link.NewBuilder().Selector(link.S.Meta).P("props").
				AddProp("k1", "v1").
				AddProps(map[string]interface{}{"k2": "v2"}),
			expString: "./meta/props",
			expProps:  map[string]interface{}{"k1": "v1", "k2": "v2"},
		},
		{
			builder: link.NewBuilder().Selector(link.S.Meta).P("replace_props").
				AddProp("k1", "v1").
				ReplaceProps(map[string]interface{}{"k2": "v2"}),
			expString: "./meta/replace_props",
			expProps:  map[string]interface{}{"k2": "v2"},
		},
	}
	for _, test := range tests {
		t.Run(test.expString, func(t *testing.T) {
			lnk, err := test.builder.Build()
			require.NoError(t, err)
			require.Equal(t, test.expString, lnk.String())
			require.EqualValues(t, test.expProps, lnk.Props)
		})
	}
}

var ff format.Factory
var qhash types.QHash
var qphash types.QPHash

func init() {
	ff = format.NewFactory()
	qhash = randomQHash()
	qphash = randomQPHash()
}

func randomQHash() types.QHash {
	digest := ff.NewContentDigest(hash.Unencrypted, ff.GenerateQID())
	_, _ = digest.Write(byteutil.RandomBytes(10))
	return digest.AsHash()
}

func randomQPHash() types.QPHash {
	digest := ff.NewContentPartDigest(hash.Unencrypted)
	_, _ = digest.Write(byteutil.RandomBytes(10))
	return digest.AsHash()
}
