package structured

import (
	"context"
	"strings"

	"eluvio/errors"

	"github.com/PaesslerAG/gval"
	"github.com/PaesslerAG/jsonpath"
)

// Query queries the given target data structure.
func Query(target interface{}, query string) (interface{}, error) {
	filter, err := NewFilter(query)
	if err != nil {
		return nil, err
	}
	return filter.Apply(target)
}

type Filter struct {
	separator string
	query     string
	eval      gval.Evaluable
}

func NewFilter(query string) (filter *Filter, err error) {
	filter = &Filter{
		separator: ".",
		query:     query,
	}
	if query != "" && query[0] == '/' {
		filter.separator = "/"
		filter.query = slashToDot(query)
	}
	lang := gval.NewLanguage(gval.Arithmetic(), jsonpath.Language())
	// lang := gval.Full(jsonpath.Language())
	filter.eval, err = lang.NewEvaluable(filter.query)
	return
}

// slashToDot converts a slash separated query string to the '$.' notation:
// $.store.books[3].price
func slashToDot(s string) string {
	if s == "/" {
		return "$"
	}

	p := ParsePath(s, "/")
	sb := strings.Builder{}
	sb.WriteString("$")
	for _, seg := range p {
		if len(seg) == 0 {
			// empty segment: recursive search
			// /path//something --> $.path..something
			sb.WriteString(`.`)
			continue
		}
		if strings.Contains("0123456789-*(?", seg[0:1]) {
			sb.WriteString(`[`)
			sb.WriteString(seg)
			sb.WriteString(`]`)
			continue
		}
		sb.WriteString(`.`)
		for {
			idx := strings.Index(seg, "[")
			if idx == -1 {
				sb.WriteString(seg)
				break
			}
			sb.WriteString(seg[:idx])
			seg = seg[idx:]
			idx = strings.Index(seg, "]")
			if idx == -1 {
				// sb.WriteString(`[`)
				sb.WriteString(seg)
				sb.WriteString(`]`)
				break
			}
			sb.WriteString(seg[:idx+1])
			if idx+1 >= len(seg) {
				break
			}
			seg = seg[idx+1:]
		}
	}
	return sb.String()
}

// the current jsonpath lib (paessler) does not support the JavaScript native
// notation: "$['store']['books'][3]['price']
func slashToDot2(s string) string {
	if s == "/" {
		return "$"
	}

	p := ParsePath(s, "/")
	sb := strings.Builder{}
	sb.WriteString("$")
	for _, seg := range p {
		if strings.Contains("0123456789-*(?", seg[0:1]) {
			sb.WriteString(`[`)
			sb.WriteString(seg)
			sb.WriteString(`]`)
			continue
		}
		sb.WriteString(`['`)
		for {
			idx := strings.Index(seg, "[")
			if idx == -1 {
				sb.WriteString(seg)
				sb.WriteString(`']`)
				break
			}
			sb.WriteString(seg[:idx])
			sb.WriteString(`']`)
			seg = seg[idx:]
			idx = strings.Index(seg, "]")
			if idx == -1 {
				// sb.WriteString(`[`)
				sb.WriteString(seg)
				sb.WriteString(`]`)
				break
			}
			sb.WriteString(seg[:idx+1])
			if idx+1 >= len(seg) {
				break
			}
			seg = seg[idx+1:]
		}
	}
	return sb.String()
}

func slashToDot3(s string) string {
	return "$" + strings.Replace(s, "/", ".", -1)
}

// Query returns the filter's query in 'native' form.
func (f *Filter) Query() string {
	return f.query
}

func (f *Filter) ApplyAndFlatten(structure interface{}) ([][3]string, error) {
	filtered, err := f.Apply(structure)
	if err != nil {
		return nil, err
	}
	return Flatten(filtered, f.separator)
}

func (f *Filter) Apply(structure interface{}) (interface{}, error) {
	filtered, err := f.eval(context.Background(), structure)
	if err != nil {
		return nil, errors.E("filter structure", err, "query", f.query)
	}
	return filtered, nil
}

// CombinePathQuery prefixes the given query with the provided path.
func CombinePathQuery(path, query string) string {
	if strings.HasPrefix(query, "/") {
		if strings.HasSuffix(path, "/") {
			return path + query[1:]
		}
		return path + query
	}
	if strings.HasSuffix(path, "/") {
		return path + query
	}
	return path + "/" + query
}
