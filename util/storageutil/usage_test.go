package storageutil_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/storageutil"
)

func TestGetUsage(t *testing.T) {
	usage, err := storageutil.GetUsage(".")
	fmt.Println(usage)
	require.NoError(t, err)
	require.NotEmpty(t, usage.Capacity)
	require.EqualValues(t, usage.Capacity, usage.Used+usage.Free, usage)
}
