package structured

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var flattenTests = []struct {
	name string
	json interface{}
	sep  string
	flat [][3]string
}{
	{
		name: "nil",
		json: nil,
		flat: [][3]string{{"/", "null", "null"}},
	},
	{
		name: "small",
		json: jm{
			"a": "va",
			"b": "vb",
			"c": ja{
				int64(0),
				int64(-1),
				float64(3.2),
				float64(6.4),
				int64(4),
				int64(5),
				int64(6),
				int64(7),
				int64(8),
				int64(9),
				int64(10),
				int64(11),
				int64(12),
			},
			"d": nil,
		},
		flat: [][3]string{
			{"/", "{}", "object"},
			{"/a", "va", "string"},
			{"/b", "vb", "string"},
			{"/c", "[]", "array"},
			{"/c/0", "0", "int"},
			{"/c/1", "-1", "int"},
			{"/c/2", "3.200000", "float"},
			{"/c/3", "6.400000", "float"},
			{"/c/4", "4", "int"},
			{"/c/5", "5", "int"},
			{"/c/6", "6", "int"},
			{"/c/7", "7", "int"},
			{"/c/8", "8", "int"},
			{"/c/9", "9", "int"},
			{"/c/10", "10", "int"},
			{"/c/11", "11", "int"},
			{"/c/12", "12", "int"},
			{"/d", "null", "null"},
		},
		//json: jm{"a": "va", "b": "vb", "c": ja{0, int8(1), int16(2), int32(3), int64(4), uint(0), uint8(1), uint16(2), uint32(3), uint64(4), float32(3.2), float64(6.4)}},
		//flat: [][3]string{
		//	{"/", "{}", "object"},
		//	{"/a", "va", "string"},
		//	{"/b", "vb", "string"},
		//	{"/c", "[]", "array"},
		//	{"/c/0", "0", "int"},
		//	{"/c/1", "1", "int8"},
		//	{"/c/2", "2", "int16"},
		//	{"/c/3", "3", "int32"},
		//	{"/c/4", "4", "int64"},
		//	{"/c/5", "0", "uint"},
		//	{"/c/6", "1", "uint8"},
		//	{"/c/7", "2", "uint16"},
		//	{"/c/8", "3", "uint32"},
		//	{"/c/9", "4", "uint64"},
		//	{"/c/10", "3.200000", "float32"},
		//	{"/c/11", "6.400000", "float64"},
		//},
	},
	{
		name: "full",
		json: parse(testJson),
		flat: [][3]string{
			{"/", "{}", "object"},
			{"/expensive", "10", "number"},
			{"/store", "{}", "object"},
			{"/store/bicycle", "{}", "object"},
			{"/store/bicycle/color", "red", "string"},
			{"/store/bicycle/price", "19.95", "number"},
			{"/store/books", "[]", "array"},
			{"/store/books/0", "{}", "object"},
			{"/store/books/0/author", "Nigel Rees", "string"},
			{"/store/books/0/category", "reference", "string"},
			{"/store/books/0/price", "8.95", "number"},
			{"/store/books/0/title", "Sayings of the Century", "string"},
			{"/store/books/1", "{}", "object"},
			{"/store/books/1/author", "Evelyn Waugh", "string"},
			{"/store/books/1/category", "fiction", "string"},
			{"/store/books/1/price", "12.99", "number"},
			{"/store/books/1/title", "Sword of Honour", "string"},
			{"/store/books/2", "{}", "object"},
			{"/store/books/2/author", "Herman Melville", "string"},
			{"/store/books/2/category", "fiction", "string"},
			{"/store/books/2/isbn", "0-553-21311-3", "string"},
			{"/store/books/2/price", "8.99", "number"},
			{"/store/books/2/title", "Moby Dick", "string"},
			{"/store/books/3", "{}", "object"},
			{"/store/books/3/author", "J. R. R. Tolkien", "string"},
			{"/store/books/3/category", "fiction", "string"},
			{"/store/books/3/isbn", "0-395-19395-8", "string"},
			{"/store/books/3/price", "22.99", "number"},
			{"/store/books/3/title", "The Lord of the Rings", "string"},
		},
	},
	{
		name: "special chars",
		json: jm{"a/a": "va/va", "b/b": "vb/vb", "c~c": ja{int64(0), int64(1), int64(2), "val/with/slashes~and~tildes"}},
		flat: [][3]string{
			{"/", "{}", "object"},
			{"/a~1a", "va/va", "string"},
			{"/b~1b", "vb/vb", "string"},
			{"/c~0c", "[]", "array"},
			{"/c~0c/0", "0", "int"},
			{"/c~0c/1", "1", "int"},
			{"/c~0c/2", "2", "int"},
			{"/c~0c/3", "val/with/slashes~and~tildes", "string"},
		},
	},
}

func TestFlatten(t *testing.T) {
	for _, test := range flattenTests {
		t.Run(test.name, func(t *testing.T) {
			var res [][3]string
			var err error
			if test.sep == "" {
				res, err = Flatten(test.json)
			} else {
				res, err = Flatten(test.json, test.sep)
			}
			require.NoError(t, err)

			// dumpAsGoCode(res)

			require.Equal(t, len(test.flat), len(res))

			for idx, r := range res {
				// fmt.Printf("%-30s %-30s %s\n", r[0], r[1], r[2])
				assert.Equal(t, test.flat[idx][0], r[0])
				assert.Equal(t, test.flat[idx][1], r[1])
				assert.Equal(t, test.flat[idx][2], r[2])
			}
		})
	}
}

func dumpAsGoCode(res [][3]string) {
	fmt.Printf("[][3]string {\n")
	for _, triplet := range res {
		fmt.Printf(`{"%s", "%s", "%s"},`, triplet[0], triplet[1], triplet[2])
		fmt.Println()
	}
	fmt.Printf("}\n")
}
