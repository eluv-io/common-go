package structured

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalBasic(t *testing.T) {
	m := NewMarshaler()
	tests := []struct {
		name        string
		convertFunc func(w io.Writer, data interface{}) error
		assert      func(t *testing.T, res string)
	}{
		{
			name:        "JSON",
			convertFunc: m.JSON,
			assert: func(t *testing.T, res string) {
				assert.JSONEq(t, jsonSource, res)
			},
		},
		{
			name:        "YAML",
			convertFunc: m.YAML,
			assert: func(t *testing.T, res string) {
				exp, _ := NewUnmarshaler().YAML([]byte(yamlSource))
				act, _ := NewUnmarshaler().YAML([]byte(res))
				assert.Equal(t, exp, act)
			},
		},
		{
			name:        "XML",
			convertFunc: m.XML,
			assert: func(t *testing.T, res string) {
				exp, _ := NewUnmarshaler().XML([]byte(xmlSource))
				act, _ := NewUnmarshaler().XML([]byte(res))
				assert.Equal(t, exp, act)
			},
		},
	}

	var source interface{}
	json.Unmarshal([]byte(jsonSource), &source)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := test.convertFunc(buf, source)

			res := buf.String()
			fmt.Println(res)

			assert.NoError(t, err)
			test.assert(t, res)
		})
	}
}

func TestMarshalXML(t *testing.T) {
	m := NewMarshaler()
	tests := []struct {
		name    string
		source  interface{}
		options []XmlMarshalOption
	}{
		{
			name:   "Element with attributes and simple text",
			source: jm{"@att1": 1.0, "@att2": 2.0, "#text": "text"},
		},
		{
			name:   "Element with attributes and child element",
			source: jm{"@att1": 1.0, "@att2": 2.0, "b": "text"},
		},
		{
			name:   "Element with attributes and list of child elements",
			source: jm{"@att1": 1.0, "@att2": 2.0, "b": ja{"b1", "b2"}},
		},
		{
			name:   "Element without attributes, but list of child elements",
			source: ja{"b1", "b2"},
		},
		{
			name:   "Element with list of child elements of different types",
			source: jm{"b": ja{"b1", "b2"}, "c": ja{1.0, 2.0}},
		},
		{
			name:   "Element with attributes and simple text, customized with options",
			source: jm{"_att1": json.Number("1"), "_att2": json.Number("2"), "TEXT": "12345"},
			options: []XmlMarshalOption{
				OptAttributePrefix("_"),
				OptTextKey("TEXT"),
				OptParseNumbersInAttributes(true),
				OptParseNumbersInElements(false),
				OptUseNumber(true),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := m.SetOptions(test.options...).XML(buf, test.source)

			fmt.Println(buf.String())
			assert.NoError(t, err)

			res, err := NewUnmarshaler().SetOptions(test.options...).XML(buf.Bytes())
			assert.NoError(t, err)
			assert.Equal(t, test.source, res)
		})
	}
}

type TestStruct struct {
	XMLName    string      `xml:"wrapper"`
	Name       string      `xml:"name"`
	Age        int         `xml:"age"`
	MapOrArray interface{} `xml:"moa"`
}

func TestMarshalXMLWrapped(t *testing.T) {
	tests := []struct {
		name     string
		source   interface{}
		expected string
		options  []XmlMarshalOption
	}{
		{
			name:     "Element with attributes and simple text",
			source:   jm{"@att1": 1.0, "@att2": 2.0, "#text": "text"},
			expected: `<moa att1="1" att2="2">text</moa>`,
		},
		{
			name:     "Element with attributes and child element",
			source:   jm{"@att1": 1.0, "@att2": 2.0, "b": "text"},
			expected: `<moa att1="1" att2="2"><b>text</b></moa>`,
		},
		{
			name:     "Element with attributes and list of child elements",
			source:   jm{"@att1": 1.0, "@att2": 2.0, "b": ja{"b1", "b2"}},
			expected: `<moa att1="1" att2="2"><b><el>b1</el><el>b2</el></b></moa>`,
		},
		{
			name:     "Element without attributes, but list of child elements",
			source:   ja{"b1", "b2"},
			expected: `<moa><el>b1</el><el>b2</el></moa>`,
		},
		{
			name:     "Element with list of child elements of different types",
			source:   jm{"bs": ja{"b1", "b2"}, "cs": ja{1.0, 2.0}},
			expected: `<moa><bs><b>b1</b><b>b2</b></bs><cs><c>1</c><c>2</c></cs></moa>`,
		},
		{
			name:     "Element with attributes and simple text, customized with options",
			source:   jm{"_att1": json.Number("1"), "_att2": json.Number("2"), "TEXT": "12345"},
			expected: `<moa att1="1" att2="2">12345</moa>`,
			options: []XmlMarshalOption{
				OptAttributePrefix("_"),
				OptTextKey("TEXT"),
				OptParseNumbersInAttributes(true),
				OptParseNumbersInElements(false),
				OptUseNumber(true),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := NewMarshaler()
			m.SetOptions(test.options...)

			src := &TestStruct{
				Name:       "joe.doe",
				Age:        99,
				MapOrArray: m.Wrap(test.source),
			}

			buf, err := xml.Marshal(src)

			fmt.Println(string(buf))
			assert.NoError(t, err)

			assert.Equal(t, `<wrapper><name>joe.doe</name><age>99</age>`+test.expected+`</wrapper>`, string(buf))
		})
	}
}

func TestMarshalLinks(t *testing.T) {
	m := NewMarshaler()
	buf := &bytes.Buffer{}
	source := jm{"/": "./meta/link"}
	err := m.XML(buf, source)
	require.NoError(t, err)

	u := NewUnmarshaler()
	data, err := u.XML(buf.Bytes())
	require.NoError(t, err)
	require.EqualValues(t, source, data)
}
