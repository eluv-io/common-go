package stackutil

import (
	"io"
	"text/template"

	"github.com/maruel/panicparse/stack"

	"github.com/qluvio/content-fabric/format/utc"
)

const textTmpl = `
{{- define "RenderCall" -}}
{{printf "\n\t%-*s" srcLineLen .SrcLine}} {{.Func.PkgDotName}}({{.Args}})
{{- end -}}

{{- define "RenderCallNoSpace" -}}
{{.SrcLine}} {{.Func.PkgDotName}}({{.Args}})
{{- end -}}

** Aggregate Stacktrace **

generated on:     {{.Now.String}}
aggregation mode: {{.Similarity}}
go-routines:      {{.GoroutineCount}}

{{range .Buckets}}
	{{- len .IDs}}: {{.State}}
	{{- if .SleepMax -}}
		{{- if ne .SleepMin .SleepMax}} [{{.SleepMin}}~{{.SleepMax}} minutes]
		{{- else}} [{{.SleepMax}} minutes]
		{{- end -}}
	{{- end}}
	{{- if .Locked}} [locked]
	{{- end -}}
	{{- if .CreatedBy.SrcPath}} [Created by {{template "RenderCallNoSpace" .CreatedBy}}]
	{{- end -}}
	{{- range .Signature.Stack.Calls}}
		{{- template "RenderCall" .}}
	{{- end}}
	{{if .Stack.Elided}}(...){{end}}
{{end}}
`

func (a *AggregateStack) writeAsText(out io.Writer) error {
	goroutineCount, srcLineLen, _ := a.calcLengths(false)
	m := template.FuncMap{
		"srcLineLen": func() int { return srcLineLen },
	}
	t, err := template.New("textTmpl").Funcs(m).Parse(textTmpl)
	if err != nil {
		return err
	}
	data := struct {
		Buckets        []*stack.Bucket
		Now            utc.UTC
		SrcLineSize    int
		GoroutineCount int
		Similarity     string
	}{a.buckets, utc.Now(), srcLineLen, goroutineCount, a.SimilarityString()}

	return t.Execute(out, data)
}

// CalcLengths returns the maximum length of the source lines and package names.
func (a *AggregateStack) calcLengths(fullPath bool) (int, int, int) {
	goroutineCount := 0
	srcLen := 0
	pkgLen := 0
	for _, bucket := range a.buckets {
		goroutineCount += len(bucket.IDs)
		for _, line := range bucket.Signature.Stack.Calls {
			l := 0
			if fullPath {
				l = len(line.FullSrcLine())
			} else {
				l = len(line.SrcLine())
			}
			if l > srcLen {
				srcLen = l
			}
			l = len(line.Func.PkgName())
			if l > pkgLen {
				pkgLen = l
			}
		}
	}
	return goroutineCount, srcLen, pkgLen
}
