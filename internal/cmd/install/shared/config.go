package shared

import (
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
)

const (
	DefaultConfigPath    = "krateo.yaml"
	DefaultOverridesPath = "krateo-overrides.yaml"
	DefaultNamespace     = "krateo-system"
)

// LoadResult contains the validated configuration and the resolved workflow steps.
type LoadResult struct {
	Config        *config.Config
	Steps         []*types.Step
	OriginalSteps []*types.Step
}

// LoadConfigAndSteps loads the Krateo configuration, validates it, and resolves the active steps.
func LoadConfigAndSteps(opts config.LoadOptions) (*LoadResult, error) {
	loader := config.NewLoader(opts)

	data, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("Failed to load configuration: %w", err)
	}

	cfg, err := config.NewConfig(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to build configuration: %w", err)
	}

	validator := config.NewValidator(cfg)
	if err := validator.Validate(); err != nil {
		return nil, fmt.Errorf("Configuration validation failed: %w", err)
	}

	steps, err := cfg.GetActiveSteps()
	if err != nil {
		return nil, fmt.Errorf("Failed to get steps: %w", err)
	}

	originalSteps, err := cfg.GetSteps()
	if err != nil {
		return nil, fmt.Errorf("Failed to get original steps: %w", err)
	}

	return &LoadResult{
		Config:        cfg,
		Steps:         steps,
		OriginalSteps: originalSteps,
	}, nil
}
