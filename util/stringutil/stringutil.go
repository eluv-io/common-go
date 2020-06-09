package stringutil

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// StripFunc removes all runes from the given string that match the given filter
// function.
func StripFunc(s string, filter func(r rune) bool) string {
	return strings.Map(func(r rune) rune {
		if filter(r) {
			return -1
		}
		return r
	}, s)
}

// AsString returns the given value as string. If the value is not a string or
// nil, it returns the empty string "".
func AsString(val interface{}) string {
	if val == nil {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// ToString converts the given value to a string, using the default conversion
// defined in fmt.Sprint(val). Returns the empty string "" if val is nil.
func ToString(val interface{}) string {
	if val == nil {
		return ""
	}
	s, ok := val.(string)
	if ok {
		return s
	}
	return fmt.Sprint(val)
}

// ToPrintString escapes non-ASCII characters and ASCII characters that are not
// printable and not whitespace.
func ToPrintString(s string) string {
	res := ""
	for _, r := range s {
		if r > unicode.MaxASCII || (!unicode.IsSpace(r) && !unicode.IsPrint(r)) {
			// Escape character
			res += strings.Trim(strconv.QuoteRuneToASCII(r), "'")
		} else {
			// Leave character untouched
			res += string(r)
		}
	}
	return res
}

// First returns the first non-empty string. Returns an empty string if all
// provided strings are empty (or no string is provided).
func First(s ...string) string {
	for _, val := range s {
		if len(val) > 0 {
			return val
		}
	}
	return ""
}

// Contains checks whether the given string is present in the provided slice.
// Returns the string's position in the slice and true, or -1 and false.
func Contains(s string, slice []string) (index int, exists bool) {
	var el string
	for index, el = range slice {
		if s == el {
			return index, true
		}
	}
	return -1, false
}

// ContainsSubstring checks whether the given string is present in the provided slice.
// Returns the string's position in the slice and true, or -1 and false.
func ContainsSubstring(s string, slice []string) (index int, exists bool) {
	var el string
	for index, el = range slice {
		if strings.Contains(el, s) {
			return index, true
		}
	}
	return -1, false
}

// ToIndexMap converts the given string slice to a map with the strings values
// mapping to their index (position) in the slice.
func ToIndexMap(slice []string) map[string]int {
	m := make(map[string]int)
	for idx, val := range slice {
		m[val] = idx
	}
	return m
}

// Converts the given slice to a string slice, using the default format of the
// fmt package.
func ToSlice(s []interface{}) []string {
	res := make([]string, len(s))
	for idx, val := range s {
		var ok bool
		res[idx], ok = val.(string)
		if !ok {
			res[idx] = fmt.Sprint(val)
		}
	}
	return res
}

const lettersDigits = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

// RandomString returns a random alphanumerical string of specified length.
func RandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = lettersDigits[rand.Intn(len(lettersDigits))]
	}
	return string(b)
}

func isLineSep(r rune) bool {
	switch r {
	case '\n', '\r':
		return true
	}
	return false
}

func SplitToLines(bb []byte) []string {
	return strings.FieldsFunc(string(bb), isLineSep)
}

// StringSlice returns a slice of strings if the given value is a slice
// containing only strings. It returns nil otherwise.
func StringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	av := reflect.ValueOf(v)
	if av.Kind() != reflect.Slice {
		return nil
	}
	ret := make([]string, 0)
	for i := 0; i < av.Len(); i++ {
		sv := av.Index(i)
		switch sv.Kind() {
		case reflect.String:
			ret = append(ret, sv.String())
		case reflect.Interface:
			switch sv.Elem().Kind() {
			case reflect.String:
				ret = append(ret, sv.Elem().String())
			default:
				return nil
			}
		default:
			return nil
		}
	}
	return ret
}
