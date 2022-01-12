package bytesize_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/bytesize"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMarshalText(t *testing.T) {
	table := []struct {
		in       bytesize.Spec
		expected string
	}{
		{0, "0B"},
		{bytesize.B, "1B"},
		{bytesize.KB, "1KB"},
		{bytesize.MB, "1MB"},
		{bytesize.GB, "1GB"},
		{bytesize.TB, "1TB"},
		{bytesize.PB, "1PB"},
		{bytesize.EB, "1EB"},
		{400 * bytesize.TB, "400TB"},
		{2048 * bytesize.MB, "2GB"},
		{bytesize.B + bytesize.KB, "1025B"},
		{bytesize.MB + 20*bytesize.KB, "1044KB"},
		{bytesize.Spec(76234765239), "76234765239B"},
		{bytesize.Spec(1024000), "1000KB"},
	}

	Convey("Marshaling to text should produce correct result", t, func() {
		for _, tt := range table {
			Convey(fmt.Sprintf("%22s -> %22s", formatInt(tt.in), formatString(tt.expected)), func() {
				b, _ := tt.in.MarshalText()
				s := string(b)

				So(s, ShouldEqual, tt.expected)
			})
		}
	})
}

func TestUnmarshalText(t *testing.T) {
	table := []struct {
		in       string
		err      error
		expected bytesize.Spec
	}{
		{"0", nil, 0},
		{"0B", nil, 0},
		{"0 KB", nil, 0},
		{"0.1 B", nil, 0},
		{"0.000001 KB", nil, 0},
		{"1", nil, bytesize.B},
		{"1K", nil, bytesize.KB},
		{"2MB", nil, 2 * bytesize.MB},
		{"5 GB", nil, 5 * bytesize.GB},
		{"20480 G", nil, 20 * bytesize.TB},
		{"50 eB", strconv.ErrRange, bytesize.Spec((1 << 64) - 1)},
		{"200000 pb", strconv.ErrRange, bytesize.Spec((1 << 64) - 1)},
		{"10 Mb", bytesize.ErrBits, 0},
		{"g", strconv.ErrSyntax, 0},
		{"10 kB ", nil, 10 * bytesize.KB},
		{"  10 kB ", nil, 10 * bytesize.KB},
		{"  0010 kB ", nil, 10 * bytesize.KB},
		{"10kb", nil, 10 * bytesize.KB},
		{"10 kBs ", strconv.ErrSyntax, 0},
		{"10 eB", nil, 10 * bytesize.EB},
		{"402.5MB", nil, 402*bytesize.MB + 512*bytesize.KB},
		{"7.000244140625TB", nil, 7*bytesize.TB + 256*bytesize.MB},
		{"10.125EB", nil, 10*bytesize.EB + 128*bytesize.PB},
		{"1.453.6693MB", strconv.ErrSyntax, 0},
	}

	Convey("Unmarshaling from text should produce correct result", t, func() {
		for _, tt := range table {

			Convey(formatUnmarshalMsg(tt.in, tt.err, tt.expected), func() {

				var s bytesize.Spec
				err := s.UnmarshalText([]byte(tt.in))

				if tt.err != nil {
					So(err, ShouldHaveSameTypeAs, &errors.Error{})
					ne, _ := err.(*errors.Error)
					So(ne.Cause(), ShouldEqual, tt.err)
				} else {
					So(err, ShouldBeNil)
				}
				So(s, ShouldEqual, tt.expected)
			})
		}
	})

}

func TestUnmarshalJson(t *testing.T) {
	table := []struct {
		in       string
		err      error
		expected bytesize.Spec
	}{
		{`1`, nil, bytesize.Spec(1)},
		{`"1B"`, nil, bytesize.Spec(1)},
		{`"1 KB"`, nil, bytesize.Spec(bytesize.KB)},
	}

	Convey("Unmarshaling from JSON should produce correct result", t, func() {
		for _, tt := range table {

			Convey(fmt.Sprintf("%22s -> %22s", formatStringBrackets(tt.in), formatString(tt.expected)), func() {

				var s bytesize.Spec
				err := s.UnmarshalJSON([]byte(tt.in))

				if tt.err != nil {
					So(err, ShouldHaveSameTypeAs, &errors.Error{})
					ne, _ := err.(*errors.Error)
					So(ne.Cause(), ShouldEqual, tt.err.Error())
				} else {
					So(err, ShouldBeNil)
				}
				So(s, ShouldEqual, tt.expected)
			})
		}
	})

}

func formatUnmarshalMsg(
	in string,
	err error,
	expected bytesize.Spec) string {

	if err != nil {
		return fmt.Sprintf("%22s -> %22s %s", formatString(in), formatInt(expected), err)
	}
	return fmt.Sprintf("%22s -> %22s", formatString(in), formatInt(expected))
}

func formatInt(v interface{}) string {
	return fmt.Sprintf("[%d]", v)
}

func formatString(v interface{}) string {
	return fmt.Sprintf("\"%s\"", v)
}

func formatStringBrackets(v interface{}) string {
	return fmt.Sprintf("[%s]", v)
}

func ExampleSpec_HumanReadable() {
	fmt.Println(bytesize.Spec(5123456).HumanReadable())
	fmt.Println(bytesize.Spec(5123456).HR())

	// Output:
	//
	// 4.9MB
	// 4.9MB (5123456B)
}
