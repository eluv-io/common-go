package stackutil_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/stackutil"
)

func TestSnapshot(t *testing.T) {
	stackb, err := os.ReadFile("testdata/stacktrace1.txt")
	require.NoError(t, err)
	stack := string(stackb)

	snapshot, err := stackutil.NewSnapshot(stack)
	require.NoError(t, err)

	require.Equal(t, 587, len(snapshot.Goroutines))

	removed := snapshot.FilterText(true, "content-fabric")
	require.Equal(t, 157, removed)
	require.Equal(t, 430, len(snapshot.Goroutines))

	removed = snapshot.FilterText(false, "qparts.")
	require.Equal(t, 423, removed)
	require.Equal(t, 7, len(snapshot.Goroutines))

	text, err := snapshot.AsText()
	require.NoError(t, err)
	fmt.Println(text)
}
