package plan

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
)

func Command() subcommands.Command {
	return &planCmd{}
}

type restConfigProvider func() (*rest.Config, error)
type stateStoreFactory func(*rest.Config, string) (state.Store, error)

type planCmd struct {
	configFile     string
	profile        string
	namespace      string
	installType    string
	diffInstalled  bool
	diffFormat     string
	output         bool
	version        string
	repository     string
	debug          bool
	skipValidation bool
	restConfigFn   restConfigProvider
	stateFactory   stateStoreFactory
	stateName      string
}

func (c *planCmd) Name() string     { return "plan" }
func (c *planCmd) Synopsis() string { return "preview configuration changes" }

func (c *planCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s. Load the installation config and print the computed workflow steps as multi-document YAML, without talking to the cluster.\n\n", c.Synopsis())

	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprint(&wri, "  krateoctl install plan [FLAGS]\n\n")

	fmt.Fprint(&wri, "FLAGS:\n\n")
	fmt.Fprint(&wri, "  --version string\n")
	fmt.Fprint(&wri, "        version/tag to fetch from the releases repository (enables remote mode)\n")
	fmt.Fprint(&wri, "  --repository string\n")
	fmt.Fprint(&wri, "        GitHub repository URL for releases (default \"https://github.com/krateoplatformops/releases\")\n")
	fmt.Fprint(&wri, "  --config string\n")
	fmt.Fprintf(&wri, "        path to local configuration file (default \"%s\", used when --version is not set)\n", shared.DefaultConfigPath)
	fmt.Fprint(&wri, "  --profile string\n")
	fmt.Fprint(&wri, "        optional profile name (e.g. dev, prod)\n")
	fmt.Fprint(&wri, "  --namespace string\n")
	fmt.Fprintf(&wri, "        namespace where the installation snapshot is stored (default \"%s\")\n", shared.DefaultNamespace)
	fmt.Fprint(&wri, "  --type string\n")
	fmt.Fprint(&wri, "        choose which file variant to use. Supported values: nodeport, loadbalancer, ingress. For example, nodeport looks for krateo.nodeport.yaml and files like pre-upgrade.nodeport.yaml. (default \"nodeport\")\n")
	fmt.Fprint(&wri, "  --diff-installed\n")
	fmt.Fprint(&wri, "        compare computed plan against the stored installation snapshot\n")
	fmt.Fprint(&wri, "  --diff-format string\n")
	fmt.Fprint(&wri, "        choose how diffs are rendered: unified (default) or table\n")
	fmt.Fprint(&wri, "        table shows a step-by-step summary for the compared plan\n")
	fmt.Fprint(&wri, "  --output\n")
	fmt.Fprint(&wri, "        output computed plan steps as multi-document YAML to stdout\n")
	fmt.Fprint(&wri, "  --skip-validation\n")
	fmt.Fprint(&wri, "        skip configuration validation (useful for emergency recovery)\n")
	fmt.Fprint(&wri, "  --debug\n")
	fmt.Fprint(&wri, "        enable debug-level logging (can also use KRATEOCTL_DEBUG env var)\n\n")

	fmt.Fprint(&wri, "MODES:\n\n")
	fmt.Fprint(&wri, "  Remote mode: When --version is specified, config is fetched from the releases\n")
	fmt.Fprint(&wri, "               repository instead of local filesystem.\n")
	fmt.Fprint(&wri, "  Local mode:  When --version is not specified, config is read from local files.\n\n")
	fmt.Fprint(&wri, "CONVENTIONS:\n\n")
	fmt.Fprint(&wri, "  - Main config is read from krateo.yaml (overridable with --config in local mode).\n")
	fmt.Fprint(&wri, "  - Overrides are loaded from krateo-overrides.yaml and, when --profile is set, from\n")
	fmt.Fprint(&wri, "    profile-specific files like krateo-overrides.<profile>.yaml.\n")
	fmt.Fprint(&wri, "  - Components and steps are filtered according to the active profile; disabled steps\n")
	fmt.Fprint(&wri, "    are still shown but include 'skip: true' in the output.\n")
	fmt.Fprint(&wri, "  - Type-specific files such as pre-upgrade.nodeport.yaml are used first.\n")
	fmt.Fprint(&wri, "    If no type-specific file exists, the generic file pre-upgrade.yaml is used.\n")
	fmt.Fprint(&wri, "  - When --output is set, computed steps are written as a stream of YAML documents,\n")
	fmt.Fprint(&wri, "    one per step, including 'id', 'type', optional 'skip', and 'with' section.\n\n")

	fmt.Fprint(&wri, "EXAMPLES:\n\n")
	fmt.Fprint(&wri, "  # Preview from a specific release version (remote mode)\n")
	fmt.Fprint(&wri, "  krateoctl install plan --version v1.0.0\n\n")
	fmt.Fprint(&wri, "  # Preview from a custom repository\n")
	fmt.Fprint(&wri, "  krateoctl install plan --version v1.0.0 --repository https://github.com/myorg/krateo-releases\n\n")
	fmt.Fprint(&wri, "  # Preview using local config file\n")
	fmt.Fprint(&wri, "  krateoctl install plan --config ./my-krateo.yaml\n\n")
	fmt.Fprint(&wri, "  # Preview with a profile\n")
	fmt.Fprint(&wri, "  krateoctl install plan --version v1.0.0 --profile dev > plan.yaml\n\n")
	fmt.Fprint(&wri, "  # Preview using nodeport-specific files such as krateo.nodeport.yaml\n")
	fmt.Fprint(&wri, "  krateoctl install plan --config ./krateo.yaml --type nodeport\n\n")
	fmt.Fprint(&wri, "  # Preview using loadbalancer-specific files such as krateo.loadbalancer.yaml\n")
	fmt.Fprint(&wri, "  krateoctl install plan --config ./krateo.yaml --type loadbalancer\n\n")
	fmt.Fprint(&wri, "  # Preview using ingress-specific files such as krateo.ingress.yaml\n")
	fmt.Fprint(&wri, "  krateoctl install plan --config ./krateo.yaml --type ingress\n\n")

	return wri.String()
}

func (c *planCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.version, "version", "", "version/tag to fetch from the releases repository")
	f.StringVar(&c.repository, "repository", "", "GitHub repository URL for releases")
	f.StringVar(&c.configFile, "config", shared.DefaultConfigPath, "path to local configuration file")
	f.StringVar(&c.profile, "profile", "", "optional profile name")
	f.StringVar(&c.namespace, "namespace", shared.DefaultNamespace, "kubernetes namespace where the installation snapshot is stored")
	f.StringVar(&c.installType, "type", "nodeport", "choose which file variant to use: nodeport, loadbalancer, or ingress")
	f.BoolVar(&c.diffInstalled, "diff-installed", false, "compare the computed plan with the stored installation snapshot")
	f.StringVar(&c.diffFormat, "diff-format", "unified", "diff rendering mode: unified or table")
	f.BoolVar(&c.output, "output", false, "output computed plan steps as multi-document YAML")
	f.BoolVar(&c.skipValidation, "skip-validation", false, "skip configuration validation")
	f.BoolVar(&c.debug, "debug", false, "enable debug-level logging")
}

func (c *planCmd) ensureDeps() {
	if c.restConfigFn == nil {
		c.restConfigFn = kube.RestConfig
	}
	if c.stateFactory == nil {
		c.stateFactory = shared.DefaultStateStoreFactory
	}
	c.namespace = shared.EnsureNamespace(c.namespace)
	c.stateName = shared.EnsureStateName(c.stateName)
}

func (c *planCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	c.ensureDeps()

	// Enable debug mode from flag or environment variable
	l := shared.NewLogger(os.Stderr, c.debug || os.Getenv(shared.KRATEOCTL_DEBUG_ENV) != "")

	result, err := shared.LoadConfigAndSteps(shared.NewLoadOptions(shared.LoadOptionsInput{
		ConfigFile:       c.configFile,
		Namespace:        c.namespace,
		Profile:          c.profile,
		Version:          c.version,
		Repository:       c.repository,
		InstallationType: c.installType,
	}), c.namespace, l.Info, c.skipValidation)
	if err != nil {
		l.Error("Failed to load configuration: %v", err)
		return subcommands.ExitFailure
	}

	steps := result.Steps

	if len(steps) == 0 {
		l.Info("ℹ No steps configured")
		return subcommands.ExitSuccess
	}

	version := c.version
	if version == "" {
		version = "local"
	}
	snapshot, err := state.BuildSnapshot(result.Config, steps, version)
	if err != nil {
		l.Error("Failed to build installation snapshot: %v", err)
		return subcommands.ExitFailure
	}

	boriginalSteps, err := yaml.Marshal(result.OriginalSteps)
	if err != nil {
		l.Error("✗ Failed to marshal original steps: %v", err)
		return subcommands.ExitFailure
	}

	bSteps, err := yaml.Marshal(steps)
	if err != nil {
		l.Error("✗ Failed to marshal steps: %v", err)
		return subcommands.ExitFailure
	}

	// Output the computed configuration as YAML if requested
	if c.output {
		// Output the original configuration file
		if c.configFile != "" {
			configFile, err := os.Open(c.configFile)
			if err != nil {
				l.Error("✗ Failed to open config file: %v", err)
				return subcommands.ExitFailure
			}
			defer configFile.Close()

			if _, err := io.Copy(os.Stdout, configFile); err != nil {
				l.Error("✗ Failed to output config file: %v", err)
				return subcommands.ExitFailure
			}
		}
	} else {
		// Only show comparison messages when not outputting steps
		if c.diffInstalled {
			rc, err := c.restConfigFn()
			if err != nil {
				l.Error("Failed to load kubeconfig for diff: %v", err)
				return subcommands.ExitFailure
			}

			store, err := c.stateFactory(rc, c.namespace)
			if err != nil {
				l.Error("Failed to initialize installation state store: %v", err)
				return subcommands.ExitFailure
			}

			installed, err := store.Load(ctx, c.stateName)
			switch {
			case apierrors.IsNotFound(err):
				l.Info("ℹ Installation snapshot %q not found in namespace %q", c.stateName, c.namespace)
			case err != nil:
				l.Error("Failed to read installation snapshot: %v", err)
				return subcommands.ExitFailure
			default:
				installedBytes, err := yaml.Marshal(installed)
				if err != nil {
					l.Error("Failed to marshal stored snapshot: %v", err)
					return subcommands.ExitFailure
				}

				planBytes, err := yaml.Marshal(snapshot)
				if err != nil {
					l.Error("Failed to marshal computed snapshot: %v", err)
					return subcommands.ExitFailure
				}

				if err := c.renderDiff(l, os.Stderr, "installed", installedBytes, "plan", planBytes, installed, snapshot); err != nil {
					l.Error("%v", err)
					return subcommands.ExitFailure
				}
			}
		} else {
			if err := c.renderDiff(l, os.Stderr, "original", boriginalSteps, "computed", bSteps, result.OriginalSteps, steps); err != nil {
				l.Error("%v", err)
				return subcommands.ExitFailure
			}
		}
	}

	return subcommands.ExitSuccess
}
