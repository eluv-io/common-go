package structured

import (
	"encoding/json"
	"strings"

	"github.com/qluvio/content-fabric/util/jsonutil"
)

type jm = map[string]interface{}
type ja = []interface{}

const testJson = `{
    "store": {
        "books": [
            {
                "category": "reference",
                "author": "Nigel Rees",
                "title": "Sayings of the Century",
                "price": 8.95
            },
            {
                "category": "fiction",
                "author": "Evelyn Waugh",
                "title": "Sword of Honour",
                "price": 12.99
            },
            {
                "category": "fiction",
                "author": "Herman Melville",
                "title": "Moby Dick",
                "isbn": "0-553-21311-3",
                "price": 8.99
            },
            {
                "category": "fiction",
                "author": "J. R. R. Tolkien",
                "title": "The Lord of the Rings",
                "isbn": "0-395-19395-8",
                "price": 22.99
            }
        ],
        "bicycle": {
            "color": "red",
            "price": 19.95
        }
    },
    "expensive": 10
}
`

func parse(jsn string) interface{} {
	var unmarshalled interface{}
	d := json.NewDecoder(strings.NewReader(jsn))
	d.UseNumber()
	err := d.Decode(&unmarshalled)
	if err != nil {
		panic(err)
	}
	return unmarshalled
}

func compact(jsn string) string {
	return jsonutil.MarshalCompactString(jsonutil.UnmarshalStringToAny(jsn))
}

func parseFloat64(jsn string) interface{} {
	var unmarshalled interface{}
	err := json.Unmarshal([]byte(jsn), &unmarshalled)
	if err != nil {
		panic(err)
	}
	return unmarshalled
}
