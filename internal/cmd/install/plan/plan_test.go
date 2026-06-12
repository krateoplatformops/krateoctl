package plan

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"gopkg.in/yaml.v3"
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

func TestPlanExecuteTableDiff(t *testing.T) {
	configPath := writeTestConfig(t, `componentsDefinition:
  demo:
    steps:
      - step-one
components:
  demo:
    enabled: false
steps:
  - id: step-one
    type: chart
    with:
      releaseName: demo
`)

	cmd := &planCmd{
		configFile: configPath,
		diffFormat: "table",
	}

	stderr := captureStderr(t, func() {
		status := cmd.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError))
		if status != subcommands.ExitSuccess {
			t.Fatalf("Execute() = %v, want %v", status, subcommands.ExitSuccess)
		}
	})

	for _, want := range []string{"STEP", "CHANGE", "SUMMARY", "modified", "step-one", "skip enabled"} {
		if !bytes.Contains([]byte(stderr), []byte(want)) {
			t.Fatalf("table diff output missing %q:\n%s", want, stderr)
		}
	}
}

func TestPlanExecuteOutput(t *testing.T) {
	configPath := writeTestConfig(t, `componentsDefinition:
  demo:
    steps:
      - step-one
  extra:
    steps:
      - step-two
components:
  extra:
    enabled: false
steps:
  - id: step-one
    type: chart
    with:
      releaseName: demo
  - id: step-two
    type: chart
    with:
      releaseName: extra
`)

	cmd := &planCmd{
		configFile: configPath,
		output:     true,
	}

	stdout := captureStdout(t, func() {
		status := cmd.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError))
		if status != subcommands.ExitSuccess {
			t.Fatalf("Execute() = %v, want %v", status, subcommands.ExitSuccess)
		}
	})

	// Output must be a single valid YAML document carrying both componentsDefinition and steps.
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("output is not valid YAML: %v\n%s", err, stdout)
	}
	if _, ok := doc["componentsDefinition"]; !ok {
		t.Fatalf("output missing componentsDefinition:\n%s", stdout)
	}
	rawSteps, ok := doc["steps"].([]any)
	if !ok {
		t.Fatalf("output missing steps list:\n%s", stdout)
	}

	// step-one is active (skip key omitted), step-two belongs to a disabled component (skip: true).
	stepByID := map[string]map[string]any{}
	for _, s := range rawSteps {
		m, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("step is not a map: %#v", s)
		}
		stepByID[m["id"].(string)] = m
	}
	if got, present := stepByID["step-one"]["skip"]; present {
		t.Fatalf("active step-one should omit skip, got skip=%v", got)
	}
	if stepByID["step-two"]["skip"] != true {
		t.Fatalf("disabled step-two should have skip: true, got %v", stepByID["step-two"]["skip"])
	}

	// The emitted document must itself be a valid, re-feedable config.
	cfg, err := config.NewConfig(doc)
	if err != nil {
		t.Fatalf("emitted document failed to parse as config: %v", err)
	}
	if err := config.NewValidator(cfg).Validate(); err != nil {
		t.Fatalf("emitted document failed validation (not re-feedable): %v", err)
	}

	// Re-running --output on the emitted document must be idempotent.
	roundTripPath := writeTestConfig(t, stdout)
	cmd2 := &planCmd{configFile: roundTripPath, output: true}
	stdout2 := captureStdout(t, func() {
		if status := cmd2.Execute(context.Background(), flag.NewFlagSet("plan", flag.ContinueOnError)); status != subcommands.ExitSuccess {
			t.Fatalf("second Execute() = %v, want %v", status, subcommands.ExitSuccess)
		}
	})
	if stdout2 != stdout {
		t.Fatalf("--output is not idempotent:\nfirst:\n%s\nsecond:\n%s", stdout, stdout2)
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

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	defer func() {
		os.Stderr = oldStderr
	}()

	fn()

	_ = w.Close()
	output := <-done
	_ = r.Close()

	return output
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	_ = w.Close()
	output := <-done
	_ = r.Close()

	return output
}
