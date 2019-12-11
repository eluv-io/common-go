package link

import (
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/structured"
)

// NewBuilder creates a link builder that can be used to build a link:
//   lnk, err := link.NewBuilder().Target(qhash).Selector(link.S.Meta).P("public", "name").Build()
func NewBuilder() *Builder {
	return &Builder{
		l: emptyLink(),
	}
}

type Builder struct {
	l *Link
}

func (b *Builder) Target(t *hash.Hash) *Builder {
	b.l.Target = t
	return b
}

func (b *Builder) Selector(s Selector) *Builder {
	b.l.Selector = s
	return b
}

func (b *Builder) P(p ...string) *Builder {
	return b.Path(p)
}

func (b *Builder) Path(p structured.Path) *Builder {
	b.l.Path = p
	return b
}

func (b *Builder) Off(off int64) *Builder {
	b.l.Off = off
	return b
}

func (b *Builder) Len(len int64) *Builder {
	b.l.Len = len
	return b
}

func (b *Builder) ReplaceProps(p map[string]interface{}) *Builder {
	b.l.Props = p
	return b
}

func (b *Builder) AddProps(p map[string]interface{}) *Builder {
	if b.l.Props == nil {
		b.l.Props = make(map[string]interface{})
	}
	for key, val := range p {
		b.l.Props[key] = val
	}
	return b
}

func (b *Builder) AddProp(key string, val interface{}) *Builder {
	if b.l.Props == nil {
		b.l.Props = make(map[string]interface{})
	}
	b.l.Props[key] = val
	return b
}

func (b *Builder) Container(qhot string) *Builder {
	b.l.Extra.Container = qhot
	return b
}

func (b *Builder) AutoUpdate(tag string) *Builder {
	b.l.Extra.AutoUpdate = &AutoUpdate{Tag: tag}
	return b
}

func (b *Builder) Build() (*Link, error) {
	err := b.l.Validate(true)
	if err != nil {
		return nil, err
	}
	res := b.l
	b.l = emptyLink()
	return res, nil
}

func (b *Builder) MustBuild() *Link {
	res, err := b.Build()
	if err != nil {
		panic(err)
	}
	return res
}

func emptyLink() *Link {
	return &Link{
		Len: -1,
	}
}
