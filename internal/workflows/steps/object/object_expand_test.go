package steps

import (
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/cache"
)

func TestObjectHandlerExpandValues(t *testing.T) {
	env := cache.New[string, string]()
	env.Set("CONFIG_VALUE", "expanded")

	handler := &objStepHandler{env: env, ns: "default"}
	handler.subst = func(k string) string {
		if v, ok := handler.env.Get(k); ok {
			return v
		}
		return "$" + k
	}

	ext := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "example",
		},
		"data": map[string]any{
			"KEY": "${CONFIG_VALUE}",
		},
	}

	uns, err := handler.toUnstructured("obj-expand", &ext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := uns.Object["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map in unstructured object")
	}

	if data["KEY"] != "expanded" {
		t.Fatalf("expected placeholder to be expanded, got %v", data["KEY"])
	}

	meta := uns.Object["metadata"].(map[string]any)
	if meta["namespace"] != "default" {
		t.Fatalf("expected namespace inheritance, got %v", meta["namespace"])
	}
}
