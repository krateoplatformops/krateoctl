package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"gopkg.in/yaml.v3"
)

func Command() subcommands.Command {
	return &planCmd{}
}

type planCmd struct {
	configFile string
	dryRun     bool
	profile    string
}

func (c *planCmd) Name() string     { return "plan" }
func (c *planCmd) Synopsis() string { return "preview configuration changes" }

func (c *planCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s. Load the installation config and print the computed workflow steps as multi-document YAML, without talking to the cluster.\n\n", c.Synopsis())

	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprint(&wri, "  krateoctl install plan [FLAGS]\n\n")

	fmt.Fprint(&wri, "FLAGS:\n\n")
	fmt.Fprint(&wri, "  -config string\n")
	fmt.Fprint(&wri, "        path to installation configuration file (default \"krateo.yaml\")\n")
	fmt.Fprint(&wri, "  -dry-run\n")
	fmt.Fprint(&wri, "        reserved flag; plan never contacts the cluster\n")
	fmt.Fprint(&wri, "  -profile string\n")
	fmt.Fprint(&wri, "        optional profile name defined in krateo-overrides.yaml (e.g. dev, prod)\n\n")

	fmt.Fprint(&wri, "CONVENTIONS:\n\n")
	fmt.Fprint(&wri, "  - Main config is read from krateo.yaml (overridable with -config).\n")
	fmt.Fprint(&wri, "  - Overrides are loaded from krateo-overrides.yaml and, when -profile is set, from\n")
	fmt.Fprint(&wri, "    profile-specific files like krateo-overrides.<profile>.yaml.\n")
	fmt.Fprint(&wri, "  - Components and steps are filtered according to the active profile; disabled steps\n")
	fmt.Fprint(&wri, "    are still shown but include 'skip: true' in the output.\n")
	fmt.Fprint(&wri, "  - Output is a stream of YAML documents, one per step, including 'id', 'type', an\n")
	fmt.Fprint(&wri, "    optional 'skip', and a 'with' section with the resolved step configuration.\n\n")

	fmt.Fprint(&wri, "EXAMPLES:\n\n")
	fmt.Fprint(&wri, "  # Preview all steps using the default config file\n")
	fmt.Fprint(&wri, "  krateoctl install plan\n\n")
	fmt.Fprint(&wri, "  # Preview steps for the 'dev' profile and save the plan\n")
	fmt.Fprint(&wri, "  krateoctl install plan -profile dev > plan.yaml\n\n")

	return wri.String()
}

func (c *planCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.configFile, "config", "krateo.yaml", "path to configuration file")
	f.BoolVar(&c.dryRun, "dry-run", false, "perform dry-run without connecting to cluster")
	f.StringVar(&c.profile, "profile", "", "optional profile name defined in krateo-overrides.yaml")
}

func (c *planCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// Load configuration
	loader := config.NewLoader(config.LoadOptions{
		ConfigPath:        c.configFile,
		UserOverridesPath: "krateo-overrides.yaml",
		Profile:           c.profile,
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

	// Get steps from configuration, respecting component enable/disable and
	// component-level overrides (including profile-specific overrides).
	steps, err := cfg.GetActiveSteps()
	if err != nil {
		fmt.Printf("✗ Failed to get steps: %v\n", err)
		return subcommands.ExitFailure
	}

	if len(steps) == 0 {
		fmt.Println("ℹ No steps configured")
		return subcommands.ExitSuccess
	}

	// Instead of the current pretty printer, emit multi‑doc YAML
	enc := yaml.NewEncoder(os.Stdout)
	defer enc.Close()

	for _, step := range steps {
		doc := map[string]any{
			"id":   step.ID,
			"type": step.Type,
		}

		// Include skip flag when the step is disabled for this profile/component.
		if step.Skip {
			doc["skip"] = true
		}

		if step.With != nil && len(step.With.Raw) > 0 {
			var with any
			if err := json.Unmarshal(step.With.Raw, &with); err == nil {
				doc["with"] = with
			}
		}

		if err := enc.Encode(doc); err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to encode plan document: %v\n", err)
			return subcommands.ExitFailure
		}
	}

	return subcommands.ExitSuccess
}
