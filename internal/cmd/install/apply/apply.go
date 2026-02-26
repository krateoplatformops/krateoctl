package apply

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/client-go/rest"
)

type workflowRunner interface {
	Run(context.Context, *types.Workflow, func(*types.Step) bool, workflows.StepNotifier) []workflows.StepResult[any]
}

type restConfigProvider func() (*rest.Config, error)
type getterFactory func(*rest.Config) (*getter.Getter, error)
type applierFactory func(*rest.Config) (*applier.Applier, error)
type deletorFactory func(*rest.Config) (*deletor.Deletor, error)
type workflowFactory func(workflows.Opts) (workflowRunner, error)
type stateStoreFactory func(*rest.Config, string) (state.Store, error)
type ensureCRDFunc func(context.Context, *rest.Config) error

func Command() subcommands.Command {
	return &applyCmd{}
}

type applyCmd struct {
	configFile string
	namespace  string
	profile    string

	restConfigFn    restConfigProvider
	getterFactory   getterFactory
	applierFactory  applierFactory
	deletorFactory  deletorFactory
	workflowFactory workflowFactory
	errEvaluator    func([]workflows.StepResult[any]) error
	stateFactory    stateStoreFactory
	ensureCRDFn     ensureCRDFunc
	stateName       string
}

func (c *applyCmd) ensureDeps() {
	if c.restConfigFn == nil {
		c.restConfigFn = kube.RestConfig
	}
	if c.getterFactory == nil {
		c.getterFactory = getter.NewGetter
	}
	if c.applierFactory == nil {
		c.applierFactory = applier.NewApplier
	}
	if c.deletorFactory == nil {
		c.deletorFactory = deletor.NewDeletor
	}
	if c.workflowFactory == nil {
		c.workflowFactory = func(opts workflows.Opts) (workflowRunner, error) {
			return workflows.New(opts)
		}
	}
	if c.errEvaluator == nil {
		c.errEvaluator = func(results []workflows.StepResult[any]) error {
			return workflows.Err(results)
		}
	}
	if c.stateFactory == nil {
		c.stateFactory = func(cfg *rest.Config, namespace string) (state.Store, error) {
			return state.NewStore(cfg, namespace)
		}
	}
	if c.ensureCRDFn == nil {
		c.ensureCRDFn = state.EnsureCRD
	}
	if c.stateName == "" {
		c.stateName = state.DefaultInstallationName
	}
}

func (c *applyCmd) Name() string     { return "apply" }
func (c *applyCmd) Synopsis() string { return "apply configuration changes to cluster" }

func (c *applyCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s. Load the installation config and execute the workflow.\n\n", c.Synopsis())
	fmt.Fprint(&wri, "USAGE:\n  krateoctl install apply [FLAGS]\n\n")
	fmt.Fprint(&wri, "FLAGS:\n")
	fmt.Fprintf(&wri, "  -config string      path to config (default \"%s\")\n", shared.DefaultConfigPath)
	fmt.Fprintf(&wri, "  -namespace string   target namespace (default \"%s\")\n", shared.DefaultNamespace)
	fmt.Fprint(&wri, "  -profile string     optional override profile\n")
	return wri.String()
}

func (c *applyCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.configFile, "config", shared.DefaultConfigPath, "path to configuration file")
	f.StringVar(&c.namespace, "namespace", shared.DefaultNamespace, "kubernetes namespace for deployment")
	f.StringVar(&c.profile, "profile", "", "optional profile name")
}

func (c *applyCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	c.ensureDeps()

	// 1. Initialize UI and Logging
	debugMode := os.Getenv("KRATEO_DEBUG") != "" || c.profile == "debug"
	logLevel := ui.LevelInfo
	if debugMode {
		logLevel = ui.LevelDebug
	}

	spin := ui.NewSpinner(os.Stdout)
	l := ui.NewLogger(spin, logLevel)
	defer spin.Stop("")

	var installationStore state.Store

	// 2. Load Configuration
	result, err := shared.LoadConfigAndSteps(config.LoadOptions{
		ConfigPath:        c.configFile,
		UserOverridesPath: shared.DefaultOverridesPath,
		Profile:           c.profile,
	})
	if err != nil {
		l.Error("Failed to load configuration: %v", err)
		return subcommands.ExitFailure
	}

	if len(result.Steps) == 0 {
		l.Info("ℹ No steps configured")
		return subcommands.ExitSuccess
	}

	snapshot, err := state.BuildSnapshot(result.Config, result.Steps)
	if err != nil {
		l.Error("Failed to build installation snapshot: %v", err)
		return subcommands.ExitFailure
	}

	// 3. Setup Kubernetes Connection
	l.Info("\n📡 Connecting to Kubernetes cluster...")
	rc, err := c.restConfigFn()
	if err != nil {
		l.Error("Failed to load kubeconfig: %v", err)
		return subcommands.ExitFailure
	}
	l.Info("✓ Kubernetes connection established")

	if err := c.ensureCRDFn(ctx, rc); err != nil {
		l.Error("Failed to ensure installation CRD: %v", err)
		return subcommands.ExitFailure
	}

	installationStore, err = c.stateFactory(rc, c.namespace)
	if err != nil {
		l.Error("Failed to initialize installation state store: %v", err)
		return subcommands.ExitFailure
	}

	// 4. Initialize Workflow
	wf, err := c.initWorkflow(rc, l)
	if err != nil {
		l.Error("Initialization failed: %v", err)
		return subcommands.ExitFailure
	}

	// 5. Execute Workflow
	l.Info("\n⚡ Applying %d steps to namespace '%s'...", len(result.Steps), c.namespace)
	l.Info("═════════════════════════════════════════════════════════════")

	// Start Spinner
	spin.SetPrefix("⚙  ")
	spin.Start()

	// Pass the Progress Reporter (moved to a local helper)
	results := wf.Run(ctx, &types.Workflow{Steps: result.Steps}, func(s *types.Step) bool {
		return s.Skip
	}, c.createProgressReporter(spin, l, len(result.Steps)))

	spin.Stop("")

	// 6. Final Report
	l.Info("═════════════════════════════════════════════════════════════")
	c.printSummary(l, result.Steps, results)

	if err := c.errEvaluator(results); err != nil {
		l.Error("\nWorkflow completed with errors.")
		return subcommands.ExitFailure
	}

	if installationStore != nil && snapshot != nil {
		if err := installationStore.Save(ctx, c.stateName, snapshot); err != nil {
			l.Warn("⚠ Unable to persist installation snapshot: %v", err)
		} else {
			l.Info("✓ Installation snapshot saved as %q", c.stateName)
		}
	}

	l.Info("✓ Successfully applied %d steps\n", len(result.Steps))
	return subcommands.ExitSuccess
}

// initWorkflow abstracts the creation of the workflow and its clients.
func (c *applyCmd) initWorkflow(rc *rest.Config, l *ui.Logger) (workflowRunner, error) {
	g, err := c.getterFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("getter: %w", err)
	}
	a, err := c.applierFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("applier: %w", err)
	}
	d, err := c.deletorFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("deletor: %w", err)
	}

	return c.workflowFactory(workflows.Opts{
		Getter:    g,
		Applier:   a,
		Deletor:   d,
		Logger:    l.Debug,
		Cfg:       rc,
		Namespace: c.namespace,
	})
}

func (c *applyCmd) createProgressReporter(spin *ui.Spinner, l *ui.Logger, total int) workflows.StepNotifier {
	return func(idx int, step *types.Step, skipped bool) {
		status := "executing"
		if skipped {
			status = "skipped"
		}
		spin.SetSuffix(fmt.Sprintf("step %d/%d - %s (%s)", idx+1, total, step.ID, status))

		l.V(ui.LevelDebug).Info("Processing workflow step: index=%d id=%s type=%s",
			idx+1, step.ID, step.Type)
	}
}

func (c *applyCmd) printSummary(l *ui.Logger, steps []*types.Step, results []workflows.StepResult[any]) {
	for i, step := range steps {
		res := results[i]
		switch {
		case res.ID() == "":
			l.Info("[PEND] %s (%s) not executed", step.ID, step.Type)
		case step.Skip:
			l.Info("[SKIP] %s (%s)", step.ID, step.Type)
		case res.Err() != nil:
			l.Error("%s (%s) failed: %v", step.ID, step.Type, res.Err())
		default:
			l.Info("✓ %s (%s)", step.ID, step.Type)
		}
	}
}
