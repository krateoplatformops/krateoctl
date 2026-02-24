package workflows

import (
	"context"
	"fmt"
	"slices"

	"github.com/krateoplatformops/krateoctl/internal/cache"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/workflows/steps"
	charthandler "github.com/krateoplatformops/krateoctl/internal/workflows/steps/chart"
	objecthandler "github.com/krateoplatformops/krateoctl/internal/workflows/steps/object"
	varhandler "github.com/krateoplatformops/krateoctl/internal/workflows/steps/var"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/client-go/rest"
)

type Opts struct {
	Getter    *getter.Getter
	Applier   *applier.Applier
	Deletor   *deletor.Deletor
	Logger    func(string, ...any)
	Cfg       *rest.Config
	Namespace string
}

func New(opts Opts) (*Workflow, error) {
	if opts.Getter == nil || opts.Applier == nil || opts.Deletor == nil {
		return nil, fmt.Errorf("dynamic getter, applier, or deletor cannot be nil")
	}

	if opts.Logger == nil {
		opts.Logger = func(string, ...any) {}
	}
	wf := &Workflow{
		logger: opts.Logger,
		ns:     opts.Namespace,
		env:    cache.New[string, string](),
	}

	wf.varHandler = varhandler.VarHandler(opts.Getter, wf.env, opts.Logger)
	wf.objectHandler = objecthandler.ObjectHandler(opts.Applier, opts.Deletor, wf.env, opts.Logger)
	wf.chartHandler = charthandler.ChartHandler(charthandler.ChartHandlerOptions{
		Env:    wf.env,
		Logger: opts.Logger,
		Dyn:    opts.Getter,
		Cfg:    opts.Cfg,
	})

	return wf, nil
}

type StepResult[T any] struct {
	id     string
	digest string
	err    error
	res    T
}

func (r *StepResult[T]) ID() string {
	return r.id
}

func (r *StepResult[T]) Digest() string {
	return r.digest
}

func (r *StepResult[T]) Err() error {
	return r.err
}

// Aggiungi questi metodi al StepResult

func (r *StepResult[T]) Result() T {
	return r.res
}

func Err[T any](results []StepResult[T]) error {
	for _, x := range results {
		if x.Err() != nil {
			return fmt.Errorf("%s: %w", x.ID(), x.Err())
		}
	}

	return nil
}

type Workflow struct {
	logger        func(string, ...any)
	ns            string
	env           *cache.Cache[string, string]
	varHandler    steps.Handler[*steps.VarResult]
	objectHandler steps.Handler[*steps.ObjectResult]
	chartHandler  steps.Handler[*steps.ChartResult]
	op            steps.Op
}

func (wf *Workflow) Op(op steps.Op) {
	wf.op = op
}

// StepNotifier is invoked before each workflow step is executed (or skipped).
// The idx argument is zero-based and maps directly to the Workflow specification order.
type StepNotifier func(idx int, step *types.Step, skipped bool)

func (wf *Workflow) Run(ctx context.Context, spec *types.Workflow, skip func(*types.Step) bool, notify StepNotifier) (results []StepResult[any]) {
	results = make([]StepResult[any], len(spec.Steps))

	if wf.op == steps.Delete {
		slices.Reverse(spec.Steps)
	}

	for i, x := range spec.Steps {
		results[i] = StepResult[any]{id: x.ID}

		if skip != nil && skip(x) {
			// wf.logr.Debug(fmt.Sprintf("skipping step with id: %s (%v)", x.ID, x.Type))
			wf.logger(fmt.Sprintf("skipping step with id: %s (%v)", x.ID, x.Type))
			if notify != nil {
				notify(i, x, true)
			}
			continue
		}

		wf.logger(fmt.Sprintf("executing step with id: %s (%v)", x.ID, x.Type))
		if notify != nil {
			notify(i, x, false)
		}

		switch x.Type {
		case types.TypeVar:
			wf.varHandler.Namespace(wf.ns)
			wf.varHandler.Op(wf.op)
			result, err := wf.varHandler.Handle(ctx, x.ID, x.With)
			results[i].res = result
			results[i].err = err

		case types.TypeObject:
			wf.objectHandler.Namespace(wf.ns)
			wf.objectHandler.Op(wf.op)
			result, err := wf.objectHandler.Handle(ctx, x.ID, x.With)
			results[i].res = result
			results[i].err = err

		case types.TypeChart:
			wf.chartHandler.Namespace(wf.ns)
			wf.chartHandler.Op(wf.op)
			result, err := wf.chartHandler.Handle(ctx, x.ID, x.With)
			results[i].res = result
			results[i].err = err

		default:
			results[i].err = fmt.Errorf("handler for step of type %q not found", x.Type)
		}

		if results[i].err != nil {
			return
		}
	}

	return
}
