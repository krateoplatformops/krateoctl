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

func TestApplyExecute(t *testing.T) {
	tests := []struct {
		name               string
		configData         string
		errEvaluator       func([]workflows.StepResult[any]) error
		wantStatus         subcommands.ExitStatus
		wantWorkflowCalled bool
		wantStateSaved     bool
		wantRestCalled     bool
	}{
		{
			name:               "returns success and saves state when workflow succeeds",
			configData:         "componentsDefinition:\n  demo:\n    steps:\n      - step-one\nsteps:\n  - id: step-one\n    type: chart\n    with:\n      releaseName: demo\n",
			wantStatus:         subcommands.ExitSuccess,
			wantWorkflowCalled: true,
			wantStateSaved:     true,
			wantRestCalled:     true,
		},
		{
			name:       "returns failure when workflow evaluation fails",
			configData: "componentsDefinition:\n  demo:\n    steps:\n      - failing\nsteps:\n  - id: failing\n    type: chart\n    with:\n      releaseName: demo\n",
			errEvaluator: func([]workflows.StepResult[any]) error {
				return errors.New("workflow failure")
			},
			wantStatus:         subcommands.ExitFailure,
			wantWorkflowCalled: true,
			wantStateSaved:     false,
			wantRestCalled:     true,
		},
		{
			name:           "skips cluster access when no steps are defined",
			configData:     "modules: {}\n",
			wantStatus:     subcommands.ExitSuccess,
			wantRestCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := writeApplyConfig(t, tc.configData)

			runner := &stubWorkflow{}
			store := &stubStateStore{}
			restCalled := false
			cmd := &applyCmd{
				configFile: cfg,
				namespace:  "test-ns",
				restConfigFn: func() (*rest.Config, error) {
					restCalled = true
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
				errEvaluator: tc.errEvaluator,
				stateName:    "test-install",
			}

			status := cmd.Execute(context.Background(), flag.NewFlagSet("apply", flag.ContinueOnError))
			if status != tc.wantStatus {
				t.Fatalf("Execute() = %v, want %v", status, tc.wantStatus)
			}
			if runner.called != tc.wantWorkflowCalled {
				t.Fatalf("workflow called = %v, want %v", runner.called, tc.wantWorkflowCalled)
			}
			if store.saved != tc.wantStateSaved {
				t.Fatalf("state saved = %v, want %v", store.saved, tc.wantStateSaved)
			}
			if restCalled != tc.wantRestCalled {
				t.Fatalf("restConfigFn called = %v, want %v", restCalled, tc.wantRestCalled)
			}
		})
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
