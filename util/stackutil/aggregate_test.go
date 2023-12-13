package stackutil_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/stackutil"
)

func TestAggregate(t *testing.T) {
	stackb, err := os.ReadFile("testdata/stacktrace1.txt")
	require.NoError(t, err)
	stack := string(stackb)

	c, err := stackutil.Aggregate(stack, stackutil.Normal)
	require.NoError(t, err)

	c.SortByCount(false)

	buckets := c.Buckets()
	require.Equal(t, 419, len(buckets[0].IDs))
	require.Equal(t, 64, len(buckets[1].IDs))
	require.Equal(t, 25, len(buckets[2].IDs))

	text, err := c.AsText()
	require.NoError(t, err)
	fmt.Println(text)
}

func TestAggregateHTML(t *testing.T) {
	stackb, err := os.ReadFile("testdata/stacktrace1.txt")
	require.NoError(t, err)
	stack := string(stackb)

	c, err := stackutil.Aggregate(stack, stackutil.Normal)
	require.NoError(t, err)

	c.SortByCount(false)

	buckets := c.Buckets()
	require.Equal(t, 419, len(buckets[0].IDs))
	require.Equal(t, 64, len(buckets[1].IDs))
	require.Equal(t, 25, len(buckets[2].IDs))

	text, err := c.AsHTML()
	require.NoError(t, err)
	require.NotEmpty(t, text)

	//err = ioutil.WriteFile("test.html", []byte(text), os.ModePerm)
	//require.NoError(t, err)
}

func TestAggregateNoStack(t *testing.T) {
	stack := "no stack"

	c, err := stackutil.Aggregate(stack, stackutil.Normal)
	require.Error(t, err)
	require.Nil(t, c)
}
