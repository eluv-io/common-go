package lru_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/format/duration"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/lru"
)

func TestExpiringCache(t *testing.T) {
	defer utc.ResetNow()

	now := utc.Now()
	utc.MockNowFn(func() utc.UTC {
		return now
	})

	cache := lru.NewExpiringCache(10, duration.Spec(5*time.Second))

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

func TestExpiringCacheResetOnAccess(t *testing.T) {
	defer utc.ResetNow()

	now := utc.Now()
	utc.MockNowFn(func() utc.UTC {
		return now
	})

	cache := lru.NewExpiringCache(10, duration.Spec(5*time.Second)).WithResetAgeOnAccess(true)

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
	assertGet := func(key interface{}, wantVal interface{}, wantExists bool) {
		res, ok := cache.Get(key)
		require.Equal(t, wantExists, ok)
		require.Equal(t, wantVal, res)
	}

	assertGoC("k1", cstr("v1"), "v1", false)

	now = now.Add(time.Second)

	assertGet("k1", "v1", true)
	assertGet("k1", "v1", true)
	assertGoC("k2", cstr("v2"), "v2", false)
	assertGet("k2", "v2", true)

	now = now.Add(4 * time.Second)

	// still not expired, because age was reset on last access...
	assertGet("k1", "v1", true)
	assertGet("k2", "v2", true)

	now = now.Add(5 * time.Second)

	assertGoC("k1", cstr("v1.1"), "v1.1", false)
	assertGoC("k2", cstr("v2.1"), "v2.1", false)

	now = now.Add(5 * time.Second)

	assertGet("k1", nil, false)
	assertGoC("k2", cstr("v2.2"), "v2.2", false)
	assertGet("k2", "v2.2", true)

	now = now.Add(5 * time.Second)
	assertGet("k1", nil, false)
	assertGet("k2", nil, false)

	cache.Add("k3", "v3")
	assertGet("k3", "v3", true)

	now = now.Add(5 * time.Second)
	assertGet("k3", nil, false)
}
