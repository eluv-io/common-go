package resolvers

import (
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
)

// resolverFns groups generic link resolution functions.
type resolverFns struct{}

// collectLinks collects links accepted and possibly transformed by the given
// filter. In addition, it replaces all link objects (link.Link) in the generic
// target structure with link pointers (*link.Link).
func (r *resolverFns) collectLinks(target interface{}, filter func(lnk *link.Link) (*link.Link, bool)) (interface{}, *foundLinks, error) {
	links := &foundLinks{
		abs: make(map[string]*absLink),
	}

	target, err := structured.Replace(target, func(path structured.Path, val interface{}) (replace bool, newVal interface{}, err error) {
		var lnk *link.Link
		switch l := val.(type) {
		case *link.Link:
			lnk = l
		case link.Link:
			lnk = &l
		default:
			return false, nil, nil
		}

		if filter != nil {
			var add bool
			lnk, add = filter(lnk)
			if !add {
				return true, lnk, nil
			}
		}

		if lnk.IsAbsolute() {
			hsh := lnk.Target.String()
			al, found := links.abs[hsh]
			if !found {
				al = &absLink{}
			}
			al.laps = append(al.laps, &lap{
				path: path,
				link: lnk,
			})
			al.rootPath = al.rootPath.CommonRoot(lnk.Path)
			links.abs[hsh] = al
		} else {
			links.rel = append(links.rel, &lap{
				path: path,
				link: lnk,
			})
		}
		return true, lnk, nil
	})
	return target, links, err
}
