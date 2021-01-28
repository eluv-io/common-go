package lru_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/format/utc"

	"github.com/qluvio/content-fabric/util/lru"
)

func TestExpiringCache(t *testing.T) {
	defer utc.ResetNow()

	now := utc.Now()
	utc.MockNowFn(func() utc.UTC {
		return now
	})

	cache := lru.NewExpiringCache(10, 5*time.Second)

	cstr := func(v interface{}) func() (interface{}, error) {
		return func() (interface{}, error) {
			return v, nil
		}
	}
	assertGoC := func(key interface{}, constructor func() (interface{}, error), wantVal interface{}, wantEviction bool) {
		res, evicted, err := cache.GetOrCreate(key, constructor)
		require.NoError(t, err)
		require.Equal(t, wantVal, res)
		require.Equal(t, wantEviction, evicted)
	}

	assertGoC("k1", cstr("v1"), "v1", false)

	now = now.Add(time.Second)

	assertGoC("k1", nil, "v1", false)
	assertGoC("k1", nil, "v1", false)
	assertGoC("k2", cstr("v2"), "v2", false)
	assertGoC("k2", nil, "v2", false)

	now = now.Add(4 * time.Second)

	assertGoC("k1", cstr("v1.1"), "v1.1", false)
	assertGoC("k2", cstr("v2"), "v2", false)

	now = now.Add(time.Second)

	assertGoC("k2", cstr("v2.1"), "v2.1", false)
}
