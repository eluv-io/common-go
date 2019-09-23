package structured_test

import (
	"io"
	"testing"

	"github.com/qluvio/content-fabric/format/structured"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValue_String(t *testing.T) {
	const s = "yo!"
	Convey("Given a value wrapping a string", t, func() {
		val := structured.NewValue(s, nil)

		Convey("Retrieving the value as interface{} returns the string", func() {
			So(val.Value(), ShouldEqual, s)
		})
		Convey("Retrieving the value as interface{} with default returns the string", func() {
			So(val.Value(5.3), ShouldEqual, s)
		})
		Convey("Retrieving the value as string succeeds", func() {
			So(val.String(), ShouldEqual, s)
		})
		Convey("Retrieving the value as string with default returns the string", func() {
			So(val.String("default"), ShouldEqual, s)
		})
		Convey("Retrieving the value as int returns 0", func() {
			So(val.Int(), ShouldEqual, 0)
		})
		Convey("Retrieving the value as int with default returns the default", func() {
			So(val.Int(10), ShouldEqual, 10)
		})
		Convey("IsError() returns false", func() {
			So(val.IsError(), ShouldBeFalse)
		})
		Convey("Error() returns nil", func() {
			So(val.Error(), ShouldBeNil)
		})
	})
}

func TestValue_Error(t *testing.T) {
	Convey("Given a value wrapping an error", t, func() {
		val := structured.NewValue(t, io.EOF)

		Convey("Retrieving the value as interface{} returns nil", func() {
			So(val.Value(), ShouldEqual, nil)
		})
		Convey("Retrieving the value as interface{} with default returns the default", func() {
			So(val.Value(5.3), ShouldEqual, 5.3)
		})
		Convey("Retrieving the value as string returns the empty string", func() {
			So(val.String(), ShouldEqual, "")
		})
		Convey("Retrieving the value as string with default returns the default", func() {
			So(val.String("default"), ShouldEqual, "default")
		})
		Convey("Retrieving the value as int returns 0", func() {
			So(val.Int(), ShouldEqual, 0)
		})
		Convey("Retrieving the value as int with default returns the default", func() {
			So(val.Int(10), ShouldEqual, 10)
		})
		Convey("IsError() returns true", func() {
			So(val.IsError(), ShouldBeTrue)
		})
		Convey("Error() returns the error", func() {
			So(val.Error(), ShouldEqual, io.EOF)
		})
	})
}
