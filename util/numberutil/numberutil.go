package numberutil

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/duration"
)

// AsInt64 returns the given value as an int.
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
	case uint:
		result = int64(x)
	case uint8:
		result = int64(x)
	case uint16:
		result = int64(x)
	case uint32:
		result = int64(x)
	case uint64:
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
	case time.Duration:
		result = int64(x)
	case duration.Spec:
		result = int64(x)
	case big.Rat:
		f, _ := x.Float64()
		result = int64(math.Round(f))
	case *big.Rat:
		f, _ := x.Float64()
		result = int64(math.Round(f))
	}
	return result, nil
}

func AsInt(val interface{}) int {
	return int(AsInt64(val))
}

// AsUInt64 returns the given value as an uint.
// If the value is not an int or nil, it returns the 'empty' int 0.
func AsUInt64(val interface{}) uint64 {
	res, err := AsUInt64Err(val)
	if err != nil {
		return 0
	}
	return res
}

// AsUInt64Err returns the given value as an uint, trying to convert it from other
// number types, string or json.Number. Returns an error if the conversion fails.
func AsUInt64Err(val interface{}) (uint64, error) {
	e := errors.Template("AsUInt64", errors.K.Invalid, "value", val)
	if val == nil {
		return 0, e(errors.K.NotExist)
	}
	var result uint64
	var err error
	switch x := val.(type) {
	case string:
		result, err = strconv.ParseUint(x, 10, 64)
		if err != nil {
			return 0, e(err)
		}
	case int:
		result = uint64(x)
	case int8:
		result = uint64(x)
	case int16:
		result = uint64(x)
	case int32:
		result = uint64(x)
	case int64:
		result = uint64(x)
	case uint:
		result = uint64(x)
	case uint8:
		result = uint64(x)
	case uint16:
		result = uint64(x)
	case uint32:
		result = uint64(x)
	case uint64:
		result = uint64(x)
	case float32:
		result = uint64(math.Round(float64(x)))
	case float64:
		result = uint64(math.Round(x))
	case json.Number:
		int_result, err := x.Int64()
		if err != nil {
			return 0, e(err)
		}
		result = uint64(int_result)
	}
	return result, nil
}

func AsUInt(val interface{}) uint {
	return uint(AsUInt64(val))
}

// AsFloat64 returns the given value as a float64.
// If the value is not a number or nil, it returns the zero value float64 0.
func AsFloat64(val interface{}) float64 {
	res, err := AsFloat64Err(val)
	if err != nil {
		return 0
	}
	return res
}

// AsFloat64Err returns the given value as a float64, trying to convert it from
// other number types, string or json.Number. Returns an error if the conversion
// fails.
func AsFloat64Err(val interface{}) (float64, error) {
	e := errors.Template("AsFloat64", errors.K.Invalid, "value", val)
	if val == nil {
		return 0, e(errors.K.NotExist)
	}
	var result float64
	var err error
	switch x := val.(type) {
	case string:
		result, err = strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, e(err)
		}
	case int:
		result = float64(x)
	case int8:
		result = float64(x)
	case int16:
		result = float64(x)
	case int32:
		result = float64(x)
	case int64:
		result = float64(x)
	case uint:
		result = float64(x)
	case uint8:
		result = float64(x)
	case uint16:
		result = float64(x)
	case uint32:
		result = float64(x)
	case uint64:
		result = float64(x)
	case float32:
		result = float64(x)
	case float64:
		result = x
	case json.Number:
		result, err = x.Float64()
		if err != nil {
			return 0, e(err)
		}
	case big.Rat:
		result, _ = x.Float64()
	case *big.Rat:
		result, _ = x.Float64()
	}
	return result, nil
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

// LessInt compares two integer values and returns
//	* true  if i1 < i2
//	* false if i1 > i2
// 	* true or the result of the tie function if i1 == i2
// If ascending is false, the sense of the comparison is inverted, effectively
// returning the result for "more".
func LessInt(ascending bool, i1 int, i2 int, tie ...func() bool) (less bool) {
	if i1 < i2 {
		return ascending
	}
	if i1 > i2 {
		return !ascending
	}
	if len(tie) == 0 || tie[0] == nil {
		return ascending
	}
	return tie[0]()
}

func ParseFloat32(s string) (f float32, err error) {
	var f64 float64
	if f64, err = strconv.ParseFloat(s, 32); err == nil {
		f = float32(f64)
	}
	return
}

func RatModulo(num, denom *big.Rat) (mod *big.Rat, err error) {
	var quo *big.Rat
	if quo, err = RatQuoSafe(num, denom); err == nil {
		quoMod := quo.Num().Int64() % quo.Denom().Int64()
		// The percentage of the denominator left over
		ratio := big.NewRat(quoMod, quo.Denom().Int64())
		mod = ratio.Mul(ratio, denom)
	}
	return
}

func RatQuoSafe(num, den *big.Rat) (quo *big.Rat, err error) {
	quo = big.NewRat(-1, 1)
	if den.Sign() == 0 {
		return nil, errors.E("RatQuotient", errors.K.Invalid, "reason", "divide by zero")
	}
	quo.Quo(num, den)
	return
}
