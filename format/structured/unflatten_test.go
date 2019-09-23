package structured

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnflatten(t *testing.T) {
	for _, test := range flattenTests {
		t.Run(test.name, func(t *testing.T) {
			var res interface{}
			var err error
			if test.sep == "" {
				res, err = Unflatten(test.flat)
			} else {
				res, err = Unflatten(test.flat, test.sep)
			}
			require.NoError(t, err)

			assert.Equal(t, test.json, res)
		})
	}
}

func TestFlattenUnflatten(t *testing.T) {
	for _, test := range flattenTests {
		t.Run(test.name, func(t *testing.T) {
			var flat [][3]string
			var err error

			if test.sep == "" {
				flat, err = Flatten(test.json)
			} else {
				flat, err = Flatten(test.json, test.sep)
			}
			require.NoError(t, err)

			var unflattened interface{}
			if test.sep == "" {
				unflattened, err = Unflatten(flat)
			} else {
				unflattened, err = Unflatten(flat, test.sep)
			}
			require.NoError(t, err)

			assert.Equal(t, test.json, unflattened)
		})
	}
}

func TestUnflattenFlatten(t *testing.T) {
	for _, test := range flattenTests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			var unflattened interface{}
			if test.sep == "" {
				unflattened, err = Unflatten(test.flat)
			} else {
				unflattened, err = Unflatten(test.flat, test.sep)
			}
			require.NoError(t, err)

			assert.Equal(t, test.json, unflattened)

			var flat [][3]string

			if test.sep == "" {
				flat, err = Flatten(unflattened)
			} else {
				flat, err = Flatten(unflattened, test.sep)
			}

			assert.Equal(t, flat, test.flat)
			require.NoError(t, err)

		})
	}
}
