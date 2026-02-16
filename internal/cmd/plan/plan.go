package plan

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
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

	// Display plan report
	fmt.Println("\n📋 Plan Report (Sequential Steps)")
	fmt.Println("═════════════════════════════════════════════════════════════")

	for i, step := range steps {
		fmt.Printf("\nStep %d: %s [%s]\n", i+1, step.ID, step.Type)

		// For verbose output, show the configuration
		if c.dryRun && step.With != nil && len(step.With.Raw) > 0 {
			var config interface{}
			if err := json.Unmarshal(step.With.Raw, &config); err == nil {
				data, _ := json.MarshalIndent(config, "  ", "  ")
				fmt.Printf("  %s\n", string(data))
			}
		}
	}

	fmt.Println("\n═════════════════════════════════════════════════════════════")
	fmt.Printf("Summary: %d steps in dependency order\n", len(steps))
	fmt.Printf("Status: ✓ Ready to apply\n\n")

	return subcommands.ExitSuccess
}
