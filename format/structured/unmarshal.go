package structured

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/qluvio/content-fabric/errors"

	"github.com/beevik/etree"
	"github.com/ghodss/yaml"
)

func NewUnmarshaler() *Unmarshaler {
	return &Unmarshaler{
		xmlConfig: &xmlMarshalConfig{
			attributePrefix:           "@",
			textKey:                   "#text",
			parseNumbersInElements:    true,
			parseNumbersInAttributes:  true,
			useNumber:                 false,
			detectSingleElementArrays: true,
			keyReplacements:           [][2]string{{"/", "__link__"}, {".", "__link_extra__"}},
		}}
}

type Unmarshaler struct {
	xmlConfig *xmlMarshalConfig
}

func (u *Unmarshaler) SetOptions(options ...XmlMarshalOption) *Unmarshaler {
	for _, option := range options {
		option(u.xmlConfig)
	}
	return u
}

func (u *Unmarshaler) JSON(text []byte) (interface{}, error) {
	var v interface{}
	err := json.Unmarshal(text, &v)
	return v, err
}

func (u *Unmarshaler) YAML(text []byte) (interface{}, error) {
	var v interface{}
	err := yaml.Unmarshal(text, &v)
	return v, err
}

func (u *Unmarshaler) XML(text []byte) (interface{}, error) {
	doc := etree.NewDocument()
	err := doc.ReadFromBytes(text)
	if err != nil {
		return nil, errors.E("unmarshal xml", errors.K.Invalid, err)
	}
	v := u.convert(doc.Root())
	return v, nil
}

type xmlMarshalConfig struct {
	// unmarshaling: the prefix for attribute keys
	attributePrefix string
	// unmarshaling: the key name used for an elements text (if that element also has attributes)
	textKey string
	// unmarshaling: try detecting and parsing numbers in element values
	parseNumbersInElements bool
	// unmarshaling: try detecting and parsing numbers in attribute values
	parseNumbersInAttributes bool
	// unmarshaling: parse numbers into json.Number objects instead of float64
	useNumber bool
	// unmarshaling: detect single element arrays based on naming convention: <books><book/></books>
	detectSingleElementArrays bool

	// marshaling: name of the generated wrapping root element
	rootElement string
	// marshaling: the number of spaces to indent children at each level
	indent int
	// the name used for array elements if the array itself does not end with "s"
	defaultArrayElementName string

	// key replacements: list of replacement pairs. First string is in-memory map key, second string XML element name
	keyReplacements [][2]string
}

func (x *xmlMarshalConfig) keyToElement(val string) string {
	return x.replace(val, true)
}

func (x *xmlMarshalConfig) elementToKey(val string) string {
	return x.replace(val, false)
}

func (x *xmlMarshalConfig) replace(val string, compareKey bool) string {
	i1, i2 := 0, 1
	if !compareKey {
		i1, i2 = 1, 0
	}
	for _, pair := range x.keyReplacements {
		if pair[i1] == val {
			return pair[i2]
		}
	}
	return val
}

type XmlMarshalOption func(*xmlMarshalConfig)

func OptAttributePrefix(prefix string) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.attributePrefix = prefix
	}
}

func OptTextKey(key string) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.textKey = key
	}
}

func OptParseNumbersInElements(enabled bool) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.parseNumbersInElements = enabled
	}
}

func OptParseNumbersInAttributes(enabled bool) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.parseNumbersInAttributes = enabled
	}
}

func OptUseNumber(enabled bool) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.useNumber = enabled
	}
}

func OptRootElement(name string) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.rootElement = name
	}
}

func OptIndent(spaces int) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.indent = spaces
	}
}

func OptDefaultArrayElementName(name string) XmlMarshalOption {
	return func(c *xmlMarshalConfig) {
		c.defaultArrayElementName = name
	}
}

func (u *Unmarshaler) convert(el *etree.Element) interface{} {
	if el == nil {
		// unmarshalling an empty XML
		return nil
	}
	children := el.ChildElements()
	text := el.Text()
	if len(el.Attr) == 0 && len(children) == 0 {
		// a pure text element
		return u.parseNumber(text, u.xmlConfig.parseNumbersInElements)
	}

	m := make(map[string]interface{})
	if text != "" && len(children) == 0 {
		m[u.xmlConfig.textKey] = u.parseNumber(text, u.xmlConfig.parseNumbersInElements)
	}
	for _, a := range el.Attr {
		key := strings.Builder{}
		key.WriteString(u.xmlConfig.attributePrefix)
		if a.Space != "" {
			key.WriteString(a.Space)
			key.WriteString(":")
		}
		key.WriteString(a.Key)
		m[key.String()] = u.parseNumber(a.Value, u.xmlConfig.parseNumbersInAttributes)
	}
	arrayConversion := false
	for _, c := range children {
		key := u.xmlConfig.elementToKey(keyOf(c))
		if v, found := m[key]; found {
			// same key already exists
			if a, ok := v.([]interface{}); ok {
				// the value is already a slice, so just append the child
				a = append(a, u.convert(c))
				// and store the new slice at key
				m[key] = a
			} else {
				// convert the existing value to an array
				arrayConversion = true
				a = make([]interface{}, 2, 5)
				a[0] = v
				a[1] = u.convert(c)
				m[key] = a
			}
		} else {
			m[key] = u.convert(c)
		}
	}
	if len(m) == 1 {
		for key, value := range m {
			if a, ok := value.([]interface{}); ok && arrayConversion {
				return a
			}
			if u.xmlConfig.detectSingleElementArrays && key+"s" == keyOf(el) {
				// the single child is potentially an element of an array, since
				// its name is the same as the parent without the final "s",
				// e.g. parent=books, child=book
				a := make([]interface{}, 1)
				a[0] = value
				return a
			}
		}
	}
	return m
}

func keyOf(c *etree.Element) string {
	keyBuilder := strings.Builder{}
	if c.Space != "" {
		keyBuilder.WriteString(c.Space)
		keyBuilder.WriteByte(':')
	}
	keyBuilder.WriteString(c.Tag)
	key := keyBuilder.String()
	return key
}

func (u *Unmarshaler) parseNumber(text string, enabled bool) interface{} {
	if enabled && isValidNumber(text) {
		if u.xmlConfig.useNumber {
			return json.Number(text)
		} else {
			f, err := strconv.ParseFloat(text, 64)
			if err == nil {
				return f
			}
		}
	}
	return text
}

// isValidNumber reports whether s is a valid JSON number literal. Copied from
// standard json package because it's not exported unfortunately...
func isValidNumber(s string) bool {
	// This function implements the JSON numbers grammar.
	// See https://tools.ietf.org/html/rfc7159#section-6
	// and http://json.org/number.gif

	if s == "" {
		return false
	}

	// Optional -
	if s[0] == '-' {
		s = s[1:]
		if s == "" {
			return false
		}
	}

	// Digits
	switch {
	default:
		return false

	case s[0] == '0':
		s = s[1:]

	case '1' <= s[0] && s[0] <= '9':
		s = s[1:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// . followed by 1 or more digits.
	if len(s) >= 2 && s[0] == '.' && '0' <= s[1] && s[1] <= '9' {
		s = s[2:]
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// e or E followed by an optional - or + and
	// 1 or more digits.
	if len(s) >= 2 && (s[0] == 'e' || s[0] == 'E') {
		s = s[1:]
		if s[0] == '+' || s[0] == '-' {
			s = s[1:]
			if s == "" {
				return false
			}
		}
		for len(s) > 0 && '0' <= s[0] && s[0] <= '9' {
			s = s[1:]
		}
	}

	// Make sure we are at the end.
	return s == ""
}
