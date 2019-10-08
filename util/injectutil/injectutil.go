package injectutil

import "github.com/eluv-io/inject-go"

// Call executes inj.Call(fn) with additional error handling logic: if the
// function call returns a non-nil error as last result, that error is returned
// as error by this function (together with nil as result slice).
//
// In contrast, the default inj.Call() only returns errors that arise from
// injection problems (e.g. function params that are not bound) as error,
// but does not examine the result values for errors.
//
// See unit tests for examples.
func Call(inj inject.Injector, fn interface{}) ([]interface{}, error) {
	res, err := inj.Call(fn)
	if err != nil {
		return nil, err
	}
	if len(res) > 0 {
		if resErr, isErr := res[len(res)-1].(error); isErr {
			return nil, resErr
		}
	}
	return res, nil
}
