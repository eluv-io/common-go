package resolvers_test

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/link/resolvers"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/qfab/daemon/model"
	"github.com/qluvio/content-fabric/qfab/daemon/model/options"
	"github.com/qluvio/content-fabric/util/jsonutil"
	"github.com/qluvio/content-fabric/util/maputil"
	"github.com/qluvio/content-fabric/util/stringutil"
)

type testCase struct {
	name         string
	model        func(t *test) string
	noAutoUpdate bool
	wantOrder    []string
	wantDag      map[string][]string
}

func TestGetLinkStatus(t *testing.T) {
	// graph chars: ─ → ↓ ↗ ↘ ↙ │
	tests := []*testCase{
		{
			/*
					 1
				  ↗     ↘
				0 ──────→ 2 ─→ 3'
			*/
			name: "base hierarchy",
			model: func(t *test) string {
				t.generateContents(4, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(2, 0, true)
				t.model.addQ(1, 0).addL(2, 0, true)
				t.model.addQ(2, 0).addL(3, 0, true)
				t.model.addQ(3, 0)
				t.model.addQ(3, 1)
				return t.v(0, 0).String()
			},
			wantOrder: []string{"2.0", "1.0", "0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "2.0"},
				"1.0": {"2.0"},
				"2.0": {"3.0"},
				"3.0": nil,
			},
		},
		{
			/*
					 1
				  ↗     ↘
				0 ──────→ 2 ─→ 3'

				no auto update links!
			*/
			name: "no auto-update links",
			model: func(t *test) string {
				t.generateContents(4, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, false).addL(2, 0, false)
				t.model.addQ(1, 0).addL(2, 0, false)
				t.model.addQ(2, 0).addL(3, 0, false)
				t.model.addQ(3, 0)
				t.model.addQ(3, 1)
				return t.v(0, 0).String()
			},
			wantOrder: nil,
			wantDag: map[string][]string{
				"0.0": nil,
			},
		},
		{
			/*
				0 ───→ 3 ───→ 4'
				  ↘         ↗
					 1 ─→ 2
			*/
			name: "more complex hierarchy",
			model: func(t *test) string {
				t.generateContents(5, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(3, 0, true)
				t.model.addQ(1, 0).addL(2, 0, true)
				t.model.addQ(2, 0).addL(4, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0)
				t.model.addQ(4, 1)
				return t.v(0, 0).String()
			},
			// that's how Kahn works...
			wantOrder: []string{"2.0", "3.0", "1.0", "0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "3.0"},
				"1.0": {"2.0"},
				"2.0": {"4.0"},
				"3.0": {"4.0"},
				"4.0": nil,
			},
		},
		{
			/*
				0 ───→ 3 ───→ 4
				  ↘         ↗
					 1 ─→ 2'
			*/
			name: "update in one branch only",
			model: func(t *test) string {
				t.generateContents(5, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(3, 0, true)
				t.model.addQ(1, 0).addL(2, 0, true)
				t.model.addQ(2, 0).addL(4, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0)
				t.model.addQ(2, 1)
				return t.v(0, 0).String()
			},
			wantOrder: []string{"1.0", "0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "3.0"},
				"1.0": {"2.0"},
				"2.0": {"4.0"},
				"3.0": {"4.0"},
				"4.0": nil,
			},
		},
		{
			/*
				0 ───→ 3' ───→ 4'
				  ↘         ↗
					 1' ─→ 2'
			*/
			name: "all nodes have new version",
			model: func(t *test) string {
				t.generateContents(5, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(3, 0, true)
				t.model.addQ(1, 0).addL(2, 0, true)
				t.model.addQ(2, 0).addL(4, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0)
				t.model.addQ(1, 1)
				t.model.addQ(2, 1)
				t.model.addQ(3, 1)
				t.model.addQ(4, 1)
				return t.v(0, 0).String()
			},
			wantOrder: []string{"2.0", "3.0", "1.0", "0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "3.0"},
				"1.0": {"2.0"},
				"2.0": {"4.0"},
				"3.0": {"4.0"},
				"4.0": nil,
			},
		},
		{
			/*
				0 ───→ 3 ───→ 4
				  ↘         ↗
					 1 ─→ 2
			*/
			name: "all nodes are up-to-date",
			model: func(t *test) string {
				t.generateContents(5, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(3, 0, true)
				t.model.addQ(1, 0).addL(2, 0, true)
				t.model.addQ(2, 0).addL(4, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0)
				return t.v(0, 0).String()
			},
			wantOrder: nil,
			wantDag: map[string][]string{
				"0.0": {"1.0", "3.0"},
				"1.0": {"2.0"},
				"2.0": {"4.0"},
				"3.0": {"4.0"},
				"4.0": nil,
			},
		},
		{
			/*
				0  ─→ 2 ─→ 3 ─→ 4 ─→ 5
				  ↘
					 1'
			*/
			name: "direct child of root needs update",
			model: func(t *test) string {
				t.generateContents(6, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(2, 0, true)
				t.model.addQ(1, 0)
				t.model.addQ(2, 0).addL(3, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0).addL(5, 0, true)
				t.model.addQ(5, 0)
				t.model.addQ(1, 1)
				return t.v(0, 0).String()
			},
			wantOrder: []string{"0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "2.0"},
				"1.0": nil,
				"2.0": {"3.0"},
				"3.0": {"4.0"},
				"4.0": {"5.0"},
				"5.0": nil,
			},
		},
		{
			/*
				0  ─→ 2 ─→ 3' ─→ 4 ─→ 5
				  ↘
					 1
			*/
			name: "node in long chain needs update",
			model: func(t *test) string {
				t.generateContents(6, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(2, 0, true)
				t.model.addQ(1, 0)
				t.model.addQ(2, 0).addL(3, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0).addL(5, 0, true)
				t.model.addQ(5, 0)
				t.model.addQ(3, 1)
				return t.v(0, 0).String()
			},
			wantOrder: []string{"2.0", "0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "2.0"},
				"1.0": nil,
				"2.0": {"3.0"},
				"3.0": {"4.0"},
				"4.0": {"5.0"},
				"5.0": nil,
			},
		},
		{
			/*
				(─→) : no auto-update link

				0  ─→ 2 (─→) 3' ─→ 4 ─→ 5
				  ↘
					 1
			*/
			name: "hierarchy with regular link",
			model: func(t *test) string {
				t.generateContents(6, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(2, 0, true)
				t.model.addQ(1, 0)
				t.model.addQ(2, 0).addL(3, 0, false)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0).addL(5, 0, true)
				t.model.addQ(5, 0)
				t.model.addQ(3, 1)
				return t.v(0, 0).String()
			},
			wantOrder: nil,
			wantDag: map[string][]string{
				"0.0": {"1.0", "2.0"},
				"1.0": nil,
				"2.0": nil,
			},
		},
		{
			/*
				(─→) : no auto-update link

				0  ─→ 2 (─→) 3' ─→ 4 ─→ 5
				  ↘
					 1
			*/
			name: "hierarchy with regular link - asking for full dag",
			model: func(t *test) string {
				t.generateContents(6, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(2, 0, true)
				t.model.addQ(1, 0)
				t.model.addQ(2, 0).addL(3, 0, false)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0).addL(5, 0, true)
				t.model.addQ(5, 0)
				t.model.addQ(3, 1)
				return t.v(0, 0).String()
			},
			noAutoUpdate: true,
			wantOrder:    nil,
			wantDag: map[string][]string{
				"0.0": {"1.0", "2.0"},
				"1.0": nil,
				"2.0": {"3.0"},
				"3.0": {"4.0"},
				"4.0": {"5.0"},
				"5.0": nil,
			},
		},
		{
			/*
				(─→) : no auto-update link


				       ↗ ─→ ↘
				0  ─→ 2 (─→) 3 ─→ 4 ─→ 5'
					  ↘
						 1
			*/
			name: "hierarchy with regular and auto-update link between 2 nodes",
			model: func(t *test) string {
				t.generateContents(6, 2)
				t.newModel()
				t.model.addQ(0, 0).addL(1, 0, true).addL(2, 0, true)
				t.model.addQ(1, 0)
				t.model.addQ(2, 0).addL(3, 0, false).addL(3, 0, true)
				t.model.addQ(3, 0).addL(4, 0, true)
				t.model.addQ(4, 0).addL(5, 0, true)
				t.model.addQ(5, 0)
				t.model.addQ(5, 1)
				return t.v(0, 0).String()
			},
			wantOrder: []string{"4.0", "3.0", "2.0", "0.0"},
			wantDag: map[string][]string{
				"0.0": {"1.0", "2.0"},
				"1.0": nil,
				"2.0": {"3.0"},
				"3.0": {"4.0"},
				"4.0": {"5.0"},
				"5.0": nil,
			},
		},
	}

	for idx, test := range tests {
		t.Run(fmt.Sprintf("test-%d-%s", idx, test.name), func(t *testing.T) {
			newTest(t).TestGetLinkStatus(test)
		})
	}

}

func newTest(t *testing.T) *test {
	return &test{
		T:          t,
		Assertions: require.New(t),
		ff:         format.NewFactory(),
	}
}

type test struct {
	*testing.T
	*require.Assertions
	ff       format.Factory
	qids     []types.QID
	versions map[string][]types.QHash // qid => hash
	model    *qModel                  // hash => meta
}

func (t *test) TestGetLinkStatus(tc *testCase) {
	root := tc.model(t)
	res, err := resolvers.GetLinkStatus(root, !tc.noAutoUpdate, "", &options.SelectOptions{Fields: []string{"version"}}, t)
	t.NoError(err)

	fmt.Println("root object:", root)
	fmt.Println(jsonutil.MarshalString(res))
	t.printUpdateOrderAndDAG(root, res)

	order, dag := t.convertUpdateOrderAndDAG(root, res)
	t.Equal(tc.wantOrder, order)
	t.Equal(tc.wantDag, dag)
}

func (t *test) Meta(qhot string, path structured.Path) (interface{}, error) {
	v, ok := t.model.versions[qhot]
	if !ok {
		return nil, errors.E("provider.Meta", errors.K.NotExist, "reason", "version not found", "qhot", qhot)
	}
	return structured.Get(path, v.meta())
}

func (t *test) GetTaggedVersion(qid id.ID, tag string) (qhash types.QHash, err error) {
	t.Equal("latest", tag)
	c, ok := t.model.contents[qid.String()]
	if !ok {
		return nil, errors.E("provider.GetTaggedVersions", errors.K.NotExist, "reason", "content does not exist", "qid", qid, "tag", tag)
	}
	v, ok := c.tags[tag]
	if !ok {
		return nil, errors.E("provider.GetTaggedVersions", errors.K.NotExist, "reason", "content does not exist", "qid", qid, "tag", tag)
	}
	return v.hash, nil
}

func (t *test) newModel() {
	t.model = &qModel{
		t:        t,
		contents: make(map[string]*content),
		versions: make(map[string]*version),
	}
}

func (t *test) generateContents(contents, versions int) {
	t.qids = make([]types.QID, contents)
	t.versions = make(map[string][]types.QHash)
	for i := 0; i < len(t.qids); i++ {
		t.qids[i] = t.ff.GenerateQID()
		qid := t.qids[i].String()
		for j := 0; j < versions; j++ {
			t.versions[qid] = append(t.versions[qid], t.generateSyntheticHash(t.qids[i], versions*i+j))
		}
	}

}

func (t *test) generateSyntheticHash(qid types.QID, n int) types.QHash {
	base := make([]byte, sha256.Size)
	base[len(base)-1] = byte(n) + 8
	h, err := hash.New(hash.Type{Code: hash.Q, Format: hash.Unencrypted}, base, 1024, qid)
	t.NoError(err)
	return h
}

//func (t *test) vs(qidIndex int, versionIndex int) string {
//	return t.v(qidIndex, versionIndex).String()
//}

func (t *test) v(qidIndex int, versionIndex int) types.QHash {
	return t.versions[t.qids[qidIndex].String()][versionIndex]
}

func (t *test) printUpdateOrderAndDAG(root string, res *model.LinkStatusRes) {

	desc := func(hsh string) string {
		return structured.Wrap(res.Details[hsh].Meta).Get("version").String()
	}

	sb := strings.Builder{}

	sb.WriteString("order: ")
	for _, c := range res.AutoUpdates.Order {
		sb.WriteString(desc(c))
		sb.WriteString(", ")
	}
	sb.WriteString("\n")

	sb.WriteString("dag:\n")

	pending := make([]string, 0)
	pending = append(pending, root)
	for ; len(pending) > 0; pending = pending[1:] {
		current := pending[0]
		sb.WriteString(desc(current))
		sb.WriteString(": ")
		children := res.ObjectDag[current]
		for _, child := range children {
			if _, ok := stringutil.Contains(child, pending); !ok {
				pending = append(pending, child)
			}
			sb.WriteString(desc(child))
			sb.WriteString(", ")
		}
		sb.WriteString("\n")
	}
	fmt.Println(sb.String())
}

func (t *test) convertUpdateOrderAndDAG(root string, res *model.LinkStatusRes) ([]string, map[string][]string) {
	var dag = make(map[string][]string)
	var order []string

	desc := func(hsh string) string {
		return structured.Wrap(res.Details[hsh].Meta).Get("version").String()
	}

	for _, c := range res.AutoUpdates.Order {
		order = append(order, desc(c))
	}

	pending := make([]string, 0)
	pending = append(pending, root)
	for ; len(pending) > 0; pending = pending[1:] {
		current := pending[0]

		children := res.ObjectDag[current]
		var cc []string
		for _, child := range children {
			if _, ok := stringutil.Contains(child, pending); !ok {
				pending = append(pending, child)
			}
			cc = append(cc, desc(child))
		}
		sort.Strings(cc)
		dag[desc(current)] = cc
	}
	return order, dag
}

type qModel struct {
	t        *test
	contents map[string]*content // qid => content
	versions map[string]*version // hash => version
}

func (m *qModel) addQ(qidIndex, versionIndex int) *version {
	qid := m.t.qids[qidIndex].String()
	c := m.contents[qid]
	if c == nil {
		c = &content{
			model:    m,
			qid:      m.t.qids[qidIndex],
			index:    qidIndex,
			versions: make(map[string]*version),
			tags:     make(map[string]*version),
		}
		m.contents[qid] = c
	}

	return c.addV(versionIndex)
}

func (m *qModel) add(qidIndex int, versionIndex int, targets ...types.QHash) {
	hsh := m.t.v(qidIndex, versionIndex)

	// create inter-object links
	var links []interface{}
	for _, target := range targets {
		l := link.NewBuilder().
			Selector(link.S.Meta).
			Target(target).P("version").
			AddProp("auto_update", maputil.From("tag", "latest")).
			MustBuild()
		links = append(links, l)
	}

	// create version
	v := &version{
		hash: hsh,
	}

	// create content if needed
	qid := m.t.qids[qidIndex].String()
	c := m.contents[qid]
	if c == nil {
		c = &content{
			model:    m,
			qid:      m.t.qids[qidIndex],
			index:    qidIndex,
			versions: make(map[string]*version),
			tags:     make(map[string]*version),
		}
		m.contents[qid] = c
	}

	// add version to content
	c.addVersion(v)

	// update model's version map
	m.versions[hsh.String()] = v
}

type content struct {
	model    *qModel
	qid      types.QID
	index    int
	versions map[string]*version // hash => version
	tags     map[string]*version // tag => version
}

func (c *content) addV(versionIndex int) *version {
	hsh := c.model.t.v(c.index, versionIndex)

	v := &version{
		content: c,
		index:   versionIndex,
		hash:    hsh,
	}

	c.versions[v.hash.String()] = v
	c.tags["latest"] = v

	c.model.versions[v.hash.String()] = v

	return v
}

func (c *content) addVersion(v *version) {
	c.versions[v.hash.String()] = v
	c.tags["latest"] = v
}

type version struct {
	content *content
	index   int
	hash    types.QHash
	links   []interface{}
}

func (v *version) addL(qidIndex int, versionIndex int, autoUpdate bool) *version {
	target := v.content.model.t.v(qidIndex, versionIndex)
	lb := link.NewBuilder().
		Selector(link.S.Meta).
		Target(target).P("version")
	if autoUpdate {
		lb.AddProp("auto_update", maputil.From("tag", "latest"))
	}
	v.links = append(v.links, lb.MustBuild())
	return v
}

func (v *version) meta() interface{} {
	return maputil.From("qid", v.hash.ID.String(), "version", fmt.Sprintf("%d.%d", v.content.index, v.index), "links", v.links)
}
