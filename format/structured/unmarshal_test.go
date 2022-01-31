package structured

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/eluv-io/common-go/util/jsonutil"

	"github.com/stretchr/testify/assert"
)

var jsonSource = `
{
    "store": {
        "books": [
            {
                "category": "reference",
                "author": "Nigel Rees",
                "title": "Sayings of the Century",
                "price": 8.95
            },
            {
                "category": "fiction",
                "author": "Evelyn Waugh",
                "title": "Sword of Honour",
                "price": 12.99
            },
            {
                "category": "fiction",
                "author": "Herman Melville",
                "title": "Moby Dick",
                "isbn": "0-553-21311-3",
                "price": 8.99
            },
            {
                "category": "fiction",
                "author": "J. R. R. Tolkien",
                "title": "The Lord of the Rings",
                "isbn": "0-395-19395-8",
                "price": 22.99
            }
        ],
        "bicycle": {
            "color": "red",
            "price": 19.95
        }
    },
    "expensive": 10
}`

var yamlSource = `
store:
  books:
  - category: reference
    author: Nigel Rees
    title: Sayings of the Century
    price: 8.95
  - category: fiction
    author: Evelyn Waugh
    title: Sword of Honour
    price: 12.99
  - category: fiction
    author: Herman Melville
    title: Moby Dick
    isbn: 0-553-21311-3
    price: 8.99
  - category: fiction
    author: J. R. R. Tolkien
    title: The Lord of the Rings
    isbn: 0-395-19395-8
    price: 22.99
  bicycle:
    color: red
    price: 19.95
expensive: 10
`

var xmlSource = `
<?xml version="1.0" encoding="UTF-8"?>
<root>
  <store>
    <books>
      <book>
        <author>Nigel Rees</author>
        <category>reference</category>
        <price>8.95</price>
        <title>Sayings of the Century</title>
      </book>
      <book>
        <author>Evelyn Waugh</author>
        <category>fiction</category>
        <price>12.99</price>
        <title>Sword of Honour</title>
      </book>
      <book>
        <author>Herman Melville</author>
        <category>fiction</category>
        <isbn>0-553-21311-3</isbn>
        <price>8.99</price>
        <title>Moby Dick</title>
      </book>
      <book>
        <author>J. R. R. Tolkien</author>
        <category>fiction</category>
        <isbn>0-395-19395-8</isbn>
        <price>22.99</price>
        <title>The Lord of the Rings</title>
      </book>
    </books>
    <bicycle>
      <color>red</color>
      <price>19.95</price>
    </bicycle>
  </store>
  <expensive>10</expensive>
</root>
`

func TestUnmarshalBasic(t *testing.T) {
	u := NewUnmarshaler()
	tests := []struct {
		name        string
		source      string
		convertFunc func(src []byte) (interface{}, error)
	}{
		{
			name:        "JSON",
			source:      jsonSource,
			convertFunc: u.JSON,
		},
		{
			name:        "YAML",
			source:      yamlSource,
			convertFunc: u.YAML,
		},
		{
			name:        "XML",
			source:      xmlSource,
			convertFunc: u.XML,
		},
	}

	var expected interface{}
	json.Unmarshal([]byte(jsonSource), &expected)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v, err := test.convertFunc([]byte(test.source))

			fmt.Println(jsonutil.MarshalString(&v))

			assert.NoError(t, err)
			assert.Equal(t, expected, v)
		})
	}
}

func TestUnmarshalXML(t *testing.T) {
	u := NewUnmarshaler()
	tests := []struct {
		name     string
		source   string
		expected interface{}
		options  []XmlMarshalOption
	}{
		{
			name:     "Element with attributes and simple text",
			source:   `<a att1="1" att2="2">text</a>`,
			expected: jm{"@att1": 1.0, "@att2": 2.0, "#text": "text"},
		},
		{
			name:     "Element with attributes and child element",
			source:   `<a att1="1" att2="2"><b>text</b></a>`,
			expected: jm{"@att1": 1.0, "@att2": 2.0, "b": "text"},
		},
		{
			name:     "Element with attributes and list of child elements",
			source:   `<a att1="1" att2="2"><b>b1</b><b>b2</b></a>`,
			expected: jm{"@att1": 1.0, "@att2": 2.0, "b": ja{"b1", "b2"}},
		},
		{
			name:     "Element without attributes, but list of child elements",
			source:   `<a><b>b1</b><b>b2</b></a>`,
			expected: ja{"b1", "b2"},
		},
		{
			name:     "Element with list of child elements of different types",
			source:   `<a><b>b1</b><b>b2</b><c>1</c><c>2</c></a>`,
			expected: jm{"b": ja{"b1", "b2"}, "c": ja{1.0, 2.0}},
		},
		{
			name:     "Array with single child",
			source:   `<books><book>b1</book></books>`,
			expected: ja{"b1"},
		},
		{
			name:     "Array with single child (complex element)",
			source:   `<books><book att1="1" att2="2">b1</book></books>`,
			expected: ja{jm{"@att1": 1.0, "@att2": 2.0, "#text": "b1"}},
		},
		{
			source: `<?xml version="1.0" encoding="UTF-8"?>
					<root>
					  <errors>
						<error>
						  <cause>authorization header missing</cause>
						  <kind>invalid</kind>
						  <op>authentication</op>
						  <stacktrace>...</stacktrace>
						</error>
					  </errors>
					</root>`,
			name:     "API error response",
			expected: jm{"errors": ja{jm{"cause": "authorization header missing", "kind": "invalid", "op": "authentication", "stacktrace": "..."}}},
		},
		{
			name:     "Element with attributes and simple text, customized with options",
			source:   `<a att1="1" att2="2">12345</a>`,
			expected: jm{"_att1": json.Number("1"), "_att2": json.Number("2"), "TEXT": "12345"},
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
			v, err := u.SetOptions(test.options...).XML([]byte(test.source))

			fmt.Println(jsonutil.MarshalString(&v))

			assert.NoError(t, err)
			assert.Equal(t, test.expected, v)
		})
	}
}
