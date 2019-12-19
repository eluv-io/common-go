package structured

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/jsonutil"
)

func TestFilter_slashToDot(t *testing.T) {
	tests := []struct {
		query string
		exp   string
	}{
		{
			query: `/`,
			exp:   `$`,
		},
		{
			query: `/store`,
			exp:   `$.store`,
		},
		{
			query: `/store/books/3/price`,
			exp:   `$.store.books[3].price`,
		},
		{
			query: `/store/books/?(@.price > 10)/title`,
			exp:   `$.store.books[?(@.price > 10)].title`,
		},
		{
			query: `/store/books/?(@.category=="reference")/title`,
			exp:   `$.store.books[?(@.category=="reference")].title`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := slashToDot(tt.query)
			require.Equal(t, tt.exp, r)
		})
	}
}

func TestFilter_slashToDot2(t *testing.T) {
	tests := []struct {
		query string
		exp   string
	}{
		{
			query: `/`,
			exp:   `$`,
		},
		{
			query: `/store`,
			exp:   `$['store']`,
		},
		{
			query: `/store/books[3]/price`,
			exp:   `$['store']['books'][3]['price']`,
		},
		{
			query: `/store/books/3/price`,
			exp:   `$['store']['books'][3]['price']`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := slashToDot2(tt.query)
			require.Equal(t, tt.exp, r)
		})
	}
}

func TestFilter_Apply(t *testing.T) {
	tests := []struct {
		query string
		json  interface{}
		exp   string
	}{
		{
			query: "/",
			json:  parse(testJson),
			exp:   compact(testJson),
		},
		{
			query: "/expensive",
			json:  parse(testJson),
			exp:   `10`,
		},
		{
			query: "$.expensive",
			json:  parse(testJson),
			exp:   `10`,
		},
		{
			query: "/store/bicycle",
			json:  parse(testJson),
			exp:   `{"color":"red","price":19.95}`,
		},
		{
			query: "$.store.bicycle",
			json:  parse(testJson),
			exp:   `{"color":"red","price":19.95}`,
		},
		{
			query: "/store/books[0]",
			json:  parse(testJson),
			exp:   `{"author":"Nigel Rees","category":"reference","price":8.95,"title":"Sayings of the Century"}`,
		},
		{
			query: "/store/books/3/price",
			json:  parse(testJson),
			exp:   `22.99`,
		},
		{
			query: "/store/books[3]/price",
			json:  parse(testJson),
			exp:   `22.99`,
		},
		{
			query: "/store/books[0:2]/price",
			json:  parse(testJson),
			exp:   `[8.95,12.99]`,
		},
		{
			query: `/store/books[?(@.category=="reference")]/title`,
			json:  parse(testJson),
			exp:   `["Sayings of the Century"]`,
		},
		{
			query: `/store/books[?(@.price > 10)]/title`,
			json:  parseFloat64(testJson),
			exp:   `["Sword of Honour","The Lord of the Rings"]`,
		},
		{
			query: `/store/books/*/title`,
			json:  parse(testJson),
			exp:   `["Sayings of the Century","Sword of Honour","Moby Dick","The Lord of the Rings"]`,
		},
		{
			query: `/store//title`,
			json:  parse(testJson),
			exp:   `["Sayings of the Century","Sword of Honour","Moby Dick","The Lord of the Rings"]`,
		},
		// this test does not work reliably because the used jsonpath lib returns items in arbitrary oder...
		//{
		//	query: `//price`,
		//	json:  parse(testJson),
		//	exp: [][3]string{
		//		{"/", "[]", "array"},
		//		{"/0", "8.95", "number"},
		//		{"/1", "12.99", "number"},
		//		{"/2", "8.99", "number"},
		//		{"/3", "22.99", "number"},
		//		{"/4", "19.95", "number"},
		//	},
		//},
	}

	for idx, test := range tests {
		t.Run(fmt.Sprintf("%.2d%s", idx, test.query), func(t *testing.T) {
			f, err := NewFilter(test.query)
			require.NoError(t, err)
			res, err := f.Apply(test.json)
			require.NoError(t, err)

			fmt.Println("query:", test.query)
			fmt.Println(jsonutil.MarshalString(res))

			assert.Equal(t, test.exp, jsonutil.MarshalCompactString(res), "query [%s] native [%s]", test.query, f.Query())
		})
	}
}

func TestFilter_ApplyAndFlatten(t *testing.T) {
	tests := []struct {
		query string
		json  interface{}
		exp   [][3]string
	}{
		{
			query: "/store/books/3/price",
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "22.99", "number"},
			},
		},
		{
			query: "$",
			json:  parse(testJson),
		},
		{
			query: "/",
			json:  parse(testJson),
		},
		{
			query: "/expensive",
			json:  parse(testJson),
			exp:   [][3]string{{"/", "10", "number"}},
		},
		{
			query: "/store/bicycle",
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "{}", "object"},
				{"/color", "red", "string"},
				{"/price", "19.95", "number"},
			},
		},
		{
			query: "/store/books[0]",
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "{}", "object"},
				{"/author", "Nigel Rees", "string"},
				{"/category", "reference", "string"},
				{"/price", "8.95", "number"},
				{"/title", "Sayings of the Century", "string"},
			},
		},
		{
			query: "/store/books[3]/price",
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "22.99", "number"},
			},
		},
		{
			query: "/store/books/3/price",
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "22.99", "number"},
			},
		},
		{
			query: "/store/books[0:2]/price",
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "[]", "array"},
				{"/0", "8.95", "number"},
				{"/1", "12.99", "number"},
			},
		},
		{
			query: `/store/books[?(@.category=="reference")]/title`,
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "[]", "array"},
				{"/0", "Sayings of the Century", "string"},
			},
		},
		{
			query: `/store/books[?(@.price > 10)]/title`,
			json:  parseFloat64(testJson),
			// json: parse(testJson),
			exp: [][3]string{
				{"/", "[]", "array"},
				{"/0", "Sword of Honour", "string"},
				{"/1", "The Lord of the Rings", "string"},
			},
		},
		{
			query: `/store/books/*/title`,
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "[]", "array"},
				{"/0", "Sayings of the Century", "string"},
				{"/1", "Sword of Honour", "string"},
				{"/2", "Moby Dick", "string"},
				{"/3", "The Lord of the Rings", "string"},
			},
		},
		{
			query: `/store//title`,
			json:  parse(testJson),
			exp: [][3]string{
				{"/", "[]", "array"},
				{"/0", "Sayings of the Century", "string"},
				{"/1", "Sword of Honour", "string"},
				{"/2", "Moby Dick", "string"},
				{"/3", "The Lord of the Rings", "string"},
			},
		},
		// this test does not work reliably because the used jsonpath lib returns items in arbitrary oder...
		//{
		//	query: `//price`,
		//	json:  parse(testJson),
		//	exp: [][3]string{
		//		{"/", "[]", "array"},
		//		{"/0", "8.95", "number"},
		//		{"/1", "12.99", "number"},
		//		{"/2", "8.99", "number"},
		//		{"/3", "22.99", "number"},
		//		{"/4", "19.95", "number"},
		//	},
		//},
	}

	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			f, err := NewFilter(test.query)
			require.NoError(t, err)
			res, err := f.ApplyAndFlatten(test.json)
			require.NoError(t, err)

			if test.exp != nil {
				assert.Equal(t, len(test.exp), len(res), "query [%s] native [%s]", test.query, f.Query())
			}

			for idx, r := range res {
				// fmt.Printf("%-30s %-30s %s\n", r[0], r[1], r[2])
				if test.exp != nil {
					assert.Equal(t, test.exp[idx][0], r[0], "query [%s] index [%d]", test.query, idx)
					assert.Equal(t, test.exp[idx][1], r[1], "query [%s] index [%d]", test.query, idx)
					assert.Equal(t, test.exp[idx][2], r[2], "query [%s] index [%d]", test.query, idx)
				}
			}

			// fmt.Println()
		})
	}
}

func TestFilter_CombinePathQuery(t *testing.T) {
	tests := []struct {
		path     string
		query    string
		expected string
	}{
		{
			path:     "",
			query:    "query",
			expected: "/query",
		},
		{
			path:     "/",
			query:    "query",
			expected: "/query",
		},
		{
			path:     "",
			query:    "/query",
			expected: "/query",
		},
		{
			path:     "/",
			query:    "/query",
			expected: "/query",
		},
		{
			path:     "/path",
			query:    "query",
			expected: "/path/query",
		},
		{
			path:     "/path",
			query:    "/query",
			expected: "/path/query",
		},
		{
			path:     "/path/",
			query:    "query",
			expected: "/path/query",
		},
		{
			path:     "/path/",
			query:    "/query",
			expected: "/path/query",
		},
	}
	for _, test := range tests {
		fmt.Printf("Combine [%s, %s] -> %s\n", test.path, test.query, test.expected)
		assert.Equal(t, test.expected, CombinePathQuery(test.path, test.query))
	}
}
