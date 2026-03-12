package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader(t *testing.T) {
	// Create a temporary krateo.yaml
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "krateo.yaml")

	configContent := `
modules:
  frontend:
    enabled: true
    chart:
      repository: "https://charts.example.com"
      name: "frontend"
      namespace: "krateo"
  finops:
    enabled: true
    chart:
      repository: "https://charts.example.com"
      name: "finops"
      namespace: "krateo"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Test load
	loader := NewLoader(LoadOptions{ConfigPath: configPath})
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test config
	cfg, err := NewConfig(data)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	modules, err := cfg.GetModules()
	if err != nil {
		t.Fatalf("failed to get modules: %v", err)
	}

	if len(modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(modules))
	}

	// Test validator still runs without module dependencies
	validator := NewValidator(cfg)
	if err := validator.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// No more expectation that finops is auto‑disabled
}

func TestLoaderProfileNotFound(t *testing.T) {
	// Create temporary files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "krateo.yaml")
	overridesPath := filepath.Join(tmpDir, "krateo-overrides.yaml")

	configContent := `
modules:
  frontend:
    enabled: true
    chart:
      repository: "https://charts.example.com"
      name: "frontend"
      namespace: "krateo"
`

	overridesContent := `
components:
  test-component:
    enabled: false
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := os.WriteFile(overridesPath, []byte(overridesContent), 0644); err != nil {
		t.Fatalf("failed to write overrides file: %v", err)
	}

	// Test load with non-existent profile
	loader := NewLoader(LoadOptions{
		ConfigPath:        configPath,
		UserOverridesPath: overridesPath,
		Profile:           "non-existent",
	})

	_, err := loader.Load()
	if err == nil {
		t.Fatalf("expected error for non-existent profile, got nil")
	}

	errMsg := err.Error()
	if !contains(errMsg, "non-existent") {
		t.Fatalf("expected error mentioning non-existent profile, got: %v", err)
	}
	if !contains(errMsg, "krateo-overrides.non-existent.yaml") {
		t.Fatalf("expected error suggesting file location, got: %v", err)
	}
	if !contains(errMsg, "install plan --help") {
		t.Fatalf("expected error suggesting help command, got: %v", err)
	}
}

func TestLoaderProfileFromFile(t *testing.T) {
	// Create temporary files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "krateo.yaml")
	overridesPath := filepath.Join(tmpDir, "krateo-overrides.yaml")
	profilePath := filepath.Join(tmpDir, "krateo-overrides.dev.yaml")

	configContent := `
modules:
  frontend:
    enabled: true
    chart:
      repository: "https://charts.example.com"
      name: "frontend"
      namespace: "krateo"
`

	overridesContent := `
components:
  test-component:
    enabled: false
`

	profileContent := `
components:
  test-component:
    enabled: true
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := os.WriteFile(overridesPath, []byte(overridesContent), 0644); err != nil {
		t.Fatalf("failed to write overrides file: %v", err)
	}

	if err := os.WriteFile(profilePath, []byte(profileContent), 0644); err != nil {
		t.Fatalf("failed to write profile file: %v", err)
	}

	// Test load with existing profile
	loader := NewLoader(LoadOptions{
		ConfigPath:        configPath,
		UserOverridesPath: overridesPath,
		Profile:           "dev",
	})

	_, err := loader.Load()
	if err != nil {
		t.Fatalf("failed to load with valid profile: %v", err)
	}
}
