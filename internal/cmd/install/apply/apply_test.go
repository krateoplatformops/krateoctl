package apply

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/client-go/rest"
)

func TestApplyExecuteSuccess(t *testing.T) {
	cfg := writeApplyConfig(t, "steps:\n  - id: step-one\n    type: chart\n    with:\n      releaseName: demo\n")

	runner := &stubWorkflow{}
	store := &stubStateStore{}
	cmd := &applyCmd{
		configFile: cfg,
		namespace:  "test-ns",
		restConfigFn: func() (*rest.Config, error) {
			return &rest.Config{}, nil
		},
		getterFactory: func(*rest.Config) (*getter.Getter, error) {
			return &getter.Getter{}, nil
		},
		applierFactory: func(*rest.Config) (*applier.Applier, error) {
			return &applier.Applier{}, nil
		},
		deletorFactory: func(*rest.Config) (*deletor.Deletor, error) {
			return &deletor.Deletor{}, nil
		},
		workflowFactory: func(workflows.Opts) (workflowRunner, error) {
			return runner, nil
		},
		stateFactory: func(*rest.Config, string) (state.Store, error) { return store, nil },
		ensureCRDFn:  func(context.Context, *rest.Config) error { return nil },
		stateName:    "test-install",
	}

	status := cmd.Execute(context.Background(), flag.NewFlagSet("apply", flag.ContinueOnError))
	if status != subcommands.ExitSuccess {
		t.Fatalf("expected success, got %v", status)
	}

	if !runner.called {
		t.Fatal("expected workflow runner to be invoked")
	}

	if !store.saved {
		t.Fatal("expected installation snapshot to be saved")
	}
}

func TestApplyExecuteFailureFromWorkflow(t *testing.T) {
	cfg := writeApplyConfig(t, "steps:\n  - id: failing\n    type: chart\n    with:\n      releaseName: demo\n")

	cmdErr := errors.New("workflow failure")
	cmd := &applyCmd{
		configFile:      cfg,
		namespace:       "test-ns",
		restConfigFn:    func() (*rest.Config, error) { return &rest.Config{}, nil },
		getterFactory:   func(*rest.Config) (*getter.Getter, error) { return &getter.Getter{}, nil },
		applierFactory:  func(*rest.Config) (*applier.Applier, error) { return &applier.Applier{}, nil },
		deletorFactory:  func(*rest.Config) (*deletor.Deletor, error) { return &deletor.Deletor{}, nil },
		workflowFactory: func(workflows.Opts) (workflowRunner, error) { return &stubWorkflow{}, nil },
		stateFactory:    func(*rest.Config, string) (state.Store, error) { return &stubStateStore{}, nil },
		ensureCRDFn:     func(context.Context, *rest.Config) error { return nil },
		errEvaluator: func([]workflows.StepResult[any]) error {
			return cmdErr
		},
	}

	status := cmd.Execute(context.Background(), flag.NewFlagSet("apply", flag.ContinueOnError))
	if status != subcommands.ExitFailure {
		t.Fatalf("expected failure, got %v", status)
	}
}

func TestApplyExecuteSkipsClusterWhenNoSteps(t *testing.T) {
	cfg := writeApplyConfig(t, "modules: {}\n")

	called := false
	cmd := &applyCmd{
		configFile: cfg,
		restConfigFn: func() (*rest.Config, error) {
			called = true
			return &rest.Config{}, nil
		},
	}

	status := cmd.Execute(context.Background(), flag.NewFlagSet("apply", flag.ContinueOnError))
	if status != subcommands.ExitSuccess {
		t.Fatalf("expected success, got %v", status)
	}

	if called {
		t.Fatal("restConfigFn should not be invoked when there are no steps")
	}
}

type stubWorkflow struct {
	called bool
}

func (s *stubWorkflow) Run(_ context.Context, spec *types.Workflow, _ func(*types.Step) bool, _ workflows.StepNotifier) []workflows.StepResult[any] {
	s.called = true
	return make([]workflows.StepResult[any], len(spec.Steps))
}

type stubStateStore struct {
	saved bool
}

func (s *stubStateStore) Save(_ context.Context, _ string, snapshot *state.Snapshot) error {
	s.saved = snapshot != nil
	return nil
}

func (s *stubStateStore) Load(_ context.Context, _ string) (*state.Snapshot, error) {
	return nil, nil
}

func writeApplyConfig(t *testing.T, data string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "krateo.yaml")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	return path
}
