package rtp

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/pacer"
)

// TestDisruptorPacerConfig_EngineConfigMapsEveryKnob guards against a knob being added to DisruptorPacerConfig (or to
// DisruptorEngineConfig) without a matching line in engineConfig. Such a field is silently unreachable: it can be set
// in deployment config and marshalled back out, but never reaches the engine. MaxBlock shipped that way initially.
func TestDisruptorPacerConfig_EngineConfigMapsEveryKnob(t *testing.T) {
	conf := (&DisruptorPacerConfig{}).InitDefaults()
	src := reflect.ValueOf(conf).Elem()

	// Assign a distinct non-zero value to every field DisruptorEngineConfig also declares under the same name and
	// type. Interface fields (the loggers) are skipped - they are supplied by the caller at construction.
	var mapped []string
	engineType := reflect.TypeOf(pacer.DisruptorEngineConfig{})
	for i := range engineType.NumField() {
		field := engineType.Field(i)
		target := src.FieldByName(field.Name)
		if !target.IsValid() || target.Type() != field.Type {
			continue
		}
		switch field.Type.Kind() {
		case reflect.String:
			target.SetString(field.Name)
		case reflect.Int, reflect.Int64:
			target.SetInt(int64(i + 1))
		default:
			continue
		}
		mapped = append(mapped, field.Name)
	}
	require.Contains(t, mapped, "MaxBlock", "MaxBlock must be settable on the pacer config")

	got := reflect.ValueOf(conf.engineConfig())
	for _, name := range mapped {
		require.Equal(t, src.FieldByName(name).Interface(), got.FieldByName(name).Interface(),
			"engineConfig() does not copy %s", name)
	}
}
