package bytesize_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/bytesize"
)

func TestHumanReadable(t *testing.T) {
	tests := []struct {
		hr bytesize.HR
	}{
		{hr: bytesize.HR(bytesize.MB)},
		{hr: bytesize.HR(1_500_000)},
		{hr: bytesize.HR(0)},
		{hr: bytesize.HR(1)},
		{hr: bytesize.HR(99.123 * float64(bytesize.PB))},
	}
	for _, test := range tests {
		t.Run(test.hr.String(), func(t *testing.T) {
			str := test.hr.String()
			parsed, err := bytesize.FromString(str)
			require.NoError(t, err)
			require.Equal(t, test.hr, bytesize.HR(parsed))

			marshaled, err := json.Marshal(test.hr)
			require.NoError(t, err)
			require.Equal(t, `"`+test.hr.String()+`"`, string(marshaled))

			var unmarshaled bytesize.HR
			err = json.Unmarshal(marshaled, &unmarshaled)
			require.NoError(t, err)
			require.Equal(t, test.hr, unmarshaled)

		})
	}
}

func ExampleHR_MarshalText() {
	hr := bytesize.HR(5123456)

	marshalText, _ := hr.MarshalText()
	fmt.Println(string(marshalText))

	type test struct {
		Size bytesize.HR `json:"size"`
	}
	t := test{
		Size: hr,
	}
	marshaled, _ := json.Marshal(&t)
	fmt.Println(string(marshaled))

	// Output:
	// 4.9MB (5123456B)
	// {"size":"4.9MB (5123456B)"}
}
