package plan

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/subcommands"
)

func TestPlanExecuteSuccess(t *testing.T) {
	cfg := writeTestConfig(t, "steps:\n  - id: step-one\n    type: chart\n    with:\n      releaseName: demo\n")

	cmd := &planCmd{configFile: cfg}
	status := cmd.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError))

	if status != subcommands.ExitSuccess {
		t.Fatalf("expected success, got %v", status)
	}
}

func TestPlanExecuteNoSteps(t *testing.T) {
	cfg := writeTestConfig(t, "modules: {}\n")

	cmd := &planCmd{configFile: cfg}
	status := cmd.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError))

	if status != subcommands.ExitSuccess {
		t.Fatalf("expected success when no steps are defined, got %v", status)
	}
}

func TestPlanExecuteMissingConfig(t *testing.T) {
	cmd := &planCmd{configFile: filepath.Join(t.TempDir(), "missing.yaml")}
	status := cmd.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError))

	if status != subcommands.ExitFailure {
		t.Fatalf("expected failure for missing config file, got %v", status)
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
