package config

import (
	"fmt"
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

// Validator tests

func TestValidateComponentStepsReferencesNonExistentStep(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"backend": map[string]any{
				"steps": []interface{}{"non-existent-step"},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error for non-existent step, got nil")
	}
	if !contains(err.Error(), "non-existent-step") {
		t.Fatalf("expected error mentioning non-existent-step, got: %v", err)
	}
}

func TestValidateComponentStepsMultipleInvalid(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"backend": map[string]any{
				"steps": []interface{}{"missing-step-1", "missing-step-2"},
			},
			"frontend": map[string]any{
				"steps": []interface{}{"missing-step-3"},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error for non-existent steps, got nil")
	}

	errMsg := err.Error()
	if !contains(errMsg, "missing-step-1") || !contains(errMsg, "missing-step-2") || !contains(errMsg, "missing-step-3") {
		t.Fatalf("expected error mentioning all missing steps, got: %v", err)
	}
}

func TestValidateComponentsNotInDefinition(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"backend": map[string]any{
				"steps": []interface{}{"install-authn"},
			},
		},
		"components": map[string]any{
			"unknown-component": map[string]any{
				"enabled": false,
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error for component not in definition, got nil")
	}
	if !contains(err.Error(), "unknown-component") {
		t.Fatalf("expected error mentioning unknown-component, got: %v", err)
	}
	if !contains(err.Error(), "not defined in 'componentsDefinition'") {
		t.Fatalf("expected error about component not in definition, got: %v", err)
	}
}

func TestValidateOrphanedSteps(t *testing.T) {
	var warnings []string
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"backend": map[string]any{
				"steps": []interface{}{"install-authn"},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
			map[string]any{
				"id":   "orphaned-step",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg).WithLogger(func(msg string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(msg, args...))
	})

	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error for orphaned step, got nil")
	}

	if !contains(err.Error(), "orphaned-step") {
		t.Fatalf("expected error about orphaned-step, got: %v", err)
	}
}

func TestValidateValidComponentSteps(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"backend": map[string]any{
				"steps": []interface{}{"install-authn"},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateNoComponentsDefinition(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"steps": []interface{}{
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error when no components defined, got nil")
	}

	if !contains(err.Error(), "no components defined") {
		t.Fatalf("expected error about no components, got: %v", err)
	}
}

func TestValidateStepConfigReferencesNonExistentStep(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"composable-operations": map[string]any{
				"steps": []interface{}{"install-core-provider"},
				"stepConfig": map[string]any{
					"ss-install-core-provider": map[string]any{
						"with": map[string]any{
							"env": "test",
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-core-provider",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error for non-existent step in stepConfig, got nil")
	}
	if !contains(err.Error(), "ss-install-core-provider") {
		t.Fatalf("expected error mentioning ss-install-core-provider, got: %v", err)
	}
	if !contains(err.Error(), "does not exist in the steps list") {
		t.Fatalf("expected error stating step doesn't exist, got: %v", err)
	}
}

func TestValidateStepConfigReferencesStepNotInComponent(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"composable-operations": map[string]any{
				"steps": []interface{}{"install-core-provider"},
				"stepConfig": map[string]any{
					"install-authn": map[string]any{
						"with": map[string]any{
							"env": "test",
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-core-provider",
				"type": "chart",
			},
			map[string]any{
				"id":   "install-authn",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err == nil {
		t.Fatalf("expected error for stepConfig referencing step not in component, got nil")
	}
	if !contains(err.Error(), "install-authn") {
		t.Fatalf("expected error mentioning install-authn, got: %v", err)
	}
	if !contains(err.Error(), "is not a step of this component") {
		t.Fatalf("expected error stating step is not in component, got: %v", err)
	}
}

func TestValidateStepConfigValid(t *testing.T) {
	cfg := mustNewConfig(t, map[string]any{
		"componentsDefinition": map[string]any{
			"composable-operations": map[string]any{
				"steps": []interface{}{"install-core-provider"},
				"stepConfig": map[string]any{
					"install-core-provider": map[string]any{
						"with": map[string]any{
							"env": "test",
						},
					},
				},
			},
		},
		"steps": []interface{}{
			map[string]any{
				"id":   "install-core-provider",
				"type": "chart",
			},
		},
	})

	validator := NewValidator(cfg)
	err := validator.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
