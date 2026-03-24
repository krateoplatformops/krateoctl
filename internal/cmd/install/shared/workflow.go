package shared

import (
	"context"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/client-go/rest"
)

type WorkflowRunner interface {
	Run(context.Context, *types.Workflow, func(*types.Step) bool, workflows.StepNotifier) []workflows.StepResult[any]
}

type GetterFactory func(*rest.Config) (*getter.Getter, error)
type ApplierFactory func(*rest.Config) (*applier.Applier, error)
type DeletorFactory func(*rest.Config) (*deletor.Deletor, error)
type WorkflowFactory func(workflows.Opts) (WorkflowRunner, error)
type StateStoreFactory func(*rest.Config, string) (state.Store, error)
type ErrEvaluator func([]workflows.StepResult[any]) error

type WorkflowDeps struct {
	GetterFactory   GetterFactory
	ApplierFactory  ApplierFactory
	DeletorFactory  DeletorFactory
	WorkflowFactory WorkflowFactory
	ErrEvaluator    ErrEvaluator
	StateFactory    StateStoreFactory
}

type ExecuteWorkflowOptions struct {
	Namespace        string
	StateName        string
	Logger           *ui.Logger
	Result           *LoadResult
	ProgressReporter workflows.StepNotifier
	SaveState        bool
}

type ExecuteWorkflowResult struct {
	Results    []workflows.StepResult[any]
	Snapshot   *state.Snapshot
	StateSaved bool
}

func ExecuteWorkflow(ctx context.Context, rc *rest.Config, opts ExecuteWorkflowOptions, deps WorkflowDeps) (*ExecuteWorkflowResult, error) {
	snapshot, err := state.BuildSnapshot(opts.Result.Config, opts.Result.Steps)
	if err != nil {
		return nil, fmt.Errorf("build installation snapshot: %w", err)
	}

	wf, err := newWorkflow(rc, opts.Namespace, opts.Logger, deps)
	if err != nil {
		return nil, err
	}

	results := wf.Run(ctx, &types.Workflow{Steps: opts.Result.Steps}, func(step *types.Step) bool {
		return step.Skip
	}, opts.ProgressReporter)

	if err := deps.ErrEvaluator(results); err != nil {
		return &ExecuteWorkflowResult{
			Results:  results,
			Snapshot: snapshot,
		}, err
	}

	stateSaved := false
	if opts.SaveState {
		store, err := deps.StateFactory(rc, opts.Namespace)
		if err != nil {
			return nil, fmt.Errorf("initialize installation state store: %w", err)
		}

		if err := store.Save(ctx, opts.StateName, snapshot); err != nil {
			return &ExecuteWorkflowResult{
				Results:  results,
				Snapshot: snapshot,
			}, fmt.Errorf("persist installation snapshot: %w", err)
		}
		stateSaved = true
	}

	return &ExecuteWorkflowResult{
		Results:    results,
		Snapshot:   snapshot,
		StateSaved: stateSaved,
	}, nil
}

func newWorkflow(rc *rest.Config, namespace string, logger *ui.Logger, deps WorkflowDeps) (WorkflowRunner, error) {
	g, err := deps.GetterFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("initialize getter: %w", err)
	}

	a, err := deps.ApplierFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("initialize applier: %w", err)
	}

	d, err := deps.DeletorFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("initialize deletor: %w", err)
	}

	wf, err := deps.WorkflowFactory(workflows.Opts{
		Getter:    g,
		Applier:   a,
		Deletor:   d,
		Logger:    logger.Debug,
		Cfg:       rc,
		Namespace: namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize workflow: %w", err)
	}

	return wf, nil
}

func LogWorkflowResults(logger *ui.Logger, steps []*types.Step, results []workflows.StepResult[any]) {
	for i, step := range steps {
		res := results[i]
		switch {
		case res.ID() == "":
			logger.Info("[PEND] %s (%s) not executed", step.ID, step.Type)
		case step.Skip:
			logger.Info("[SKIP] %s (%s)", step.ID, step.Type)
		case res.Err() != nil:
			logger.Error("%s (%s) failed: %v", step.ID, step.Type, res.Err())
		default:
			logger.Info("✓ %s (%s)", step.ID, step.Type)
		}
	}
}
