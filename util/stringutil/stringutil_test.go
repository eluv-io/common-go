package stringutil

import (
	"testing"

	"github.com/stretchr/testify/require"
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
