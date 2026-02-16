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
	cfg := NewConfig(data)
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
