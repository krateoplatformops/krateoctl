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
    depends: []
    chart:
      repository: "https://charts.example.com"
      name: "frontend"
      namespace: "krateo"
  finops:
    enabled: true
    depends: [frontend]
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

	// Test validator
	validator := NewValidator(cfg)
	if err := validator.Validate(); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Test merger
	merger := NewMerger(cfg)
	if err := merger.disableModule("finops"); err != nil {
		t.Fatalf("failed to disable module: %v", err)
	}

	finops, err := cfg.GetModule("finops")
	if err != nil {
		t.Fatalf("failed to get finops module: %v", err)
	}

	if enabled, ok := finops["enabled"].(bool); !ok || enabled {
		t.Fatal("finops should be disabled")
	}
}
