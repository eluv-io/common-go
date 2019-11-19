package resolvers

import (
	"sort"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/format/types"
	"github.com/qluvio/content-fabric/qfab/daemon/model"
	"github.com/qluvio/content-fabric/qfab/daemon/model/options"
	"github.com/qluvio/content-fabric/util/stringutil"
)

// GetLinkStatus analyzes the object hierarchy for the given content object
// based on inter-object links. The result includes the full dag (directed
// acyclic graph) of the content objects defined through the links.
//
// If autoUpdate is true, only auto-update links are analyzed, and all outdated
// links are returned in the result together with the update order required for
// a full hierarchy update. The provided updateTag is the target version of each
// each auto-update link - currently only "latest" is supported.
//
// The provided "select" options are used to restrict the metadata that is
// returned for each content object of the dag.
func GetLinkStatus(qhot string, autoUpdate bool, updateTag string, opt *options.SelectOptions, provider ContentProvider) (*model.LinkStatusRes, error) {
	lu := linkStatusResolver{
		resolverFns:   resolverFns{},
		prov:          provider,
		autoUpdate:    autoUpdate,
		updateTag:     updateTag,
		selectOptions: opt,
		qidtagToHash:  make(map[string]*hash.Hash),
		needsUpdate:   make(map[string]bool),
		res: &model.LinkStatusRes{
			ObjectDag:   make(map[string][]string),
			AutoUpdates: model.AutoUpdates{},
			Details:     make(map[string]*model.LinkStatusResDetails),
		},
	}
	err := lu.getLinkStatus(qhot)
	if err != nil {
		return nil, err
	}
	return lu.res, nil
}

type ContentProvider interface {
	MetaProvider
	GetTaggedVersion(qid id.ID, tag string) (qhash types.QHash, err error)
}

type linkStatusResolver struct {
	resolverFns
	prov          ContentProvider
	autoUpdate    bool
	updateTag     string
	qidtagToHash  map[string]*hash.Hash
	res           *model.LinkStatusRes
	selectOptions *options.SelectOptions
	// set of qhashes that keeps track of all qhashes that need an update
	// (because they have a new version or because one of their children has a
	// new version)
	needsUpdate map[string]bool
}

func (r *linkStatusResolver) getLinkStatus(qhot string) error {
	hasUpdates, err := r.doGetLinkStatus(qhot)
	if err != nil {
		return err
	}
	if !r.autoUpdate || !hasUpdates {
		return nil
	}
	r.res.AutoUpdates.Order, err = r.calculateUpdateOrder(qhot)
	if err != nil {
		return errors.E("GetLinkStatus", errors.K.Invalid, err, "qhot", qhot)
	}
	return nil
}

func (r *linkStatusResolver) doGetLinkStatus(qhot string) (bool, error) {
	var err error
	e := errors.Template("GetLinkStatus", errors.K.Invalid, "qhot", qhot, "auto_update", r.autoUpdate)

	if r.autoUpdate && r.updateTag != "" && r.updateTag != "latest" {
		return false, e(err, "reason", "invalid update tag - only 'latest' is supported", "update_tag", r.updateTag)
	}

	if _, exists := r.res.Details[qhot]; exists {
		// qhot already visited
		return r.needsUpdate[qhot], nil
	}

	meta, err := r.prov.Meta(qhot, nil)
	if err != nil {
		return false, e(err, "reason", "failed to get metadata")
	}
	r.addDetails(r.QID(qhot), qhot, meta)

	var links *foundLinks
	meta, links, err = r.collectLinks(meta, func(lnk *link.Link) (*link.Link, bool) {
		if lnk.IsAbsolute() {
			if lnk.Target.AssertCode(hash.Q) == nil {
				if !r.autoUpdate || r.isAutoUpdate(lnk) {
					return lnk, true
				}
			}
		}
		return lnk, false
	})
	if err != nil {
		return false, e(err, "reason", "failed to collect links")
	}

	contentNeedsUpdate := false
	r.res.ObjectDag[qhot] = make([]string, 0, 5)
	for hsh, al := range links.abs {
		if r.autoUpdate {
			for _, lp := range al.laps {
				linkNeedsUpdate := false
				au, err := r.getAutoUpdate(lp.link)
				if err == nil {
					linkNeedsUpdate, err = r.updateLink(qhot, lp, au)
				}
				if err != nil {
					return false, e(err, "link", lp.link)
				}
				if linkNeedsUpdate {
					contentNeedsUpdate = true
				}
			}
		}
		children := r.res.ObjectDag[qhot]
		if _, ok := stringutil.Contains(hsh, children); !ok {
			childNeedsUpdate := false
			r.res.ObjectDag[qhot] = append(children, hsh)
			childNeedsUpdate, err = r.doGetLinkStatus(hsh)
			if err != nil {
				return false, e(err)
			}
			if childNeedsUpdate {
				contentNeedsUpdate = true
			}
		}
	}
	if contentNeedsUpdate {
		r.needsUpdate[qhot] = true
	}

	return contentNeedsUpdate, nil
}

// updateLink updates the given link by modifying the response object
// accordingly if
// a) it is an auto-update link and
// b) a new version of the target content exists.
//
// Returns true if the link is an update link and needs update, false otherwise.
func (r *linkStatusResolver) updateLink(qhot string, lp *lap, au *AutoUpdate) (bool, error) {
	e := errors.Template("update.link")
	var err error

	tag := r.updateTag
	if tag == "" {
		tag = au.Tag
	}
	if tag == "" {
		tag = "latest"
	} else if tag != "latest" {
		return false, e(errors.K.Invalid, "reason", "invalid auto-update tag - only 'latest' is currently supported")
	}

	key := lp.link.Target.ID.String() + ":" + tag
	hsh := r.qidtagToHash[key]
	if hsh == nil {
		hsh, err = r.prov.GetTaggedVersion(lp.link.Target.ID, tag)
		if err != nil {
			return false, e(err)
		}
		r.qidtagToHash[key] = hsh
	}
	if hsh.Equal(lp.link.Target) {
		// link doesn't require update
		return false, nil
	}

	// create updated link
	newLink := *(lp.link)
	newLink.Target = hsh
	updatedLink := &model.AutoUpdateLinks{
		Hash:    qhot,
		Path:    lp.path.String(),
		Current: lp.link,
		Updated: &newLink,
	}

	// add to result
	r.res.AutoUpdates.Links = append(r.res.AutoUpdates.Links, updatedLink)

	hshs := hsh.String()
	if _, ok := r.res.Details[hshs]; !ok {
		// also retrieve details for updated link target
		meta, err := r.prov.Meta(hshs, nil)
		if err != nil {
			return false, e(err, "reason", "failed to get metadata")
		}
		r.addDetails(hsh.ID.String(), hshs, meta)
	}

	return true, nil
}

func (r *linkStatusResolver) prependString(slice []string, el string) []string {
	slice = append(slice, "")
	copy(slice[1:], slice)
	slice[0] = el
	return slice
}

func (r *linkStatusResolver) addDetails(qid, hash string, meta interface{}) {
	details := &model.LinkStatusResDetails{
		QID:  qid,
		Meta: r.selectOptions.ApplyTo(meta),
	}
	r.res.Details[hash] = details
}

func (r *linkStatusResolver) QID(qhot string) string {
	hsh, err := hash.Q.FromString(qhot)
	if err != nil {
		return ""
	}
	return hsh.ID.String()
}

func (r *linkStatusResolver) calculateUpdateOrder(root string) ([]string, error) {
	order, err := r.calculateTopologicalOrder(root, r.res.ObjectDag)
	if err != nil {
		return nil, err
	}
	// remove objects that need no update
	n := 0
	for _, hsh := range order {
		if r.needsUpdate[hsh] {
			order[n] = hsh
			n++
		}
	}
	order = order[:n]

	// reverse order
	last := len(order) - 1
	for i := 0; i < len(order)/2; i++ {
		order[i], order[last-i] = order[last-i], order[i]
	}
	return order, nil
}

func (r *linkStatusResolver) calculateTopologicalOrder(root string, dag map[string][]string) ([]string, error) {
	// calculates topological order of directed acyclic graph according to
	// Kahn's algorithm

	// calculate incoming edge count for each node
	inEdges := make(map[string]int)
	for n := range dag {
		if dag[n] != nil {
			for _, v := range dag[n] {
				inEdges[v]++
			}
		}
	}

	var queue []string
	if root != "" {
		queue = append(queue, root)
	} else {
		// find nodes with no incoming edges
		for n := range dag {
			if _, ok := inEdges[n]; !ok {
				queue = append(queue, n)
			}
		}
	}

	var order []string
	for len(queue) > 0 {
		var n string
		n = queue[0]
		queue[0] = queue[len(queue)-1]
		queue = queue[:(len(queue) - 1)]
		order = append(order, n)
		// makes the order deterministic - mainly useful for unit tests...
		sort.Strings(dag[n])
		for _, v := range dag[n] {
			inEdges[v]--
			if inEdges[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	for node, count := range inEdges {
		if count > 0 {
			return order, errors.E("TopologicalSort", errors.K.Invalid, "reason", "not a DAG - contains cycles", "node", node)
		}
	}

	return order, nil
}

func (r *linkStatusResolver) getAutoUpdate(lnk *link.Link) (*AutoUpdate, error) {
	val := structured.Wrap(lnk.Props).Get("auto_update")
	if val.IsError() {
		// not an auto-update link
		return nil, nil
	}

	var au AutoUpdate
	err := val.Decode(&au)
	if err != nil {
		return nil, errors.E("link.parse.auto-update", err, "reason", "failed to unmarshal auto_update property")
	}
	return &au, nil
}

func (r *linkStatusResolver) isAutoUpdate(lnk *link.Link) bool {
	au, err := r.getAutoUpdate(lnk)
	return au != nil || err != nil // if err, it's an auto-update link, but it's invalid
}

type AutoUpdate struct {
	Tag string `json:"tag"`
}
