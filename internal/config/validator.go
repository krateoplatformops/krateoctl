package config

import (
	"fmt"
)

// Validator performs validation on the configuration.
type Validator struct {
	config *Config
}

// NewValidator creates a new configuration validator.
func NewValidator(config *Config) *Validator {
	return &Validator{config: config}
}

// Validate performs comprehensive validation of the configuration.
// Returns an error if validation fails, nil otherwise.
func (v *Validator) Validate() error {
	modules, err := v.config.GetModules()
	if err != nil {
		return fmt.Errorf("failed to get modules: %w", err)
	}

	if err := v.validateModules(modules); err != nil {
		return err
	}

	return nil
}

// validateModules validates all module configurations.
func (v *Validator) validateModules(modules map[string]ModuleConfig) error {
	for name, mod := range modules {
		if err := v.validateModule(name, mod); err != nil {
			return err
		}
	}

	return nil
}

// validateModule validates a single module configuration.
func (v *Validator) validateModule(name string, mod ModuleConfig) error {
	if name == "" {
		return fmt.Errorf("module name cannot be empty")
	}

	// Check if module has chart config
	if mod.Chart != nil {
		hasRepo := mod.Chart.Repository != ""
		hasURL := mod.Chart.URL != ""
		if !hasRepo && !hasURL {
			return fmt.Errorf("module %s: chart must have repository or url", name)
		}
		if hasRepo {
			if mod.Chart.Name == "" && mod.Chart.Chart == "" {
				return fmt.Errorf("module %s: chart name is required when repository is specified", name)
			}
		}
	}

	return nil
}
