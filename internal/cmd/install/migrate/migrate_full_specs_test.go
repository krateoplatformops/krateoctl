package migrate

import (
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestMigrateFullSpecsCommand(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*migrateFullSpecsCmd)
		wantErr bool
	}{
		{
			name: "defaults are applied",
			setup: func(cmd *migrateFullSpecsCmd) {
				cmd.ensureDeps()
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &migrateFullSpecsCmd{}
			if tt.setup != nil {
				tt.setup(cmd)
			}

			if cmd.namespace != "krateo-system" {
				t.Errorf("namespace = %q, want %q", cmd.namespace, "krateo-system")
			}

			if cmd.outputPath != "krateo.yaml" {
				t.Errorf("outputPath = %q, want %q", cmd.outputPath, "krateo.yaml")
			}

			if cmd.installerNamespace == "" {
				t.Errorf("installerNamespace should be set after ensureDeps()")
			}
		})
	}
}

func TestMigrateFullSpecsCmd_Name(t *testing.T) {
	cmd := &migrateFullSpecsCmd{}
	if got := cmd.Name(); got != "migrate-full" {
		t.Errorf("Name() = %q, want %q", got, "migrate-full")
	}
}

func TestMigrateFullSpecsCmd_Synopsis(t *testing.T) {
	cmd := &migrateFullSpecsCmd{}
	syn := cmd.Synopsis()
	if syn == "" {
		t.Errorf("Synopsis() returned empty string")
	}
}

func TestMigrateFullSpecsCmd_Usage(t *testing.T) {
	cmd := &migrateFullSpecsCmd{}
	usage := cmd.Usage()
	if usage == "" {
		t.Errorf("Usage() returned empty string")
	}

	// Check that important flags are documented
	if !containsSubstring(usage, "--namespace") {
		t.Errorf("Usage missing --namespace flag")
	}
	if !containsSubstring(usage, "--output") {
		t.Errorf("Usage missing --output flag")
	}
	if !containsSubstring(usage, "--installer-namespace") {
		t.Errorf("Usage missing --installer-namespace flag")
	}
}

func TestMigrateFullSpecsCmd_EnsureDeps(t *testing.T) {
	cmd := &migrateFullSpecsCmd{}
	cmd.ensureDeps()

	if cmd.restConfigFn == nil {
		t.Errorf("restConfigFn not set")
	}
	if cmd.dynamicFactory == nil {
		t.Errorf("dynamicFactory not set")
	}
	if cmd.writeFile == nil {
		t.Errorf("writeFile not set")
	}
	if cmd.kubeClientFactory == nil {
		t.Errorf("kubeClientFactory not set")
	}
	if cmd.stateFactory == nil {
		t.Errorf("stateFactory not set")
	}
	if cmd.ensureCRDFn == nil {
		t.Errorf("ensureCRDFn not set")
	}
	if cmd.getterFactory == nil {
		t.Errorf("getterFactory not set")
	}
	if cmd.applierFactory == nil {
		t.Errorf("applierFactory not set")
	}
	if cmd.deletorFactory == nil {
		t.Errorf("deletorFactory not set")
	}
	if cmd.workflowFactory == nil {
		t.Errorf("workflowFactory not set")
	}
	if cmd.errEvaluator == nil {
		t.Errorf("errEvaluator not set")
	}
}

func TestMigrateFullSpecsCmd_NamespaceDefault(t *testing.T) {
	cmd := &migrateFullSpecsCmd{}
	cmd.ensureDeps()

	if cmd.namespace != "krateo-system" {
		t.Errorf("namespace = %q, want %q", cmd.namespace, "krateo-system")
	}

	if cmd.installerNamespace != cmd.namespace {
		t.Errorf("installerNamespace should default to namespace")
	}
}

func TestMigrateFullSpecsCmd_CustomInstallerNamespace(t *testing.T) {
	cmd := &migrateFullSpecsCmd{
		namespace:          "custom-ns",
		installerNamespace: "other-ns",
	}
	cmd.ensureDeps()

	if cmd.installerNamespace != "other-ns" {
		t.Errorf("installerNamespace = %q, want %q", cmd.installerNamespace, "other-ns")
	}
}

func TestMigrateFullSpecsCmd_KubeClientFactory(t *testing.T) {
	called := false
	cmd := &migrateFullSpecsCmd{
		kubeClientFactory: func(cfg *rest.Config) (kubernetes.Interface, error) {
			called = true
			return nil, nil
		},
	}

	cmd.kubeClientFactory(nil)
	if !called {
		t.Errorf("kubeClientFactory not called")
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
