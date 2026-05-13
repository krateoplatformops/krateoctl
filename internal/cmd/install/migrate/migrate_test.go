package migrate

import (
	"reflect"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"sigs.k8s.io/yaml"
)

func TestLoadComponentsDefinition(t *testing.T) {
	data := []byte(`componentsDefinition:
  foo:
    description: Example component
    steps:
      - step-a
      - step-b
`)

	components, err := loadComponentsDefinition(data)
	if err != nil {
		t.Fatalf("loadComponentsDefinition() error = %v", err)
	}

	foo, ok := components["foo"]
	if !ok {
		t.Fatalf("expected foo component, got %v", components)
	}

	if foo.Description != "Example component" {
		t.Fatalf("unexpected description %q", foo.Description)
	}

	if len(foo.Steps) != 2 || foo.Steps[0] != "step-a" || foo.Steps[1] != "step-b" {
		t.Fatalf("unexpected steps %v", foo.Steps)
	}
}

func TestLoadComponentsDefinitionErrors(t *testing.T) {
	if _, err := loadComponentsDefinition(nil); err == nil {
		t.Fatalf("expected error when data is empty")
	}

	data := []byte(`componentsDefinition: {}`)
	if _, err := loadComponentsDefinition(data); err == nil {
		t.Fatalf("expected error for missing component entries")
	}
}

func TestApplyDefaultComponentsPrunesMissingComponentsWithoutSynthesizingSteps(t *testing.T) {
	originalSteps := []config.StepDefinition{
		{
			ID:   "create-jwt-sign-key",
			Type: types.TypeObject,
			With: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]any{
					"name": "jwt-sign-key",
				},
			},
		},
		{
			ID:   "install-authn",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "authn",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-snowplow",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "snowplow",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-eventrouter",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "eventrouter",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-eventsse-etcd",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "etcd",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-eventsse",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "eventsse",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-sweeper",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "sweeper",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-frontend",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "frontend",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "extract-eventsse-internal-port",
			Type: types.TypeVar,
			With: map[string]any{
				"name": "EVENTSSE_INTERNAL_PORT",
				"valueFrom": map[string]any{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]any{
						"name": "eventsse-internal",
					},
					"selector": ".spec.ports[0].port",
				},
			},
		},
		{
			ID:   "install-composable-portal-starter",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "portal",
				"repository": "https://marketplace.krateo.io",
			},
		},
		{
			ID:   "install-core-provider",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "core-provider",
				"repository": "https://charts.krateo.io",
			},
		},
		{
			ID:   "install-oasgen-provider",
			Type: types.TypeChart,
			With: map[string]any{
				"name":       "oasgen-provider",
				"repository": "https://charts.krateo.io",
			},
		},
	}

	doc := &config.Document{Steps: originalSteps}
	if err := applyDefaultComponents(doc, "nodeport"); err != nil {
		t.Fatalf("applyDefaultComponents() error = %v", err)
	}

	if got := stepIDs(doc.Steps); !reflect.DeepEqual(got, stepIDs(originalSteps)) {
		t.Fatalf("steps changed during normalization: got %#v, want %#v", got, stepIDs(originalSteps))
	}

	backend, ok := doc.ComponentsDefinition["backend"]
	if !ok {
		t.Fatalf("expected backend component to remain, got %#v", doc.ComponentsDefinition)
	}

	wantBackendSteps := []string{"create-jwt-sign-key", "install-authn", "install-snowplow"}
	if !reflect.DeepEqual(backend.Steps, wantBackendSteps) {
		t.Fatalf("backend steps = %#v, want %#v", backend.Steps, wantBackendSteps)
	}

	if _, ok := doc.ComponentsDefinition["finops"]; ok {
		t.Fatalf("expected finops component to be pruned")
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}

	var data map[string]any
	if err := yaml.Unmarshal(raw, &data); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	cfg, err := config.NewConfig(data)
	if err != nil {
		t.Fatalf("config.NewConfig() error = %v", err)
	}

	if err := config.NewValidator(cfg).Validate(); err != nil {
		t.Fatalf("validator rejected pruned migration config: %v", err)
	}
}

func stepIDs(steps []config.StepDefinition) []string {
	ids := make([]string, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID)
	}
	return ids
}
