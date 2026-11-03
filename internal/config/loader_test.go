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

func TestLoaderLoadConfigWithTypeVariants(t *testing.T) {
	tmpDir := t.TempDir()
	basePath := filepath.Join(tmpDir, "krateo.yaml")
	kindPath := filepath.Join(tmpDir, "krateo.kind.yaml")
	nodeportPath := filepath.Join(tmpDir, "krateo.nodeport.yaml")

	writeTestFile(t, basePath, "modules:\n  base:\n    enabled: true\n")
	writeTestFile(t, kindPath, "modules:\n  kind:\n    enabled: true\n")
	writeTestFile(t, nodeportPath, "modules:\n  nodeport:\n    enabled: true\n")

	tests := []struct {
		name        string
		installType string
		wantModule  string
	}{
		{
			name:        "kind yaml selects kind file",
			installType: "kind.yaml",
			wantModule:  "kind",
		},
		{
			name:        "nodeport yaml selects nodeport file",
			installType: "nodeport.yaml",
			wantModule:  "nodeport",
		},
		{
			name:        "nodeport alias still prefers kind file",
			installType: "nodeport",
			wantModule:  "kind",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loader := NewLoader(LoadOptions{
				ConfigPath:       basePath,
				InstallationType: tc.installType,
			})

			data, err := loader.Load()
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}

			modules, ok := data["modules"].(map[string]any)
			if !ok {
				t.Fatalf("Load() modules type = %T, want map[string]any", data["modules"])
			}
			if _, ok := modules[tc.wantModule]; !ok {
				t.Fatalf("Load() modules = %v, want key %q", modules, tc.wantModule)
			}
		})
	}
}

func TestLoaderReplacesNamespaceTemplateBeforeYAMLParse(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "krateo.yaml")

	writeTestFile(t, configPath, `
componentsDefinition:
  backend:
    steps:
      - extract-authn
steps:
  - id: extract-authn
    type: var
    with:
      name: AUTHN_IP
      valueFrom:
        apiVersion: v1
        kind: Service
        metadata:
          name: authn
          namespace: {{ .Namespace }}
        selector: .status.loadBalancer.ingress[0].ip
  - id: install-deviser
    type: chart
    with:
      releaseName: deviser
      values:
        config:
          DB_HOST: pg-cluster-rw.{{ .Namespace }}.svc.cluster.local
`)

	loader := NewLoader(LoadOptions{
		ConfigPath: configPath,
		Namespace:  "demo-system",
	})

	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	steps := data["steps"].([]any)
	first := steps[0].(map[string]any)
	valueFrom := first["with"].(map[string]any)["valueFrom"].(map[string]any)
	metadata := valueFrom["metadata"].(map[string]any)
	if got := metadata["namespace"]; got != "demo-system" {
		t.Fatalf("metadata.namespace = %v, want demo-system", got)
	}

	second := steps[1].(map[string]any)
	values := second["with"].(map[string]any)["values"].(map[string]any)
	config := values["config"].(map[string]any)
	if got := config["DB_HOST"]; got != "pg-cluster-rw.demo-system.svc.cluster.local" {
		t.Fatalf("DB_HOST = %v, want pg-cluster-rw.demo-system.svc.cluster.local", got)
	}
}

func writeTestFile(t *testing.T, path string, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
