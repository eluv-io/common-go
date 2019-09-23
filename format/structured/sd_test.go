package structured_test

import (
	"testing"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/util/maputil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSD(t *testing.T) {
	var val *structured.Value

	Convey("After wrapping an empty data structure in an SD object", t, func() {
		sd := structured.Wrap(nil)

		Convey("Get returns nil", func() {
			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Value(), ShouldEqual, nil)

			Convey("Get with a path returns an error", func() {
				val = sd.Get("some", "path")
				So(val.IsError(), ShouldBeTrue)
				So(errors.IsNotExist(val.Error()), ShouldBeTrue)
				So(val.Value(), ShouldBeNil)
			})
		})
	})

	Convey("After wrapping a data structure in an SD object", t, func() {
		var err error
		data := maputil.From("a", "one", "b", "two")
		sd := structured.Wrap(data)

		Convey("Get returns the structure", func() {
			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Map(), ShouldEqual, data)
			So(sd.Data, ShouldEqual, data)

			Convey("Get with a path returns a subtree of the structure", func() {
				val = sd.Get("a")
				So(val.IsError(), ShouldBeFalse)
				So(val.String(), ShouldEqual, "one")
			})
		})

		Convey("Set allows to replace subtrees", func() {
			err = sd.Set(sd.Path("a"), "x")
			So(err, ShouldBeNil)
			err = sd.Set(sd.Path("b"), "y")
			So(err, ShouldBeNil)

			So(sd.Get("a").String(), ShouldEqual, "x")
			So(sd.Get("b").String(), ShouldEqual, "y")
			So(sd.Get().Map(), ShouldResemble, maputil.From("a", "x", "b", "y"))
		})

		Convey("Merge allows to combine structures", func() {
			err = sd.Merge(nil, maputil.From("c", []interface{}{"c1", "c2", "c3"}))
			So(err, ShouldBeNil)
			So(sd.Get().Map(), ShouldResemble, maputil.From("a", "one", "b", "two", "c", []interface{}{"c1", "c2", "c3"}))

			Convey("Query allows to filter structures", func() {
				val = sd.Query("/c/1")
				So(val.IsError(), ShouldBeFalse)
				So(val.String(), ShouldEqual, "c2")
			})
		})

		Convey("Delete removes subtrees", func() {
			err = sd.Delete("a")
			So(err, ShouldBeNil)
			So(sd.Get().Value(), ShouldResemble, maputil.From("b", "two"))

			err = sd.Delete("b")
			So(err, ShouldBeNil)
			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Value(), ShouldResemble, map[string]interface{}{})
		})

		Convey("Delete with the root path removes all data", func() {
			err = sd.Delete()
			So(err, ShouldBeNil)
			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Value(), ShouldBeNil)
		})

		Convey("Clear removes all data", func() {
			err = sd.Clear()
			So(err, ShouldBeNil)

			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Value(), ShouldBeNil)
		})
	})
}
