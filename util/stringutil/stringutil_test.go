package stringutil

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/errors"
	elog "github.com/qluvio/content-fabric/log"
)

func TestStripFunc(t *testing.T) {
	stripA := func(r rune) bool {
		return r == 'a' || r == 'A'
	}
	require.Empty(t, StripFunc("", stripA))
	require.Empty(t, StripFunc("aaa", stripA))

	require.Equal(t, "bb", StripFunc("Abba", stripA))
	require.Equal(t, "Bubb", StripFunc("Bubba", stripA))
	require.Equal(t, "Queen", StripFunc("Queen", stripA))
}

func TestAsString(t *testing.T) {
	require.Equal(t, "", AsString(nil))
	require.Equal(t, "", AsString(""))
	require.Equal(t, "", AsString(89))
	require.Equal(t, "", AsString([]byte("string")))

	require.Equal(t, "string", AsString("string"))
}

func TestToString(t *testing.T) {
	require.Equal(t, "", ToString(nil))
	require.Equal(t, "", ToString(""))
	require.Equal(t, "89", ToString(89))
	require.Equal(t, "[115 116 114 105 110 103]", ToString([]byte("string")))
	require.Equal(t, "string", ToString("string"))
}

func TestToPrintString(t *testing.T) {
	require.Equal(t, "", ToPrintString(""))
	require.Equal(t, "Fran & Freddie's Diner\t\\u263a\r\n", ToPrintString("Fran & Freddie's Diner\t\u263a\r\n"))
}

func TestFirst(t *testing.T) {
	require.Empty(t, First())
	require.Empty(t, First(""))
	require.Empty(t, First("", "", ""))

	require.Equal(t, "one", First("one"))
	require.Equal(t, "one", First("one", "two"))
	require.Equal(t, "one", First("", "one", ""))
	require.Equal(t, "one", First("", "", "one", "two"))
}

func TestToSlice(t *testing.T) {
	require.Equal(t, []string{}, ToSlice(nil))
	require.Equal(t, []string{}, ToSlice([]interface{}{}))
	require.Equal(t, []string{"one", "two"}, ToSlice([]interface{}{"one", "two"}))
	require.Equal(t, []string{"one", "2"}, ToSlice([]interface{}{"one", 2}))
}

func TestSplitToLines(t *testing.T) {
	s := "aha\ngot from win\r\nbut also \tfrom mac\n\rand what?"
	expected := []string{
		"aha",
		"got from win",
		"but also \tfrom mac",
		"and what?",
	}
	ss := SplitToLines([]byte(s))
	require.Equal(t, expected, ss)
}

func TestStringSlice(t *testing.T) {
	s1 := []string{"a", "b"}
	require.Equal(t, s1, StringSlice(s1))

	require.Nil(t, StringSlice(nil))

	s2 := []interface{}{"a", 8, "b"}
	require.Nil(t, StringSlice(s2))

	s3 := []interface{}{"a", "b"}
	require.Equal(t, s1, StringSlice(s3))
}

func TestIndent(t *testing.T) {
	require.Equal(t, "  a\n  b\n  c", IndentLines("a\nb\nc", 2))
	require.Equal(t, "...a\n...b\n...c", PrefixLines("a\nb\nc", "..."))
	require.Equal(t, "...a", PrefixLines("a", "..."))
}

func TestStringer(t *testing.T) {

	require.Equal(t, "a string", Stringer(func() string { return "a string" }).String())

	log := elog.Get("/TestStringer")
	log.SetInfo()

	i := 0
	fn := func() string { i++; return fmt.Sprint(i) }

	for j := 1; j < 10; j++ {
		log.Debug("msg", "i", Stringer(fn))
		require.Equal(t, 0, i)
	}

	for j := 1; j < 10; j++ {
		log.Info("msg", "i", Stringer(fn))
		require.Equal(t, j, i)
	}

	jsn, err := json.Marshal(errors.E("op", errors.K.Invalid, "val", Stringer(fn)))
	require.NoError(t, err)
	require.Contains(t, string(jsn), `"val":"10"`)
}

func TestLessLex(t *testing.T) {
	tests := []struct {
		i    string
		j    string
		want bool
	}{
		{"a", "a", false},
		{"a", "b", true},
		{"b", "a", false},
		{"a", "aa", true},
		{"0", "0", false},
		{"0", "1", true},
		{"1", "0", false},
		{"9", "10", true},
		{"9a", "9a", false},
		{"9a", "9b", true},
		{"9a", "10a", true},
		{"9.a", "10.a", true},
		{"9a9", "9a9", false},
		{"9a9", "9a10", true},
		{"a9", "a9", false},
		{"a9", "a10", true},
		{"007", "7", true},
	}
	for _, tt := range tests {
		got := LessLex(tt.i, tt.j)
		require.Equal(t, tt.want, got, "%s < %s", tt.i, tt.j)
	}
}
