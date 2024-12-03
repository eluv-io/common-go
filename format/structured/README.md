# Package `structured`

Package `structured` provides helper functions to work with generic, JSON-like data. It includes functions for extracting, merging, copying, querying, filtering, flattening, and unflattening data structures. The package also provides a `Value` struct that wraps structured data and offers methods for querying, manipulation, and conversion. 

### Path (path.go)

`Path` provides a set of tools for working with paths, including creation, manipulation, comparison, formatting, and parsing. A path is a slice of string segments and a separator used to format and parse the path. The default separator is `/`. Path also supports escaping the separator in path segments using the mechanism describe in rfc6901, which is done by replacing the separator with `~1` and escaping `~` as `~0`.

### Get / Resolve (resolve.go)

`Get` provides a way to extract data from a generic data structure using a path. The data structure can be a combination of maps, slices, and structs. The path is a string that represents the path to the desired data. The path can contain keys for maps, indices for slices, or field names for structs (supports `json:` struct tags. 

The `Resolve` function is a more flexible version of `Get` that allows customized traversal with a transformer function.

### Visit / Replace (visit.go)

`Visit` traverses a generic data structure in depth-first order and calls the visitor function with each encountered element.

`Replace` is a variant of `Visit` that allows replacing elements in the data structure during traversal.

### Merge (merge.go)

`Merge` allows to merge generic data structures.

### Copy (copy.go)

Creates a "relatively" deep copy of a generic data structure, duplicating simple types, `[]interface{}` and `map[string]interface{}` elements. Any other types like structs, channels, etc. are copied by reference or according to the optional custom copy function.

### Query / Filter (filter.go)

Query provides a limited implementation of JSONPath for querying generic data structures.

### FilterGlob (glob.go)

`FilterGlob` filters a generic data structure according to the provided "select" and "remove" paths. Only elements at "select" paths are included in the result and further reduced by "remove" paths. Removal takes precedence in case of conflicting select and remove paths.

"select" and "remove" paths may contain wildcards '*' in place of path segments, e.g. /a/*/b or /a/*/*/b/*/c. A wildcard therefore represents all keys in a map or all indices in a slice.

### Flatten / Unflatten (flatten.go / unflatten.go

`Flatten` converts the given data structure into a list of triplets `[path, value, type]` consisting of the flattened paths, their corresponding values and type information, for example:

```go
[ "/", "{}", "object"]
[ "/first", "joe", "string" ]
[ "/last", "doe", "string" ]
[ "/age", "24", "number" ]
[ "/children", "[]", "array" ]
[ "/children/0", "fred", "string" ]
[ "/children/1", "cathy", "string" ]
[ "/children/3", "jenny", "string" ]
```

The `Unflatten` function reverses the process of flattening, transforming a list of triplets back into a structured data object.

### Value (value.go)

`Value` combines the various functions of the `structured` package in a struct that wraps generic structured data, offering query, manipulation, and conversion functions for the data:

1. **Data Wrapping and Unwrapping**: The `Wrap` function wraps any data structure into a `Value` object, which can then be manipulated or queried using the methods provided by the `Value` struct. The `Unwrap` function retrieves the raw data from a `Value` object.

2. **JSON Handling**: The `WrapJson` function parses a given JSON document into `interface{}` and returns the result as a `Value`. The `Value` struct also implements `MarshalJSON` and `UnmarshalJSON` methods that operate on the underlying value. Its `Decode` method decodes the `Value` into a target struct using the `mapstructure` package.

3. **Data Manipulation**: The `Value` struct provides methods for setting, merging, and deleting data at a given path. It also provides a `Clear` method to clear the data.

4. **Data Query**: The `Get`, `GetP`, and `At` methods retrieve the value at a given path. The `Query` method applies a filter to the data. Rather than returning errors in case of invalid paths, the error is embedded in the `Value` instance. It can be checked with `IsError` and retrieved with `Error`.

5. **Type Conversion**: The `Value` struct provides methods to access the data as specific types, such as `Int`, `UInt`, `Float64`, `String`, `Map`, `Slice`, `Bool`, `UTC`, `ID`, and `Duration`. Rather than returning errors in case of invalid conversions, they return an optional default or the zero value. 

