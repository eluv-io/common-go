package jsonutil_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/jsonutil"
)

type model struct {
	tracker jsonutil.FieldTracker
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Size    int    `json:"size"`
}

func (m *model) UnmarshalJSON(bts []byte) error {
	type alias model
	var a alias
	err := json.Unmarshal(bts, &a)
	if err != nil {
		return err
	}
	*m = model(a)

	_ = json.Unmarshal(bts, &m.tracker)
	return nil
}

type modelNoTags struct {
	tracker jsonutil.FieldTracker
	Name    string
	Enabled bool
	Size    int
}

func (m *modelNoTags) UnmarshalJSON(bts []byte) error {
	type alias modelNoTags
	var a alias
	err := json.Unmarshal(bts, &a)
	if err != nil {
		return err
	}
	*m = modelNoTags(a)

	_ = json.Unmarshal(bts, &m.tracker)
	return nil
}

func TestDefaults(t *testing.T) {
	jsn := `{"name":"test","enabled":true}`
	var m model
	err := json.Unmarshal([]byte(jsn), &m)
	require.NoError(t, err)
	require.Equal(t, "test", m.Name)
	require.Equal(t, true, m.Enabled)

	err = jsonutil.SetDefaults(model{Name: "default", Size: 50, Enabled: false}, &m, m.tracker)
	require.NoError(t, err)

	require.Equal(t, "test", m.Name)
	require.Equal(t, true, m.Enabled)
	require.Equal(t, 50, m.Size)
}

func TestDefaultsNoTags(t *testing.T) {
	jsn := `{"Name":"test","Enabled":true}`
	var m modelNoTags
	err := json.Unmarshal([]byte(jsn), &m)
	require.NoError(t, err)
	require.Equal(t, "test", m.Name)
	require.Equal(t, true, m.Enabled)

	err = jsonutil.SetDefaults(modelNoTags{Name: "default", Size: 50, Enabled: false}, &m, m.tracker)
	require.NoError(t, err)

	require.Equal(t, "test", m.Name)
	require.Equal(t, true, m.Enabled)
	require.Equal(t, 50, m.Size)
}
