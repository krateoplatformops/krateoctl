package apply

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/secrets"
	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/install/lifecycle"
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

type sharedWorkflowRunner struct {
	workflowRunner
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
	configFile     string
	namespace      string
	profile        string
	version        string
	repository     string
	installType    string
	debug          bool
	initSecrets    bool // Hidden utility flag for generating sample secrets
	skipValidation bool // Skip configuration validation

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
	wri := strings.Builder{}
	fmt.Fprintf(&wri, "%s. Load the installation config and execute the workflow.\n\n", c.Synopsis())
	fmt.Fprint(&wri, "USAGE:\n  krateoctl install apply [FLAGS]\n\n")
	fmt.Fprint(&wri, "FLAGS:\n")
	fmt.Fprint(&wri, "  --version string      version/tag to fetch from the releases repository (enables remote mode)\n")
	fmt.Fprint(&wri, "  --repository string   GitHub repository URL for releases (default \"https://github.com/krateoplatformops/releases\")\n")
	fmt.Fprintf(&wri, "  --config string       path to local configuration file (default \"%s\", used when --version is not set)\n", shared.DefaultConfigPath)
	fmt.Fprintf(&wri, "  --namespace string    target namespace (default \"%s\")\n", shared.DefaultNamespace)
	fmt.Fprint(&wri, "  --type string         installation type: nodeport, loadbalancer, or ingress (default \"nodepoint\")\n")
	fmt.Fprint(&wri, "  --profile string      optional profile name (e.g. dev, prod)\n")
	fmt.Fprint(&wri, "  --skip-validation     skip configuration validation (useful for emergency recovery)\n")
	fmt.Fprint(&wri, "  --debug               enable debug-level logging (can also use KRATEOCTL_DEBUG env var)\n\n")
	fmt.Fprint(&wri, "MODES:\n\n")
	fmt.Fprint(&wri, "  Remote mode: When --version is specified, config is fetched from the releases\n")
	fmt.Fprint(&wri, "               repository instead of local filesystem.\n")
	fmt.Fprint(&wri, "  Local mode:  When --version is not specified, config is read from local files.\n\n")
	fmt.Fprint(&wri, "EXAMPLES:\n\n")
	fmt.Fprint(&wri, "  # Apply from a specific release version (remote mode)\n")
	fmt.Fprint(&wri, "  krateoctl install apply --version v1.0.0\n\n")
	fmt.Fprint(&wri, "  # Apply from a custom repository\n")
	fmt.Fprint(&wri, "  krateoctl install apply --version v1.0.0 --repository https://github.com/myorg/krateo-releases\n\n")
	fmt.Fprint(&wri, "  # Apply using local config file\n")
	fmt.Fprint(&wri, "  krateoctl install apply --config ./my-krateo.yaml\n\n")
	fmt.Fprint(&wri, "  # Apply with LoadBalancer installation type\n")
	fmt.Fprint(&wri, "  krateoctl install apply --config ./krateo.yaml --type loadbalancer\n\n")
	fmt.Fprint(&wri, "  # Apply with Ingress installation type\n")
	fmt.Fprint(&wri, "  krateoctl install apply --config ./krateo.yaml --type ingress\n\n")
	return wri.String()
}

func (c *applyCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.version, "version", "", "version/tag to fetch from the releases repository")
	f.StringVar(&c.repository, "repository", "", "GitHub repository URL for releases")
	f.StringVar(&c.configFile, "config", shared.DefaultConfigPath, "path to local configuration file")
	f.StringVar(&c.namespace, "namespace", shared.DefaultNamespace, "kubernetes namespace for deployment")
	f.StringVar(&c.installType, "type", "nodeport", "installation type: nodeport, loadbalancer, or ingress")
	f.StringVar(&c.profile, "profile", "", "optional profile name")
	f.BoolVar(&c.skipValidation, "skip-validation", false, "skip configuration validation")
	f.BoolVar(&c.debug, "debug", false, "enable debug-level logging")
	// Hidden utility flag - not documented in Usage()
	f.BoolVar(&c.initSecrets, "init-secrets", false, "")
}

func (c *applyCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	c.ensureDeps()

	// 1. Initialize UI and Logging
	// Enable debug mode from flag or environment variable
	debugMode := c.debug || os.Getenv(shared.KRATEOCTL_DEBUG_ENV) != ""
	logLevel := ui.LevelInfo
	if debugMode {
		logLevel = ui.LevelDebug
	}

	spin := ui.NewSpinner(os.Stdout)
	l := ui.NewLogger(spin, logLevel)
	defer spin.Stop("")
	lifecycleManager := lifecycle.NewManager(c.namespace, func(cfg *rest.Config) (*getter.Getter, error) {
		return c.getterFactory(cfg)
	})

	// Generate a timestamp for unique job names (to avoid conflicts with previously failed jobs)
	jobNameSuffix := time.Now().Format("20060102-150405")

	// 2. Load Configuration
	result, err := shared.LoadConfigAndSteps(config.LoadOptions{
		ConfigPath:        c.configFile,
		UserOverridesPath: shared.DefaultOverridesPath,
		Profile:           c.profile,
		Version:           c.version,
		Repository:        c.repository,
	}, l.Info, c.skipValidation)
	if err != nil {
		l.Error("Failed to load configuration: %v", err)
		return subcommands.ExitFailure
	}

	if len(result.Steps) == 0 {
		l.Info("ℹ No steps configured")
		return subcommands.ExitSuccess
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

	// Initialize sample secrets if requested (hidden utility feature)
	if c.initSecrets {
		l.Info("\n🔐 Initializing sample secrets...")
		if err := secrets.InitializeSecrets(ctx, rc, c.namespace); err != nil {
			l.Error("Failed to initialize secrets: %v", err)
			return subcommands.ExitFailure
		}
		l.Info("✓ Sample secrets created successfully (%s, %s, %s) in namespace '%s'", secrets.KrateoDbSecretName, secrets.KrateoDbUserSecretName, secrets.JWTSecretName, c.namespace)
	}

	// 4.5. Apply Pre-Upgrade Manifests (if they exist)
	a, err := c.applierFactory(rc)
	if err != nil {
		l.Error("Failed to initialize applier: %v", err)
		return subcommands.ExitFailure
	}

	if err := lifecycleManager.Apply(ctx, a, l, "pre-upgrade", c.version, c.repository, c.configFile, rc, jobNameSuffix); err != nil {
		l.Error("Failed to apply pre-upgrade manifests: %v", err)
		return subcommands.ExitFailure
	}

	// 5. Execute Workflow
	l.Info("\n⚡ Applying %d steps to namespace '%s'...", len(result.Steps), c.namespace)
	l.Info("═════════════════════════════════════════════════════════════")

	// Start Spinner
	spin.SetPrefix("⚙  ")
	spin.Start()

	// Use the shared workflow executor for the core apply path.
	execResult, err := shared.ExecuteWorkflow(ctx, rc, shared.ExecuteWorkflowOptions{
		Namespace:        c.namespace,
		StateName:        c.stateName,
		Logger:           l,
		Result:           result,
		ProgressReporter: c.createProgressReporter(spin, l, len(result.Steps)),
		SaveState:        false,
	}, shared.WorkflowDeps{
		GetterFactory:  shared.GetterFactory(c.getterFactory),
		ApplierFactory: shared.ApplierFactory(c.applierFactory),
		DeletorFactory: shared.DeletorFactory(c.deletorFactory),
		WorkflowFactory: func(opts workflows.Opts) (shared.WorkflowRunner, error) {
			wf, err := c.workflowFactory(opts)
			if err != nil {
				return nil, err
			}
			return sharedWorkflowRunner{workflowRunner: wf}, nil
		},
		ErrEvaluator: shared.ErrEvaluator(c.errEvaluator),
		StateFactory: shared.StateStoreFactory(c.stateFactory),
	})
	spin.Stop("")

	// 6. Final Report
	l.Info("═════════════════════════════════════════════════════════════")
	if execResult != nil {
		shared.LogWorkflowResults(l, result.Steps, execResult.Results)
	}

	if err != nil {
		l.Error("\nWorkflow completed with errors.")
		return subcommands.ExitFailure
	}

	// 6.5. Apply Post-Upgrade Manifests (if they exist)
	if err := lifecycleManager.Apply(ctx, a, l, "post-upgrade", c.version, c.repository, c.configFile, rc, jobNameSuffix); err != nil {
		l.Error("Failed to apply post-upgrade manifests: %v", err)
		return subcommands.ExitFailure
	}

	if execResult != nil && execResult.Snapshot != nil {
		store, storeErr := c.stateFactory(rc, c.namespace)
		if storeErr != nil {
			l.Error("Failed to initialize installation state store: %v", storeErr)
			return subcommands.ExitFailure
		}
		if err := store.Save(ctx, c.stateName, execResult.Snapshot); err != nil {
			l.Warn("⚠ Unable to persist installation snapshot: %v", err)
		} else {
			l.Info("✓ Installation snapshot saved as %q with apiVersion %q and kind %q in the namespace %q", c.stateName, "krateo.io/v1", "Installation", c.namespace)
		}
	}

	l.Info("✓ Successfully applied %d steps\n", len(result.Steps))
	return subcommands.ExitSuccess
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
