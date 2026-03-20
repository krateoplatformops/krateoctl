package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testLoaderConfig = `
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
	testLoaderOverrides = `
components:
  test-component:
    enabled: false
`
	testLoaderProfile = `
components:
  test-component:
    enabled: true
`
)

func TestLoader(t *testing.T) {
	tests := []struct {
		name              string
		profile           string
		writeBaseOverride bool
		writeProfile      bool
		wantModules       int
		wantErrContains   []string
	}{
		{
			name:        "loads base config",
			wantModules: 2,
		},
		{
			name:              "fails when profile file is missing",
			profile:           "non-existent",
			writeBaseOverride: true,
			wantErrContains: []string{
				"non-existent",
				"krateo-overrides.non-existent.yaml",
				"install plan --help",
			},
		},
		{
			name:              "loads profile override from file",
			profile:           "dev",
			writeBaseOverride: true,
			writeProfile:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "krateo.yaml")
			overridesPath := filepath.Join(tmpDir, "krateo-overrides.yaml")
			profilePath := filepath.Join(tmpDir, "krateo-overrides.dev.yaml")

			writeTestFile(t, configPath, testLoaderConfig)
			if tc.writeBaseOverride {
				writeTestFile(t, overridesPath, testLoaderOverrides)
			}
			if tc.writeProfile {
				writeTestFile(t, profilePath, testLoaderProfile)
			}

			loader := NewLoader(LoadOptions{
				ConfigPath:        configPath,
				UserOverridesPath: overridesPath,
				Profile:           tc.profile,
			})

			data, err := loader.Load()
			if len(tc.wantErrContains) > 0 {
				if err == nil {
					t.Fatal("Load() error = nil, want profile resolution error")
				}
				for _, want := range tc.wantErrContains {
					if !contains(err.Error(), want) {
						t.Fatalf("Load() error %q does not contain %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}

			cfg, err := NewConfig(data)
			if err != nil {
				t.Fatalf("NewConfig() error: %v", err)
			}

			modules, err := cfg.GetModules()
			if err != nil {
				t.Fatalf("GetModules() error: %v", err)
			}
			if tc.wantModules > 0 && len(modules) != tc.wantModules {
				t.Fatalf("GetModules() returned %d modules, want %d", len(modules), tc.wantModules)
			}

			validator := NewValidator(cfg)
			if err := validator.Validate(); err != nil {
				t.Fatalf("Validate() error: %v", err)
			}
		})
	}
}

func writeTestFile(t *testing.T, path string, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
