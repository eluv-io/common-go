package stackutil

import (
	"html/template"
	"io"
	"time"

	"github.com/maruel/panicparse/stack"
)

const htmlTmpl = `<!DOCTYPE html>

{{- define "RenderCall" -}}
<div class="stackline">
	<span class="package">{{.SrcLine}}</span> <span class="{{funcClass .}}">{{.Func.PkgDotName}}</span>({{.Args}})
</div>
{{- end -}}

{{- define "RenderCreated" -}}
{{.SrcLine}} <span class="{{funcClass .}}">{{.Func.PkgDotName}}</span>({{.Args}})
{{- end -}}

<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>PanicParse</title>
<link rel="shortcut icon" type="image/png" href="data:image/png;base64,{{notoColorEmoji1F4A3}}"/>
<style>
	body {
		/* background: black; */
		/* color: lightgray; */
	}
	body, pre {
		font-family: Ubuntu Mono, Menlo, monospace;
		font-size: 14px;
	}
	.FuncStdLibExported {
		color: #008000;
	}
	.FuncStdLib {
		color: #008000;
	}
	.FuncMain {
		color: #C0C000;
	}
	.FuncOtherExported {
		color: #A00000;
	}
	.FuncOther {
		color: #A00000;
	}
	.RoutineFirst {
	}
	.Routine {
		padding-top: 15px;
	}
	.routine-header {
		padding-bottom: 5px;
	}
	.count {
		font-weight: bold;
	}
	.package {
		width: 100px;
	}
	.stackline {
		padding-left: 50px;
		line-height: 130%;
	}
</style>
</head>
<body>
<div id="legend">
Generated on {{.Now.String}}.
</div>
<div id="content">
{{range .Buckets}}
	<div class="Routine">
		<div class="routine-header">
			<span class="count">{{len .IDs}}: </span>
			<span class="state">{{.State}}</span>
			{{if .SleepMax -}}
				{{- if ne .SleepMin .SleepMax}}
					<span class="sleep">[{{.SleepMin}}~{{.SleepMax}} minutes]</span>
				{{- else}}
					<span class="sleep">[{{.SleepMax}} minutes]</span>
				{{- end -}}
			{{- end}}
			{{if .Locked}}
				<span class="locked">[locked]</span>
			{{- end -}}
			{{- if .CreatedBy.SrcPath}}
				<span class="created">[Created by {{template "RenderCreated" .CreatedBy}}]</span>
			{{- end -}}
		</div>
		{{range .Signature.Stack.Calls}}
			{{template "RenderCall" .}}
		{{- end}}
		{{if .Stack.Elided}}(...)<br>{{end}}
	</div>
{{end}}
</div>
</body>
</html>
`

func (a *AggregateStack) writeAsHTML(out io.Writer) error {
	m := template.FuncMap{
		"funcClass":           funcClass,
		"notoColorEmoji1F4A3": notoColorEmoji1F4A3,
	}
	t, err := template.New("htmlTpl").Funcs(m).Parse(htmlTmpl)
	if err != nil {
		return err
	}
	data := struct {
		Buckets []*stack.Bucket
		Now     time.Time
	}{a.buckets, time.Now().Truncate(time.Second)}

	return t.Execute(out, data)
}

func funcClass(line *stack.Call) template.HTML {
	if line.IsStdlib {
		if line.Func.IsExported() {
			return "FuncStdLibExported"
		}
		return "FuncStdLib"
	} else if line.IsPkgMain() {
		return "FuncMain"
	} else if line.Func.IsExported() {
		return "FuncOtherExported"
	}
	return "FuncOther"
}

// notoColorEmoji1F4A3 is the bomb emoji U+1F4A3 in Noto Emoji as a PNG.
//
// Source: https://www.google.com/get/noto/help/emoji/smileys-people.html
// License: http://scripts.sil.org/cms/scripts/page.php?site_id=nrsi&id=OFL
//
// Created with:
//   python -c "import base64;a=base64.b64encode(open('emoji_u1f4a3.png','rb').read()); print '\n'.join(a[i:i+70] for i in range(0,len(a),70))"
func notoColorEmoji1F4A3() template.HTML {
	return "" +
		"iVBORw0KGgoAAAANSUhEUgAAAIgAAACACAMAAADnN9ENAAAA5FBMVEVMaXFvTjQhISEhIS" +
		"EhISEhISEhISEhISEhISEhISEhISEhISHRbBTRbBTRbBQhISHRbBTRbBTRbBTRbBQ6OjrR" +
		"bBTZcBTsehbzfhf1fxfidRVOTk75oiL7uCr90zL3kB3/6zv2hhlHMR5OTk5KSkpKSkpRUV" +
		"FKSkpKSkpMTExBQUE2NjYpKSlOTk5EREQ8PDw4ODgtLS1RUVFQUFAmJiYkJCQ0NDRLS0tT" +
		"U1MoKCgyMjJHR0dTU1NdXV0rKytXV1dVVVUvLy9eXl5aWlpcXFxcXFxeXl5fX186OjphYW" +
		"E/Pz9KSkrdB5CTAAAATHRSTlMAEUZggJjf/6/vcL86e1nP2//vmSC7//////9E////////" +
		"/2WPpYHp////////////////////z/////8w6P////+p/8f///////+/QBb3BQAACWJJRE" +
		"FUeAHslgWC6yAURUOk1N3d3d29+9/SzzxCStvQEfp9zgZycu8DnvTNN78YJCuqqsry77WQ" +
		"NRum2BX0u7JQiYWJw/l7NBz4AderQkFut8frdn+kFBu2wodeYeHxBwjBkFd6StiFOYiboF" +
		"CAxf9MRXHc9KGpqurDBpqghzcYuCOCeMp21oKelTBVCQt5kDiisXhCJ5aMQkFu6+lg4tDY" +
		"r2rikRCPKFgQYlGeiYpN7OHbpMj8OgQ8PAGdWIIlnnwbFKYdtgDAJj+MDgZSX/Zwsx4mby" +
		"YR6RbntRZVegQDX7/tI1YexMTNObQ+y992EUWRQJIJQjqTzWRyRjtRiMTqKtWQJCTCD4TM" +
		"aS6bBzLGxLKRKNer1MELX0wEmYHk8pSMGUmIlMI+LC7uTeEQmhEvnZBC/kqGTolfkmSn72" +
		"NPbFhoWOHswmczeYYC7YaV4MfBHl+BEYmCSJYVSUM3ukgRM5C7g4edqAqLMBq0m1sRh8oe" +
		"Fl4zzp8swtFApXKlmkIkECD8+mpYEZ8iWZGq1YFKKazg582IDiu8bk7Ob5YaOnVCs9UWu+" +
		"C99D7LWR5fVeY/YpVOp9Po9rp1g25/AIGIXmhp0yMLgSTIhcYDVYaj0ag1Hk/a0yZ1qVVK" +
		"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" +
		"SuQmCC"
}
