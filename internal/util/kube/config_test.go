package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
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

// setHome points HOME, and the default kubeconfig path client-go resolves at
// init, at home.
func setHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	prev := clientcmd.RecommendedHomeFile
	clientcmd.RecommendedHomeFile = filepath.Join(home, ".kube", "config")
	t.Cleanup(func() { clientcmd.RecommendedHomeFile = prev })
}

func TestRestConfig(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string // path under HOME -> server
		kubeconfig []string          // paths under HOME, as absolute KUBECONFIG entries
		wantHost   string
		wantErr    bool
	}{
		{
			name:     "default kubeconfig",
			files:    map[string]string{filepath.Join(".kube", "config"): "https://127.0.0.1:6443"},
			wantHost: "https://127.0.0.1:6443",
		},
		{
			name: "KUBECONFIG absolute path wins over default",
			files: map[string]string{
				filepath.Join(".kube", "config"):           "https://127.0.0.1:6443",
				filepath.Join("ocp", "auth", "kubeconfig"): "https://custom-server:6443",
			},
			kubeconfig: []string{filepath.Join("ocp", "auth", "kubeconfig")},
			wantHost:   "https://custom-server:6443",
		},
		{
			name:       "KUBECONFIG list skips missing files",
			files:      map[string]string{filepath.Join("ocp", "kubeconfig"): "https://custom-server:6443"},
			kubeconfig: []string{"missing", filepath.Join("ocp", "kubeconfig")},
			wantHost:   "https://custom-server:6443",
		},
		{
			name:    "missing kubeconfig",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			setHome(t, home)
			for path, server := range tt.files {
				writeKubeconfig(t, filepath.Join(home, path), server)
			}

			paths := make([]string, 0, len(tt.kubeconfig))
			for _, path := range tt.kubeconfig {
				paths = append(paths, filepath.Join(home, path))
			}
			t.Setenv("KUBECONFIG", strings.Join(paths, string(os.PathListSeparator)))

			if tt.wantErr {
				if _, err := RestConfig(); err == nil {
					t.Fatal("RestConfig() expected error, got nil")
				}
				return
			}

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
	setHome(t, home)
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
