package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const kubeconfigTemplate = `apiVersion: v1
clusters:
- cluster:
    server: %s
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
kind: Config
preferences: {}
users:
- name: test-user
  user:
    token: test-token
`

func writeKubeconfig(t *testing.T, path, server string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir kubeconfig dir: %v", err)
	}

	if err := os.WriteFile(path, []byte(fmt.Sprintf(kubeconfigTemplate, server)), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
}

func TestRestConfig(t *testing.T) {
	tests := []struct {
		name       string
		relPath    string
		kubeconfig string
		server     string
		wantHost   string
		wantErr    bool
	}{
		{
			name:     "default kubeconfig",
			relPath:  filepath.Join(".kube", "config"),
			server:   "https://127.0.0.1:6443",
			wantHost: "https://127.0.0.1:6443",
		},
		{
			name:       "custom kubeconfig",
			relPath:    filepath.Join(".kube", "custom-config"),
			kubeconfig: filepath.Join(".kube", "custom-config"),
			server:     "https://custom-server:6443",
			wantHost:   "https://custom-server:6443",
		},
		{
			name:    "missing kubeconfig falls back",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				t.Setenv("HOME", "/nonexistent")
				t.Setenv("KUBECONFIG", "")
				if _, err := RestConfig(); err == nil {
					t.Fatal("RestConfig() expected error, got nil")
				}
				return
			}

			home := t.TempDir()
			writeKubeconfig(t, filepath.Join(home, tt.relPath), tt.server)
			t.Setenv("HOME", home)
			t.Setenv("KUBECONFIG", tt.kubeconfig)

			cfg, err := RestConfig()
			if err != nil {
				t.Fatalf("RestConfig() error = %v", err)
			}
			if cfg == nil {
				t.Fatal("RestConfig() returned nil config")
			}
			if cfg.Host != tt.wantHost {
				t.Fatalf("RestConfig() Host = %s, want %s", cfg.Host, tt.wantHost)
			}
		})
	}
}

func TestClientConfig(t *testing.T) {
	home := t.TempDir()
	writeKubeconfig(t, filepath.Join(home, ".kube", "config"), "https://127.0.0.1:6443")
	t.Setenv("HOME", home)
	t.Setenv("KUBECONFIG", "")

	cfg, err := ClientConfig()
	if err != nil {
		t.Fatalf("ClientConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("ClientConfig() returned nil")
	}

	rawCfg, err := cfg.RawConfig()
	if err != nil {
		t.Fatalf("RawConfig() error = %v", err)
	}
	if rawCfg.CurrentContext != "test-context" {
		t.Fatalf("ClientConfig() context = %s, want test-context", rawCfg.CurrentContext)
	}
}