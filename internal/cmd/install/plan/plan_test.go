package plan

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/subcommands"
)

func TestPlanExecute(t *testing.T) {
	tests := []struct {
		name       string
		configData string
		missing    bool
		wantStatus subcommands.ExitStatus
	}{
		{
			name:       "returns success with steps",
			configData: "componentsDefinition:\n  demo:\n    steps:\n      - step-one\nsteps:\n  - id: step-one\n    type: chart\n    with:\n      releaseName: demo\n",
			wantStatus: subcommands.ExitSuccess,
		},
		{
			name:       "returns success when no steps are defined",
			configData: "modules: {}\n",
			wantStatus: subcommands.ExitSuccess,
		},
		{
			name:       "returns failure when config is missing",
			missing:    true,
			wantStatus: subcommands.ExitFailure,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "krateo.yaml")
			if !tc.missing {
				configPath = writeTestConfig(t, tc.configData)
			}

			cmd := &planCmd{configFile: configPath}
			status := cmd.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError))

			if status != tc.wantStatus {
				t.Fatalf("Execute() = %v, want %v", status, tc.wantStatus)
			}
		})
	}
}

func writeTestConfig(t *testing.T, data string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "krateo.yaml")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	return path
}
