package stackutil

import (
	"io"
	"text/template"

	"github.com/maruel/panicparse/v2/stack"

	"github.com/eluv-io/utc-go"
)

const snapshotTextTmpl = `
{{- define "RenderCall" -}}
{{printf "\n\t%-*s" srcLineLen (srcLine .)}} {{.Func.Complete}}({{.Args}})
{{- end -}}

{{- define "RenderCallNoSpace" -}}
{{srcLine .}} {{.Func.Complete}}({{.Args}})
{{- end -}}

** Goroutine Stacktrace **

generated on:     {{.Timestamp.String}}
go-routines:      {{.GoroutineCount}}

{{range .Goroutines}}
gid={{.ID}} {{.State}}
	{{- if .SleepMax -}}
		{{- if ne .SleepMin .SleepMax}} [{{.SleepMin}}~{{.SleepMax}} minutes]
		{{- else}} [{{.SleepMax}} minutes]
		{{- end -}}
	{{- end}}
	{{- if .Locked}} [locked]
	{{- end -}}
	{{- if len .CreatedBy.Calls }} [Created by {{template "RenderCallNoSpace" index .CreatedBy.Calls 0 }}]
	{{- end -}}
	{{- range .Signature.Stack.Calls}}
		{{- template "RenderCall" .}}
	{{- end}}
	{{if .Stack.Elided}}(...){{end}}
{{end}}
`

func (s *Snapshot) writeAsText(out io.Writer) error {
	srcLineLen, _ := s.calcLengths(false)
	m := template.FuncMap{
		"srcLineLen": func() int { return srcLineLen },
		"srcLine":    srcLine,
	}
	t, err := template.New("snapshotTextTmpl").Funcs(m).Parse(snapshotTextTmpl)
	if err != nil {
		return err
	}
	data := struct {
		Goroutines     []*stack.Goroutine
		Timestamp      utc.UTC
		SrcLineSize    int
		GoroutineCount int
	}{s.Goroutines, s.Timestamp, srcLineLen, len(s.Goroutines)}
	return t.Execute(out, data)
}

// CalcLengths returns the maximum length of the source lines and package names.
func (s *Snapshot) calcLengths(fullPath bool) (int, int) {
	srcLen := 0
	pkgLen := 0
	for _, goroutine := range s.Goroutines {
		for _, line := range goroutine.Signature.Stack.Calls {
			l := 0
			if fullPath {
				l = len(fullSrcLine(line))
			} else {
				l = len(srcLine(line))
			}
			if l > srcLen {
				srcLen = l
			}
			l = len(line.Func.DirName)
			if l > pkgLen {
				pkgLen = l
			}
		}
	}
	return srcLen, pkgLen
}
