package httputil

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/format/structured"
)

// StringQuery retrieves the given query parameter from the query as a string. Returns the default value if the query
// parameter does not exist.
func StringQuery(query url.Values, name string, defaultValue string) string {
	values, exist := query[name]
	if !exist || len(values) == 0 {
		return defaultValue
	}
	return values[0]
}

// BoolQuery retrieves the given query parameter from the query as a boolean. Returns the default value if the query
// parameter does not exist.
func BoolQuery(query url.Values, name string, defaultValue bool) bool {
	values, ok := query[name]
	if !ok || len(values) == 0 {
		return defaultValue
	}
	return StringToBool(values[0], false)
}

// ArrayQueryWithSplit retrieves the given query parameter from the query as an array of strings. "splitChar"
// indicates the character to use to split the query parameter into an array of strings. If several query parameters of
// the same name are found, their results are concatenated.
func ArrayQueryWithSplit(query url.Values, name string, splitChar string) []string {
	values := query[name]
	var result []string
	for _, el1 := range values {
		for _, el2 := range strings.Split(el1, splitChar) {
			result = append(result, el2)
		}
	}
	return result
}

// IntQuery retrieves the given query parameter from the query as an integer. Returns the default value if the query
// parameter does not exist or is not an int.
func IntQuery(query url.Values, name string, defaultValue int) int {
	str := query.Get(name)
	res, err := strconv.Atoi(str)
	if err != nil {
		return defaultValue
	}
	return res
}

// StringToBool converts a string to a boolean value. Returns the default value if value is invalid or empty.
func StringToBool(value string, defaultValue bool) bool {
	switch strings.ToLower(value) {
	case "true", "t", "yes", "y", "1", "":
		return true
	}
	return defaultValue
}

// Float64Query retrieves the given query parameter from the query as a float64. Returns the default value if the query
func Float64Query(query url.Values, name string, defaultValue float64) float64 {
	str := query.Get(name)
	res, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return defaultValue
	}
	return res
}

// DurationQuery retrieves the given query parameter from the query as a string. Returns the default value if the query
// parameter does not exist.
func DurationQuery(query url.Values, name string, unit duration.Spec, defaultValue duration.Spec) duration.Spec {
	values, exist := query[name]
	if !exist || len(values) == 0 {
		return defaultValue
	}
	return structured.Wrap(values[0]).Duration(unit, defaultValue)
}

// UintPtrQuery returns a pointer to a uint for the given query parameter. Returns nil if the parameter does not exist
// or if it's not a valid uint.
func UintPtrQuery(query url.Values, name string) *uint {
	str := query.Get(name)
	res, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return nil
	}
	resUint := uint(res)
	return &resUint
}
