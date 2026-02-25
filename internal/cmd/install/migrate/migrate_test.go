package migrate

import "testing"

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
