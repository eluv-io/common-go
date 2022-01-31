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

// ContainsSubstring checks whether the given string is contained in any of the
// strings of the provided slice. Returns the matching string's position in the
// slice and true, or -1 and false.
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

// IndentLines indents all lines in the given string with specified number of
// spaces.
func IndentLines(s string, spaces int) string {
	return PrefixLines(s, strings.Repeat(" ", spaces))
}

// PrefixLines prefixes each line in the given string with the given prefix.
func PrefixLines(s, prefix string) string {
	return prefix + strings.Replace(s, "\n", "\n"+prefix, -1)
}

// PostfixLines postfixes each line in the given string with the given prefix.
func PostfixLines(s, postfix string) string {
	return strings.Replace(s, "\n", postfix+"\n", -1) + postfix
}

// WrapLines pre- and postfixes each line in s with w.
func WrapLines(s, w string) string {
	return w + strings.Replace(s, "\n", w+"\n"+w, -1) + w
}

// Stringer decorates any parameter-less function that returns a string as a
// fmt.Stringer interface.
//
// Useful in situations where string generation is costly and should only be
// performed when necessary, i.e. in logging statements. The following will call
// call obj.AsJSON() only in case the log is actually in DEBUG.
//
//   log.Debug("costly string", stringutil.Stringer(obj.AsJSON))
type Stringer func() string

func (s Stringer) String() string {
	if s == nil {
		return "nil"
	}
	return s()
}

func (s Stringer) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// LessLex reports whether i must sort before j, using lexicographic order
// (aka alphabetical, dictionary, or natural sort order). That is, compare
// numbers by value rather than character by character. Currently only integer
// comparison is implemented (no floats)
//
// Usage: sort.Slice(list, func(i, j int) bool { return stringutil.LessLex(list[i], list[j]) })
func LessLex(i, j string) bool {
	var err error
	var ipos, jpos int   // Current character indices
	var inum, jnum int64 // Current number
	for ipos < len(i) && jpos < len(j) {
		ichar := i[ipos]
		jchar := j[jpos]
		// Convert numbers to int64 for comparison
		if unicode.IsDigit(rune(ichar)) && unicode.IsDigit(rune(jchar)) {
			if inum, ipos, err = parseNum(i, ipos); err != nil {
				return i < j
			}
			if jnum, jpos, err = parseNum(j, jpos); err != nil {
				return i < j
			}
			if inum == jnum {
				if ipos != jpos {
					return ipos > jpos // e.g. 7 compared to 007
				}
			} else {
				return inum < jnum
			}
		} else {
			if ichar == jchar {
				ipos++
				jpos++
			} else {
				return ichar < jchar
			}
		}
	}
	return len(i) < len(j)
}

// parseNum converts the number string at the start index to int64
func parseNum(s string, startPos int) (num int64, endPos int, err error) {
	endPos = digitEndPosition(s, startPos)
	num, err = strconv.ParseInt(s[startPos:endPos], 10, 64)
	return
}

// digitEndPosition finds the end index of the number beginning from the start
// index
func digitEndPosition(s string, startPos int) int {
	i := startPos
	for ; i < len(s); i++ {
		if !unicode.IsDigit(rune(s[i])) {
			break
		}
	}
	return i
}

// MatchRunes returns true if all runes of string s match all provided functions, false otherwise.
//
// Example usage:
//	if stringutil.MatchRunes("some string", unicode.IsLetter, unicode.IsDigit) { ... }
func MatchRunes(s string, funcs ...func(r rune) bool) bool {
	for _, r := range s {
		for _, f := range funcs {
			if !f(r) {
				return false
			}
		}
	}
	return true
}
