package netutil

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUrlJSON(t *testing.T) {
	type A struct {
		U *URL `json:"url"`
	}

	mustParse := func(s string) *url.URL {
		ret, err := url.Parse(s)
		if err != nil {
			panic(err)
		}
		return ret
	}

	type testCase struct {
		name string
		a    *A
	}
	for _, tc := range []*testCase{
		{name: "no url", a: &A{U: &URL{}}},
		{name: "wrong url", a: &A{U: &URL{URL: mustParse("http:127.0.0.1:8008")}}},
		{name: "with url", a: &A{U: &URL{URL: mustParse("http://127.0.0.1:8008")}}},
		{name: "with query", a: &A{U: &URL{URL: mustParse("http://127.0.0.1:8008?authorization=eyjbAA")}}},
	} {
		bb, err := json.Marshal(tc.a)
		require.NoError(t, err, tc.name)
		fmt.Println(tc.name, string(bb))
		a := &A{}
		err = json.Unmarshal(bb, a)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.a.U, a.U, tc.name)
	}

}
