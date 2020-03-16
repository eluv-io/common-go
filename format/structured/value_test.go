package structured_test

import (
	"io"
	"testing"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/util/jsonutil"
	"github.com/qluvio/content-fabric/util/maputil"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestValue_Manip(t *testing.T) {
	var val *structured.Value

	Convey("After wrapping an empty data structure in a Value", t, func() {
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

	Convey("After wrapping a data structure in a Value", t, func() {
		var err error
		data := maputil.From("a", "one", "b", "two")
		// wrap an equivalent structure - keep it separate because otherwise
		// the reference struct will be modified as well!
		sd := structured.Wrap(maputil.From("a", "one", "b", "two"))

		Convey("Get returns the structure", func() {
			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Map(), ShouldResemble, data)
			So(sd.Data, ShouldResemble, data)

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
			deleted := sd.Delete("a")
			So(deleted, ShouldBeTrue)
			So(sd.Get().Value(), ShouldResemble, maputil.From("b", "two"))

			deleted = sd.Delete("b")
			So(deleted, ShouldBeTrue)
			val = sd.Get()
			So(val.IsError(), ShouldBeFalse)
			So(val.Value(), ShouldResemble, map[string]interface{}{})
		})

		Convey("Deleting inexistent paths does nothing", func() {
			deleted := sd.Delete("c", "d", "e")
			So(deleted, ShouldBeFalse)
			So(sd.Get().Value(), ShouldResemble, data)
		})

		Convey("Delete with the root path removes all data", func() {
			deleted := sd.Delete()
			So(deleted, ShouldBeTrue)
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

func TestValueSD(t *testing.T) {
	require.Nil(t, structured.NewValue(nil, nil).Get("/blub").Map()["key"])
	require.Nil(t, structured.NewValue(nil, io.EOF).Get("/blub").Map()["key"])
	require.Equal(t, 23, structured.NewValue(nil, io.EOF).Int(23))
	require.Equal(t, 23, structured.Wrap(nil).Int(23))
	require.Equal(t, 23, structured.Wrap(nil, io.EOF).Int(23))
	require.Equal(t, 23, structured.Wrap(23).Int())
}

func TestGet(t *testing.T) {
	val := structured.Wrap(jsonutil.UnmarshalStringToAny(`
{
  "glossary": {
    "title": "example glossary",
    "div": {
      "title": "S",
      "list": {
        "entry": {
          "id": "SGML",
          "sort_as": "SGML",
          "term": "Standard Generalized Markup Language",
          "acronym": "SGML",
          "abbrev": "ISO 8879:1986",
          "def": {
            "para": "A meta-markup language, used to create markup languages such as DocBook.",
            "see_also": [
              "GML",
              "XML"
            ]
          },
          "see": "markup"
        }
      }
    }
  }
}`))
	tests := []struct {
		path string
		want interface{}
	}{
		{
			path: "/glossary/title",
			want: "example glossary",
		},
		{
			path: "/glossary/div/list/entry/acronym",
			want: "SGML",
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			require.Equal(t, test.want, val.GetP(test.path).Value())
			require.Equal(t, test.want, val.Get(structured.ParsePath(test.path)...).Value())
		})
	}
}

func TestValue_Decode(t *testing.T) {
	type jm = map[string]interface{}
	type TStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	tests := []struct {
		value   *structured.Value
		target  *TStruct
		want    *TStruct
		wantErr bool
	}{
		{structured.Wrap(jm{"name": "Joe", "age": 28}), &TStruct{}, &TStruct{"Joe", 28}, false},
		{structured.Wrap(jm{"name": "Joe"}), &TStruct{}, &TStruct{"Joe", 0}, false},
		{structured.Wrap(nil), &TStruct{}, &TStruct{"", 0}, false},
		{structured.Wrap(jm{"name": "Joe", "age": "twentyeight"}), &TStruct{}, nil, true},
		{structured.Wrap(nil, io.EOF), nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(jsonutil.MarshalCompactString(tt.value), func(t *testing.T) {
			err := tt.value.Decode(tt.target)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, tt.target)
			}
		})
	}
}
