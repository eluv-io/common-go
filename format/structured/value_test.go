package structured_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/eluv-io/errors-go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/util/codecutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/maputil"
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

func TestToBool(t *testing.T) {
	require.True(t, structured.Wrap(true).ToBool())
	require.True(t, structured.Wrap("true").ToBool())
	require.True(t, structured.Wrap("True").ToBool())
	require.True(t, structured.Wrap("TrUe").ToBool())

	require.False(t, structured.Wrap(false).ToBool())
	require.False(t, structured.Wrap("false").ToBool())
	require.False(t, structured.Wrap("False").ToBool())
	require.False(t, structured.Wrap(time.Now()).ToBool())
	require.False(t, structured.Wrap("0").ToBool())
	require.False(t, structured.Wrap("1").ToBool())
}

func TestMarshaling(t *testing.T) {
	tests := []interface{}{
		"a string",
		10.123,
		true,
		map[string]interface{}{
			"k1": "v1",
			"k2": "v2",
		},
		[]interface{}{
			"one",
			"two",
			"tree",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s", tt), func(t *testing.T) {
			val := structured.Wrap(tt)
			bts, err := json.Marshal(val)
			require.NoError(t, err)
			fmt.Println(string(bts))

			{ // test json unmarshal
				var val2 structured.Value
				err = json.Unmarshal(bts, &val2)
				require.NoError(t, err)

				require.Equal(t, val, &val2)
			}

			if _, isMap := tt.(map[string]interface{}); isMap { // test codecutil.Decode
				t.Run(fmt.Sprintf("decode %s", tt), func(t *testing.T) {
					var gen interface{}
					err = json.Unmarshal(bts, &gen)
					require.NoError(t, err)

					var val2 structured.Value
					err = codecutil.MapDecode(gen, &val2)
					require.NoError(t, err)

					require.Equal(t, val, &val2)
				})
			}

		})
	}
}

func TestValueErr(t *testing.T) {
	theErr := errors.Str("test error")

	// success: no error, returns data
	res, err := structured.Wrap("hello").ValueErr()
	require.NoError(t, err)
	require.Equal(t, "hello", res)

	// success: nil data is not an error for ValueErr
	res, err = structured.Wrap(nil).ValueErr()
	require.NoError(t, err)
	require.Nil(t, res)

	// stored error, no default
	res, err = structured.Wrap(nil, theErr).ValueErr()
	require.ErrorIs(t, err, theErr)
	require.Nil(t, res)

	// stored error, with default
	res, err = structured.Wrap(nil, theErr).ValueErr("default")
	require.ErrorIs(t, err, theErr)
	require.Equal(t, "default", res)
}

func TestInt64Err(t *testing.T) {
	theErr := errors.Str("test error")

	// success
	res, err := structured.Wrap(int64(42)).Int64Err()
	require.NoError(t, err)
	require.Equal(t, int64(42), res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).Int64Err()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).Int64Err(99)
	require.NoError(t, err)
	require.Equal(t, int64(99), res)

	// stored error, no default → (0, err)
	res, err = structured.Wrap(nil, theErr).Int64Err()
	require.ErrorIs(t, err, theErr)
	require.Equal(t, int64(0), res)

	// stored error, with default → (default, err)
	res, err = structured.Wrap(nil, theErr).Int64Err(99)
	require.ErrorIs(t, err, theErr)
	require.Equal(t, int64(99), res)

	// wrong type, no default → (0, err)
	res, err = structured.Wrap("not a number").Int64Err()
	require.Error(t, err)
	require.Equal(t, int64(0), res)

	// wrong type, with default → (default, err)
	res, err = structured.Wrap("not a number").Int64Err(99)
	require.Error(t, err)
	require.Equal(t, int64(99), res)
}

func TestIntErr(t *testing.T) {
	// success
	res, err := structured.Wrap(7).IntErr()
	require.NoError(t, err)
	require.Equal(t, 7, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).IntErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).IntErr(5)
	require.NoError(t, err)
	require.Equal(t, 5, res)

	// wrong type
	_, err = structured.Wrap("bad").IntErr()
	require.Error(t, err)
}

func TestUInt64Err(t *testing.T) {
	// success
	res, err := structured.Wrap(uint64(10)).UInt64Err()
	require.NoError(t, err)
	require.Equal(t, uint64(10), res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).UInt64Err()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).UInt64Err(7)
	require.NoError(t, err)
	require.Equal(t, uint64(7), res)

	// wrong type
	_, err = structured.Wrap("bad").UInt64Err()
	require.Error(t, err)
}

func TestUIntErr(t *testing.T) {
	// success
	res, err := structured.Wrap(uint(3)).UIntErr()
	require.NoError(t, err)
	require.Equal(t, uint(3), res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).UIntErr()
	require.True(t, errors.IsNotExist(err))
}

func TestFloat64Err(t *testing.T) {
	// success
	res, err := structured.Wrap(3.14).Float64Err()
	require.NoError(t, err)
	require.InDelta(t, 3.14, res, 1e-9)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).Float64Err()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).Float64Err(2.71)
	require.NoError(t, err)
	require.InDelta(t, 2.71, res, 1e-9)

	// wrong type
	_, err = structured.Wrap("bad").Float64Err()
	require.Error(t, err)
}

func TestStringErr(t *testing.T) {
	theErr := errors.Str("test error")

	// success
	res, err := structured.Wrap("hello").StringErr()
	require.NoError(t, err)
	require.Equal(t, "hello", res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).StringErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).StringErr("fallback")
	require.NoError(t, err)
	require.Equal(t, "fallback", res)

	// stored error → (default, err)
	res, err = structured.Wrap(nil, theErr).StringErr("fallback")
	require.ErrorIs(t, err, theErr)
	require.Equal(t, "fallback", res)

	// wrong type, no default → ("", err)
	s, err := structured.Wrap(42).StringErr()
	require.Error(t, err)
	require.Equal(t, "", s)

	// wrong type, with default → (default, err)
	s, err = structured.Wrap(42).StringErr("fallback")
	require.Error(t, err)
	require.Equal(t, "fallback", s)
}

func TestToStringErr(t *testing.T) {
	theErr := errors.Str("test error")

	// success: converts non-string types
	res, err := structured.Wrap(42).ToStringErr()
	require.NoError(t, err)
	require.Equal(t, "42", res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).ToStringErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).ToStringErr("fallback")
	require.NoError(t, err)
	require.Equal(t, "fallback", res)

	// stored error
	_, err = structured.Wrap(nil, theErr).ToStringErr()
	require.ErrorIs(t, err, theErr)
}

func TestStringSliceErr(t *testing.T) {
	theErr := errors.Str("test error")

	// success
	res, err := structured.Wrap([]string{"a", "b"}).StringSliceErr()
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).StringSliceErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).StringSliceErr("x", "y")
	require.NoError(t, err)
	require.Equal(t, []string{"x", "y"}, res)

	// stored error, with default → (default, err)
	res, err = structured.Wrap(nil, theErr).StringSliceErr("x", "y")
	require.ErrorIs(t, err, theErr)
	require.Equal(t, []string{"x", "y"}, res)

	// wrong type
	_, err = structured.Wrap("not a slice").StringSliceErr()
	require.Error(t, err)
}

func TestMapErr(t *testing.T) {
	theErr := errors.Str("test error")

	m := map[string]interface{}{"k": "v"}

	// success
	res, err := structured.Wrap(m).MapErr()
	require.NoError(t, err)
	require.Equal(t, m, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).MapErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	def := map[string]interface{}{"default": true}
	res, err = structured.Wrap(nil).MapErr(def)
	require.NoError(t, err)
	require.Equal(t, def, res)

	// stored error, with default → (default, err)
	res, err = structured.Wrap(nil, theErr).MapErr(def)
	require.ErrorIs(t, err, theErr)
	require.Equal(t, def, res)

	// wrong type
	_, err = structured.Wrap("not a map").MapErr()
	require.Error(t, err)
}

func TestSliceErr(t *testing.T) {
	theErr := errors.Str("test error")

	sl := []interface{}{"a", 1}

	// success
	res, err := structured.Wrap(sl).SliceErr()
	require.NoError(t, err)
	require.Equal(t, sl, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).SliceErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).SliceErr("x", 2)
	require.NoError(t, err)
	require.Equal(t, []interface{}{"x", 2}, res)

	// stored error, with default → (default, err)
	res, err = structured.Wrap(nil, theErr).SliceErr("x")
	require.ErrorIs(t, err, theErr)
	require.Equal(t, []interface{}{"x"}, res)

	// wrong type
	_, err = structured.Wrap("not a slice").SliceErr()
	require.Error(t, err)
}

func TestBoolErr(t *testing.T) {
	// success
	res, err := structured.Wrap(true).BoolErr()
	require.NoError(t, err)
	require.True(t, res)

	res, err = structured.Wrap(false).BoolErr()
	require.NoError(t, err)
	require.False(t, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).BoolErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).BoolErr(true)
	require.NoError(t, err)
	require.True(t, res)

	// wrong type
	_, err = structured.Wrap("not a bool").BoolErr()
	require.Error(t, err)
}

func TestToBoolErr(t *testing.T) {
	// bool values
	res, err := structured.Wrap(true).ToBoolErr()
	require.NoError(t, err)
	require.True(t, res)

	res, err = structured.Wrap(false).ToBoolErr()
	require.NoError(t, err)
	require.False(t, res)

	// string parsing
	res, err = structured.Wrap("true").ToBoolErr()
	require.NoError(t, err)
	require.True(t, res)

	res, err = structured.Wrap("False").ToBoolErr()
	require.NoError(t, err)
	require.False(t, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).ToBoolErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).ToBoolErr(true)
	require.NoError(t, err)
	require.True(t, res)

	// unparseable value, no default → (false, err)
	_, err = structured.Wrap("not-a-bool").ToBoolErr()
	require.Error(t, err)

	// unparseable value, with default → (default, err)
	res, err = structured.Wrap("not-a-bool").ToBoolErr(true)
	require.Error(t, err)
	require.True(t, res)
}

func TestUTCErr(t *testing.T) {
	theErr := errors.Str("test error")
	now := utc.Now()

	// utc.UTC value: returned as-is
	res, err := structured.Wrap(now).UTCErr()
	require.NoError(t, err)
	require.Equal(t, now, res)

	// time.Time value: just verify it succeeds without error
	_, err = structured.Wrap(time.Now()).UTCErr()
	require.NoError(t, err)

	// string value: parsed successfully (value comparison omitted due to serialization precision)
	_, err = structured.Wrap(now.String()).UTCErr()
	require.NoError(t, err)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).UTCErr()
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).UTCErr(now)
	require.NoError(t, err)
	require.Equal(t, now, res)

	// stored error
	_, err = structured.Wrap(nil, theErr).UTCErr()
	require.ErrorIs(t, err, theErr)

	// unparseable string
	_, err = structured.Wrap("not-a-time").UTCErr()
	require.Error(t, err)
}

func TestDurationErr(t *testing.T) {
	theErr := errors.Str("test error")

	// success: duration.Spec value
	res, err := structured.Wrap(duration.Hour).DurationErr(duration.Nanosecond)
	require.NoError(t, err)
	require.Equal(t, duration.Hour, res)

	// success: numeric value scaled by unit
	res, err = structured.Wrap(1).DurationErr(duration.Second)
	require.NoError(t, err)
	require.Equal(t, duration.Second, res)

	// success: string value
	res, err = structured.Wrap("3m").DurationErr(duration.Second)
	require.NoError(t, err)
	require.Equal(t, 3*duration.Minute, res)

	// nil data, no default → NotExist
	_, err = structured.Wrap(nil).DurationErr(duration.Second)
	require.True(t, errors.IsNotExist(err))

	// nil data, with default → (default, nil)
	res, err = structured.Wrap(nil).DurationErr(duration.Second, duration.Hour)
	require.NoError(t, err)
	require.Equal(t, duration.Hour, res)

	// stored error, with default → (default, err)
	res, err = structured.Wrap(nil, theErr).DurationErr(duration.Second, duration.Hour)
	require.ErrorIs(t, err, theErr)
	require.Equal(t, duration.Hour, res)

	// invalid string conversion
	_, err = structured.Wrap("an invalid string").DurationErr(duration.Second)
	require.Error(t, err)

	// invalid string, with default → (default, err)
	res, err = structured.Wrap("an invalid string").DurationErr(duration.Second, duration.Hour)
	require.Error(t, err)
	require.Equal(t, duration.Hour, res)
}

func TestValue_Duration(t *testing.T) {
	// invalid conversions
	require.Equal(t, duration.Zero, structured.Wrap(nil).Duration(duration.Second))
	require.Equal(t, duration.Zero, structured.Wrap(nil, errors.Str("an error")).Duration(duration.Second))
	require.Equal(t, duration.Zero, structured.Wrap("an invalid string").Duration(duration.Second))
	// invalid conversions, return default value
	require.Equal(t, duration.Hour, structured.Wrap(nil).Duration(duration.Second, duration.Hour))
	require.Equal(t, duration.Hour, structured.Wrap(nil, errors.Str("an error")).Duration(duration.Second, duration.Hour))
	require.Equal(t, duration.Hour, structured.Wrap("an invalid string").Duration(duration.Second, duration.Hour))

	require.Equal(t, duration.Zero, structured.Wrap(0).Duration(duration.Second))
	require.Equal(t, duration.Zero, structured.Wrap("0").Duration(duration.Second))
	require.Equal(t, duration.Zero, structured.Wrap("").Duration(duration.Second))

	require.Equal(t, duration.Hour, structured.Wrap(duration.Hour).Duration(duration.Nanosecond))
	require.Equal(t, duration.Second, structured.Wrap(1).Duration(duration.Second))
	require.Equal(t, duration.Second, structured.Wrap("1").Duration(duration.Second))
	require.Equal(t, 99*duration.Second, structured.Wrap(99.0).Duration(duration.Second))
	require.Equal(t, 3*duration.Minute, structured.Wrap("3m").Duration(duration.Second))
	require.Equal(t, 90*duration.Second, structured.Wrap(json.Number("1.5")).Duration(duration.Minute))
	require.Equal(t, 90*duration.Second, structured.Wrap(1.5).Duration(duration.Minute))
	require.Equal(t, 90*duration.Second, structured.Wrap(big.NewRat(3, 2)).Duration(duration.Minute))
}
