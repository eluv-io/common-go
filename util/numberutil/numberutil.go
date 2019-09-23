package numberutil

import (
	"encoding/json"
	"math"
	"strconv"

	"github.com/qluvio/content-fabric/errors"
)

// AsInt returns the given value as an int.
// If the value is not an int or nil, it returns the 'empty' int 0.
func AsInt64(val interface{}) int64 {
	res, err := AsInt64Err(val)
	if err != nil {
		return 0
	}
	return res
}

// AsInt64Err returns the given value as an int, trying to convert it from other
// number types, string or json.Number. Returns an error if the conversion fails.
func AsInt64Err(val interface{}) (int64, error) {
	e := errors.Template("AsInt64", errors.K.Invalid, "value", val)
	if val == nil {
		return 0, e(errors.K.NotExist)
	}
	var result int64
	var err error
	switch x := val.(type) {
	case string:
		result, err = strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0, e(err)
		}
	case int:
		result = int64(x)
	case int8:
		result = int64(x)
	case int16:
		result = int64(x)
	case int32:
		result = int64(x)
	case int64:
		result = int64(x)
	case float32:
		result = int64(math.Round(float64(x)))
	case float64:
		result = int64(math.Round(x))
	case json.Number:
		result, err = x.Int64()
		if err != nil {
			return 0, e(err)
		}
	}
	return result, nil
}

func AsInt(val interface{}) int {
	return int(AsInt64(val))
}

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
