package apply

import (
	"bytes"
	"context"
	"flag"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/client-go/rest"

	"github.com/krateoplatformops/provider-runtime/pkg/logging"
)

type workflowRunner interface {
	Run(context.Context, *types.Workflow, func(*types.Step) bool) []workflows.StepResult[any]
}

type restConfigProvider func() (*rest.Config, error)
type getterFactory func(*rest.Config) (*getter.Getter, error)
type applierFactory func(*rest.Config) (*applier.Applier, error)
type deletorFactory func(*rest.Config) (*deletor.Deletor, error)
type workflowFactory func(workflows.Opts) (workflowRunner, error)

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
}

func (c *applyCmd) Name() string     { return "apply" }
func (c *applyCmd) Synopsis() string { return "apply configuration changes to cluster" }

func (c *applyCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s. Load the installation config, compute the workflow steps and execute them against a Kubernetes cluster.\n\n", c.Synopsis())

	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprint(&wri, "  krateoctl install apply [FLAGS]\n\n")

	fmt.Fprint(&wri, "FLAGS:\n\n")
	fmt.Fprint(&wri, "  -config string\n")
	fmt.Fprint(&wri, "        path to installation configuration file (default \"krateo.yaml\")\n")
	fmt.Fprint(&wri, "  -namespace string\n")
	fmt.Fprint(&wri, "        Kubernetes namespace where resources will be created (default \"krateo-system\")\n")
	fmt.Fprint(&wri, "  -profile string\n")
	fmt.Fprint(&wri, "        optional profile name defined in krateo-overrides.yaml (e.g. dev, prod)\n\n")

	fmt.Fprint(&wri, "CONVENTIONS:\n\n")
	fmt.Fprint(&wri, "  - Main config is read from krateo.yaml (overridable with -config).\n")
	fmt.Fprint(&wri, "  - Overrides are loaded from krateo-overrides.yaml and, when -profile is set, from\n")
	fmt.Fprint(&wri, "    profile-specific files like krateo-overrides.<profile>.yaml.\n")
	fmt.Fprint(&wri, "  - Components and steps are resolved from the active profile; steps marked with\n")
	fmt.Fprint(&wri, "    'skip: true' in the plan are not executed.\n")
	fmt.Fprint(&wri, "  - Kubernetes connectivity is taken from your current kubeconfig (KUBECONFIG or\n")
	fmt.Fprint(&wri, "    default kubeconfig location).\n\n")

	fmt.Fprint(&wri, "EXAMPLES:\n\n")
	fmt.Fprint(&wri, "  # Apply the default configuration to the 'krateo-system' namespace\n")
	fmt.Fprint(&wri, "  krateoctl install apply\n\n")
	fmt.Fprint(&wri, "  # Apply the 'dev' profile configuration to a custom namespace\n")
	fmt.Fprint(&wri, "  krateoctl install apply -profile dev -namespace my-namespace\n\n")

	return wri.String()
}

func (c *applyCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.configFile, "config", "krateo.yaml", "path to configuration file")
	f.StringVar(&c.namespace, "namespace", "krateo-system", "kubernetes namespace for deployment")
	f.StringVar(&c.profile, "profile", "", "optional profile name defined in krateo-overrides.yaml")
}

func (c *applyCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	c.ensureDeps()

	result, err := shared.LoadConfigAndSteps(config.LoadOptions{
		ConfigPath:        c.configFile,
		UserOverridesPath: "krateo-overrides.yaml",
		Profile:           c.profile,
	})
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		return subcommands.ExitFailure
	}

	steps := result.Steps

	if len(steps) == 0 {
		fmt.Println("ℹ No steps configured")
		return subcommands.ExitSuccess
	}

	// Load Kubernetes configuration
	fmt.Println("\n📡 Connecting to Kubernetes cluster...")
	rc, err := c.restConfigFn()
	if err != nil {
		fmt.Printf("✗ Failed to load kubeconfig: %v\n", err)
		fmt.Println("  Make sure you have kubectl configured and KUBECONFIG is set")
		return subcommands.ExitFailure
	}

	// Create dynamic clients
	fmt.Println("✓ Kubernetes connection established")

	g, err := c.getterFactory(rc)
	if err != nil {
		fmt.Printf("✗ Failed to create getter client: %v\n", err)
		return subcommands.ExitFailure
	}

	a, err := c.applierFactory(rc)
	if err != nil {
		fmt.Printf("✗ Failed to create applier client: %v\n", err)
		return subcommands.ExitFailure
	}

	d, err := c.deletorFactory(rc)
	if err != nil {
		fmt.Printf("✗ Failed to create deletor client: %v\n", err)
		return subcommands.ExitFailure
	}

	// Create workflow
	log := logging.NewNopLogger()
	wf, err := c.workflowFactory(workflows.Opts{
		Getter:    g,
		Applier:   a,
		Deletor:   d,
		Log:       log,
		Cfg:       rc,
		Namespace: c.namespace,
	})
	if err != nil {
		fmt.Printf("✗ Failed to create workflow: %v\n", err)
		return subcommands.ExitFailure
	}

	// Create workflow spec from steps
	spec := &types.Workflow{
		Steps: steps,
	}

	// Execute workflow
	fmt.Printf("\n⚡ Applying %d steps to namespace '%s'...\n", len(steps), c.namespace)
	fmt.Println("═════════════════════════════════════════════════════════════")

	results := wf.Run(ctx, spec, func(s *types.Step) bool {
		return s.Skip // Skip steps marked with skip: true
	})

	// Check for errors
	if err := c.errEvaluator(results); err != nil {
		fmt.Printf("\n✗ Workflow failed: %v\n", err)
		return subcommands.ExitFailure
	}

	// Report success
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Printf("✓ Successfully applied %d steps\n\n", len(steps))

	return subcommands.ExitSuccess
}
