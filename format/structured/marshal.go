package structured

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/util/maputil"

	"github.com/beevik/etree"
	"github.com/ghodss/yaml"
)

func NewMarshaler() *Marshaler {
	return &Marshaler{
		xmlConfig: &xmlMarshalConfig{
			attributePrefix:         "@",
			textKey:                 "#text",
			rootElement:             "root",
			indent:                  2,
			defaultArrayElementName: "el",
			keyReplacements:         [][2]string{{"/", "__link__"}, {".", "__link_extra__"}},
		},
	}
}

type Marshaler struct {
	xmlConfig *xmlMarshalConfig
}

func (m *Marshaler) SetOptions(options ...XmlMarshalOption) *Marshaler {
	for _, option := range options {
		option(m.xmlConfig)
	}
	return m
}

func (m *Marshaler) JSON(w io.Writer, data interface{}) error {
	b, err := json.Marshal(data)
	if err == nil {
		_, err = w.Write(b)
	}
	return err
}

func (m *Marshaler) YAML(w io.Writer, data interface{}) error {
	b, err := yaml.Marshal(data)
	if err == nil {
		_, err = w.Write(b)
	}
	return err
}

func (m *Marshaler) XML(w io.Writer, data interface{}) error {
	if data == nil {
		return nil
	}
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	root := doc.CreateElement(m.xmlConfig.rootElement)
	err := m.marshal(root, data)

	doc.Indent(m.xmlConfig.indent)
	_, err = doc.WriteTo(w)
	return err
}

func (m *Marshaler) marshal(elem *etree.Element, data interface{}) (err error) {
	data = dereference(data)
	switch t := data.(type) {
	case map[string]interface{}:
		keys := maputil.SortedKeys(t)
		for _, key := range keys {
			val := t[key]
			if key == m.xmlConfig.textKey {
				elem.SetText(fmt.Sprintf("%v", val))
			} else if strings.HasPrefix(key, m.xmlConfig.attributePrefix) {
				elem.CreateAttr(strings.TrimPrefix(key, m.xmlConfig.attributePrefix), fmt.Sprintf("%v", val))
			} else {
				err = m.marshal(elem.CreateElement(m.xmlConfig.keyToElement(key)), val)
				if err != nil {
					return err
				}
			}
		}
	case []interface{}:
		for _, val := range t {
			err = m.marshal(elem.CreateElement(m.arrayElementName(elem.Tag)), val)
			if err != nil {
				return err
			}
		}
	case string:
		elem.SetText(t)
	case fmt.Stringer:
		elem.SetText(t.String())
	case float64, bool:
		elem.SetText(fmt.Sprintf("%v", t))
	case nil:
		// nothing to do...
	default:
		return errors.E("marshal xml", errors.K.Invalid, "type", fmt.Sprintf("%T", data))
	}
	return
}

func (m *Marshaler) arrayElementName(parent string) string {
	if strings.HasSuffix(parent, "s") {
		return strings.TrimSuffix(parent, "s")
	}
	return m.xmlConfig.defaultArrayElementName
}

func (m *Marshaler) Wrap(data interface{}) interface{} {
	return &CustomSDWrapper{
		data: data,
		m:    m,
	}
}

type CustomSDWrapper struct {
	data interface{}
	m    *Marshaler
}

func (c *CustomSDWrapper) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.data)
}

func (c *CustomSDWrapper) MarshalXML(e *xml.Encoder, start xml.StartElement) (err error) {
	c.data = dereference(c.data)
	switch t := c.data.(type) {
	case map[string]interface{}:
		keys := maputil.SortedKeys(t)

		// add all attributes
		for _, key := range keys {
			if strings.HasPrefix(key, c.m.xmlConfig.attributePrefix) {
				start.Attr = append(start.Attr, xml.Attr{
					Name:  xml.Name{Local: strings.TrimPrefix(key, c.m.xmlConfig.attributePrefix)},
					Value: fmt.Sprintf("%v", t[key]),
				})
			}
		}
		err = e.EncodeToken(start)
		if err != nil {
			return err
		}
		for _, key := range keys {
			val := t[key]
			if key == c.m.xmlConfig.textKey {
				err = e.EncodeToken(xml.CharData(fmt.Sprintf("%v", val)))
			} else if strings.HasPrefix(key, c.m.xmlConfig.attributePrefix) {
				// already treated above
			} else {
				err = e.EncodeElement(&CustomSDWrapper{
					data: val,
					m:    c.m,
				}, xml.StartElement{Name: xml.Name{Local: key}})
			}
			if err != nil {
				return err
			}
		}
		err = e.EncodeToken(start.End())
	case []interface{}:
		err = e.EncodeToken(start)
		if err != nil {
			return err
		}
		for _, val := range t {
			err = e.EncodeElement(&CustomSDWrapper{
				data: val,
				m:    c.m,
			}, xml.StartElement{
				Name: xml.Name{Local: c.m.arrayElementName(start.Name.Local)},
				Attr: nil,
			})
			if err != nil {
				return err
			}
		}
		err = e.EncodeToken(start.End())
	default:
		err = e.EncodeElement(t, start)
	}
	return err
}
