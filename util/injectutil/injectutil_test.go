package injectutil

import (
	"io"
	"testing"

	"github.com/eluv-io/inject-go"
	"github.com/stretchr/testify/require"
)

func TestCall(t *testing.T) {
	mod := inject.NewModule()
	inj, err := inject.NewInjector(mod)
	require.NoError(t, err)

	var res []interface{}

	{
		// void function
		called := false
		res, err = Call(inj, func() {
			called = true
		})
		require.NoError(t, err)
		require.Equal(t, 0, len(res))
		require.True(t, called)
	}
	{
		// no error in function signature
		res, err = Call(inj, func() string {
			return "test"
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(res))
		require.Equal(t, "test", res[0])
	}
	{
		// nil error
		res, err = Call(inj, func() (string, error) {
			return "test", nil
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(res))
		require.Equal(t, "test", res[0])
		require.Equal(t, nil, res[1])
	}
	{
		// non-nil error
		res, err = Call(inj, func() (string, error) {
			return "", io.EOF
		})
		require.Error(t, err)
		require.Equal(t, io.EOF, err)
		require.Equal(t, 0, len(res))
		require.Nil(t, nil, res)
	}
	{
		// non-nil error, with regular inj.Call()
		// ==> err is nil and must be retrieved from result value slice
		res, err = inj.Call(func() (string, error) {
			return "", io.EOF
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(res))
		require.Equal(t, "", res[0])
		require.Equal(t, io.EOF, res[1])
	}

}
