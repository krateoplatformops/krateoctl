package apply

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"github.com/krateoplatformops/plumbing/helm/v3"

	"github.com/krateoplatformops/provider-runtime/pkg/logging"
)

func Command() subcommands.Command {
	return &applyCmd{}
}

type applyCmd struct {
	configFile string
	namespace  string
}

func (c *applyCmd) Name() string     { return "apply" }
func (c *applyCmd) Synopsis() string { return "apply configuration changes to cluster" }

func (c *applyCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s\n\n", c.Synopsis())
	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprintf(&wri, "  krateoctl %s [FLAGS]\n\n", c.Name())
	fmt.Fprint(&wri, "FLAGS:\n\n")
	return wri.String()
}

func (c *applyCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.configFile, "config", "krateo.yaml", "path to configuration file")
	f.StringVar(&c.namespace, "namespace", "krateo-system", "kubernetes namespace for deployment")
}

func (c *applyCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// Load configuration
	loader := config.NewLoader(config.LoadOptions{
		ConfigPath:        c.configFile,
		UserOverridesPath: "krateo-overrides.yaml",
	})

	data, err := loader.Load()
	if err != nil {
		fmt.Printf("✗ Failed to load configuration: %v\n", err)
		return subcommands.ExitFailure
	}

	cfg := config.NewConfig(data)

	// Validate configuration
	validator := config.NewValidator(cfg)
	if err := validator.Validate(); err != nil {
		fmt.Printf("✗ Configuration validation failed: %v\n", err)
		return subcommands.ExitFailure
	}

	// Get steps from configuration
	steps, err := cfg.GetSteps()
	if err != nil {
		fmt.Printf("✗ Failed to get steps: %v\n", err)
		return subcommands.ExitFailure
	}

	if len(steps) == 0 {
		fmt.Println("ℹ No steps configured")
		return subcommands.ExitSuccess
	}

	// Load Kubernetes configuration
	fmt.Println("\n📡 Connecting to Kubernetes cluster...")
	rc, err := kube.RestConfig()
	if err != nil {
		fmt.Printf("✗ Failed to load kubeconfig: %v\n", err)
		fmt.Println("  Make sure you have kubectl configured and KUBECONFIG is set")
		return subcommands.ExitFailure
	}

	// Create dynamic clients
	fmt.Println("✓ Kubernetes connection established")

	g, err := getter.NewGetter(rc)
	if err != nil {
		fmt.Printf("✗ Failed to create getter client: %v\n", err)
		return subcommands.ExitFailure
	}

	a, err := applier.NewApplier(rc)
	if err != nil {
		fmt.Printf("✗ Failed to create applier client: %v\n", err)
		return subcommands.ExitFailure
	}

	d, err := deletor.NewDeletor(rc)
	if err != nil {
		fmt.Printf("✗ Failed to create deletor client: %v\n", err)
		return subcommands.ExitFailure
	}

	// Create Helm client
	helmClient, err := helm.NewClient(rc, "default", slog.Default().Handler())
	if err != nil {
		fmt.Printf("✗ Failed to create Helm client: %v\n", err)
		return subcommands.ExitFailure
	}

	// Create workflow
	wf, err := workflows.New(workflows.Opts{
		Getter:         g,
		Applier:        a,
		Deletor:        d,
		Log:            logging.NewNopLogger(),
		HelmClient:     helmClient,
		MaxHelmHistory: 10,
		Namespace:      c.namespace,
	})
	if err != nil {
		fmt.Printf("✗ Failed to create workflow: %v\n", err)
		return subcommands.ExitFailure
	}

	// Create workflow spec from steps
	spec := &types.WorkflowSpec{
		Steps: steps,
	}

	// Execute workflow
	fmt.Printf("\n⚡ Applying %d steps to namespace '%s'...\n", len(steps), c.namespace)
	fmt.Println("═════════════════════════════════════════════════════════════")

	results := wf.Run(ctx, spec, func(s *types.Step) bool {
		// For this simple implementation, we run all steps sequentially
		return false
	})

	// Check for errors
	if err := workflows.Err(results); err != nil {
		fmt.Printf("\n✗ Workflow failed: %v\n", err)
		return subcommands.ExitFailure
	}

	// Report success
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Printf("✓ Successfully applied %d steps\n\n", len(steps))

	return subcommands.ExitSuccess
}
