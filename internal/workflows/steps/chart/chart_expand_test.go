package steps

import (
	"reflect"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/cache"
)

func TestChartHandlerExpandValues(t *testing.T) {
	env := cache.New[string, string]()
	env.Set("AUTHN_PORT", "30082")
	env.Set("SNOWPLOW_PORT", "30081")

	handler := &chartStepHandler{env: env}
	handler.subst = func(k string) string {
		if v, ok := handler.env.Get(k); ok {
			return v
		}
		return "$" + k
	}

	input := map[string]any{
		"config": map[string]any{
			"AUTHN": "http://127.0.0.1:${AUTHN_PORT}",
			"nested": []any{
				"start",
				"${SNOWPLOW_PORT}",
				map[string]any{
					"port": "${AUTHN_PORT}",
				},
			},
		},
		"unchanged": 42,
	}

	got := handler.expandValues(input).(map[string]any)

	cfg := got["config"].(map[string]any)
	if cfg["AUTHN"] != "http://127.0.0.1:30082" {
		t.Fatalf("expected AUTHN to be expanded, got %v", cfg["AUTHN"])
	}

	nested := cfg["nested"].([]any)
	if nested[1] != "30081" {
		t.Fatalf("expected slice entry to expand, got %v", nested[1])
	}

	nestedMap := nested[2].(map[string]any)
	if nestedMap["port"] != "30082" {
		t.Fatalf("expected nested map entry to expand, got %v", nestedMap["port"])
	}

	if !reflect.DeepEqual(got["unchanged"], 42) {
		t.Fatalf("non-string values should remain untouched")
	}
}
