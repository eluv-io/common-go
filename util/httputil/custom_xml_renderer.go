package httputil

import (
	"bytes"
	"net/http"

	"github.com/eluv-io/common-go/format/structured"
)

var (
	defaultMarshaler   = structured.NewMarshaler()
	defaultUnmarshaler = structured.NewUnmarshaler()
)

func NewCustomXMLRenderer(data interface{}) *CustomXMLRenderer {
	return &CustomXMLRenderer{
		Data:        data,
		marshaler:   defaultMarshaler,
		unmarshaler: defaultUnmarshaler,
	}
}

type CustomXMLRenderer struct {
	Data        interface{}
	marshaler   *structured.Marshaler
	unmarshaler *structured.Unmarshaler
}

var xmlContentType = []string{"application/xml; charset=utf-8"}

func (r CustomXMLRenderer) Render(w http.ResponseWriter) error {
	writeContentType(w, xmlContentType)

	// marshal as JSON, parse JSON into interface{}, marshal as XML
	// this prevents us from having to annotate all model structs with xml
	// tags in addition to json tags, and simplifies xml generation.
	buf := &bytes.Buffer{}
	err := r.marshaler.JSON(buf, r.Data)
	if err != nil {
		return err
	}
	generic, err := r.unmarshaler.JSON(buf.Bytes())
	if err != nil {
		return err
	}
	err = r.marshaler.XML(w, generic)
	return err
}

func (r CustomXMLRenderer) WriteContentType(w http.ResponseWriter) {
	writeContentType(w, xmlContentType)
}

func writeContentType(w http.ResponseWriter, value []string) {
	header := w.Header()
	if val := header["Content-Type"]; len(val) == 0 {
		header["Content-Type"] = value
	}
}
