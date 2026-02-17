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
	fmt.Fprintf(&wri, "%s\n\n", c.Synopsis())
	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprintf(&wri, "  krateoctl %s [FLAGS]\n\n", c.Name())
	fmt.Fprint(&wri, "FLAGS:\n\n")
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
