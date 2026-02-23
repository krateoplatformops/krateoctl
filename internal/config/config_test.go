package config

import (
	"testing"
)

func TestGetActiveStepsAppliesStepWithOverrides(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"components": map[string]any{
			"eventsse": map[string]any{
				"steps": []interface{}{"install-eventsse"},
				"stepConfig": map[string]any{
					"install-eventsse": map[string]any{
						"with": map[string]any{
							"releaseName": "eventsse-profile",
							"repo":        "custom-repo",
							"version":     "9.9.9",
						},
						"helmValues": map[string]any{
							"env": map[string]any{
								"DEBUG": "true",
							},
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-eventsse",
				"type": "chart",
				"with": map[string]any{
					"releaseName": "eventsse",
					"repo":        "base-repo",
					"version":     "1.0.0",
					"values": map[string]any{
						"env": map[string]any{
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

	with := *steps[0].With

	if got := with["releaseName"]; got != "eventsse-profile" {
		t.Fatalf("expected releaseName override, got %v", got)
	}
	if got := with["repo"]; got != "custom-repo" {
		t.Fatalf("expected repo override, got %v", got)
	}
	if got := with["version"]; got != "9.9.9" {
		t.Fatalf("expected version override, got %v", got)
	}

	values := with["values"].(map[string]any)
	env := values["env"].(map[string]any)
	if got := env["DEBUG"]; got != "true" {
		t.Fatalf("expected env override, got %v", got)
	}
}

func TestGetActiveStepsLegacyHelmDefaults(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"components": map[string]any{
			"frontend": map[string]any{
				"steps": []interface{}{"install-frontend"},
				"helmDefaults": map[string]any{
					"service": map[string]any{
						"type": "ClusterIP",
					},
				},
				"stepConfig": map[string]any{
					"install-frontend": map[string]any{
						"helmValues": map[string]any{
							"env": map[string]any{
								"FOO": "bar",
							},
						},
						"with": map[string]any{
							"releaseName": "frontend-profile",
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-frontend",
				"type": "chart",
				"with": map[string]any{
					"releaseName": "frontend",
					"values": map[string]any{
						"service": map[string]any{
							"type": "NodePort",
						},
						"env": map[string]any{
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

	with := *steps[0].With
	if got := with["releaseName"]; got != "frontend-profile" {
		t.Fatalf("expected releaseName override, got %v", got)
	}

	values := with["values"].(map[string]any)
	service := values["service"].(map[string]any)
	if got := service["type"]; got != "ClusterIP" {
		t.Fatalf("expected service.type override, got %v", got)
	}
	env := values["env"].(map[string]any)
	if got := env["FOO"]; got != "bar" {
		t.Fatalf("expected env override, got %v", got)
	}
}

func TestGetActiveStepsComponentWithOverrides(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"components": map[string]any{
			"eventrouter": map[string]any{
				"steps": []interface{}{"install-eventrouter"},
				"helmDefaults": map[string]any{
					"with": map[string]any{
						"namespace": "custom-ns",
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-eventrouter",
				"type": "chart",
				"with": map[string]any{
					"namespace": "base-ns",
					"values":    map[string]any{},
				},
			},
		},
	})

	steps, err := cfg.GetActiveSteps()
	if err != nil {
		t.Fatalf("GetActiveSteps() error = %v", err)
	}

	with := *steps[0].With
	if got := with["namespace"]; got != "custom-ns" {
		t.Fatalf("expected namespace override, got %v", got)
	}
}

func mustNewConfig(t *testing.T, data map[string]any) *Config {
	t.Helper()
	cfg, err := NewConfig(data)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	return cfg
}
