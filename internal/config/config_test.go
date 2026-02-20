package config

import (
	"encoding/json"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
)

func TestGetActiveStepsAppliesStepWithOverrides(t *testing.T) {
	cfg := mustNewConfig(t, map[string]interface{}{
		"components": map[string]interface{}{
			"eventsse": map[string]interface{}{
				"steps": []interface{}{"install-eventsse"},
				"stepConfig": map[string]interface{}{
					"install-eventsse": map[string]interface{}{
						"with": map[string]interface{}{
							"releaseName": "eventsse-profile",
							"repo":        "custom-repo",
							"version":     "9.9.9",
						},
						"helmValues": map[string]interface{}{
							"env": map[string]interface{}{
								"DEBUG": "true",
							},
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]interface{}{
				"id":   "install-eventsse",
				"type": "chart",
				"with": map[string]interface{}{
					"releaseName": "eventsse",
					"repo":        "base-repo",
					"version":     "1.0.0",
					"values": map[string]interface{}{
						"env": map[string]interface{}{
							"DEBUG": "false",
						},
					},
				},
			},
		},
	})

	steps, err := cfg.GetActiveSteps()
	if err != nil {
		t.Fatalf("GetActiveSteps() error = %v", err)
	}

	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}

	with := unmarshalWith(t, steps[0])

	if got := with["releaseName"]; got != "eventsse-profile" {
		t.Fatalf("expected releaseName override, got %v", got)
	}
	if got := with["repo"]; got != "custom-repo" {
		t.Fatalf("expected repo override, got %v", got)
	}
	if got := with["version"]; got != "9.9.9" {
		t.Fatalf("expected version override, got %v", got)
	}

	values := with["values"].(map[string]interface{})
	env := values["env"].(map[string]interface{})
	if got := env["DEBUG"]; got != "true" {
		t.Fatalf("expected env override, got %v", got)
	}
}

func TestGetActiveStepsLegacyHelmDefaults(t *testing.T) {
	cfg := mustNewConfig(t, map[string]interface{}{
		"components": map[string]interface{}{
			"frontend": map[string]interface{}{
				"steps": []interface{}{"install-frontend"},
				"helmDefaults": map[string]interface{}{
					"service": map[string]interface{}{
						"type": "ClusterIP",
					},
				},
				"stepConfig": map[string]interface{}{
					"install-frontend": map[string]interface{}{
						"helmValues": map[string]interface{}{
							"env": map[string]interface{}{
								"FOO": "bar",
							},
						},
						"with": map[string]interface{}{
							"releaseName": "frontend-profile",
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]interface{}{
				"id":   "install-frontend",
				"type": "chart",
				"with": map[string]interface{}{
					"releaseName": "frontend",
					"values": map[string]interface{}{
						"service": map[string]interface{}{
							"type": "NodePort",
						},
						"env": map[string]interface{}{
							"FOO": "base",
						},
					},
				},
			},
		},
	})

	steps, err := cfg.GetActiveSteps()
	if err != nil {
		t.Fatalf("GetActiveSteps() error = %v", err)
	}

	with := unmarshalWith(t, steps[0])
	if got := with["releaseName"]; got != "frontend-profile" {
		t.Fatalf("expected releaseName override, got %v", got)
	}

	values := with["values"].(map[string]interface{})
	service := values["service"].(map[string]interface{})
	if got := service["type"]; got != "ClusterIP" {
		t.Fatalf("expected service.type override, got %v", got)
	}
	env := values["env"].(map[string]interface{})
	if got := env["FOO"]; got != "bar" {
		t.Fatalf("expected env override, got %v", got)
	}
}

func TestGetActiveStepsComponentWithOverrides(t *testing.T) {
	cfg := mustNewConfig(t, map[string]interface{}{
		"components": map[string]interface{}{
			"eventrouter": map[string]interface{}{
				"steps": []interface{}{"install-eventrouter"},
				"helmDefaults": map[string]interface{}{
					"with": map[string]interface{}{
						"namespace": "custom-ns",
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]interface{}{
				"id":   "install-eventrouter",
				"type": "chart",
				"with": map[string]interface{}{
					"namespace": "base-ns",
					"values":    map[string]interface{}{},
				},
			},
		},
	})

	steps, err := cfg.GetActiveSteps()
	if err != nil {
		t.Fatalf("GetActiveSteps() error = %v", err)
	}

	with := unmarshalWith(t, steps[0])
	if got := with["namespace"]; got != "custom-ns" {
		t.Fatalf("expected namespace override, got %v", got)
	}
}

func unmarshalWith(t *testing.T, step *types.Step) map[string]interface{} {
	var with map[string]interface{}
	if err := json.Unmarshal(step.With.Raw, &with); err != nil {
		t.Fatalf("failed to unmarshal chart spec: %v", err)
	}
	return with
}

func mustNewConfig(t *testing.T, data map[string]interface{}) *Config {
	t.Helper()
	cfg, err := NewConfig(data)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	return cfg
}
