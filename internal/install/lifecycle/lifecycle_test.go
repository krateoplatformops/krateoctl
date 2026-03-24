package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalManifestsTypeVariants(t *testing.T) {
	tmpDir := t.TempDir()

	writeLifecycleManifest(t, filepath.Join(tmpDir, "pre-upgrade.kind.yaml"), "kind-job")
	writeLifecycleManifest(t, filepath.Join(tmpDir, "pre-upgrade.nodeport.yaml"), "nodeport-job")
	writeLifecycleManifest(t, filepath.Join(tmpDir, "pre-upgrade.yaml"), "generic-job")

	tests := []struct {
		name        string
		installType string
		wantName    string
	}{
		{
			name:        "kind yaml selects kind manifest",
			installType: "kind.yaml",
			wantName:    "kind-job",
		},
		{
			name:        "nodeport yaml selects nodeport manifest",
			installType: "nodeport.yaml",
			wantName:    "nodeport-job",
		},
		{
			name:        "nodeport alias still prefers kind manifest",
			installType: "nodeport",
			wantName:    "kind-job",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifests, err := loadLocalManifests(tmpDir, "pre-upgrade", tc.installType)
			if err != nil {
				t.Fatalf("loadLocalManifests() unexpected error: %v", err)
			}
			if len(manifests) != 1 {
				t.Fatalf("loadLocalManifests() len = %d, want 1", len(manifests))
			}
			if got := manifests[0].GetName(); got != tc.wantName {
				t.Fatalf("loadLocalManifests() name = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func writeLifecycleManifest(t *testing.T, path, name string) {
	t.Helper()

	data := "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: " + name + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
