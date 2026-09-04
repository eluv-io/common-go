package mpegts

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/pacer"
)

// TestPacerConfig_EngineConfigMapsEveryKnob guards against a knob being added to a pacer config (or to
// DisruptorEngineConfig) without a matching line in engineConfig. Such a field is silently unreachable: it can be set
// in deployment config and marshalled back out, but never reaches the engine. MaxBlock shipped that way initially.
func TestPacerConfig_EngineConfigMapsEveryKnob(t *testing.T) {
	t.Run("ts", func(t *testing.T) {
		conf := (&TsDisruptorPacerConfig{}).InitDefaults()
		requireEngineConfigMapsEveryKnob(t, reflect.ValueOf(conf).Elem(), conf.engineConfig)
	})
	t.Run("ats", func(t *testing.T) {
		conf := (&AtsDisruptorPacerConfig{}).InitDefaults()
		requireEngineConfigMapsEveryKnob(t, reflect.ValueOf(conf).Elem(), conf.engineConfig)
	})
}

// requireEngineConfigMapsEveryKnob assigns a distinct non-zero value to every field of conf that
// DisruptorEngineConfig also declares under the same name and type, then checks engineConfig copied all of them.
// Interface fields (the loggers) are skipped - they are supplied by the caller at construction, not defaulted.
func requireEngineConfigMapsEveryKnob(
	t *testing.T,
	conf reflect.Value,
	engineConfig func() pacer.DisruptorEngineConfig,
) {
	var mapped []string
	engineType := reflect.TypeOf(pacer.DisruptorEngineConfig{})
	for i := range engineType.NumField() {
		field := engineType.Field(i)
		target := conf.FieldByName(field.Name)
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

	got := reflect.ValueOf(engineConfig())
	for _, name := range mapped {
		require.Equal(t, conf.FieldByName(name).Interface(), got.FieldByName(name).Interface(),
			"engineConfig() does not copy %s", name)
	}
}
