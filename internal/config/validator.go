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
func (v *Validator) validateModules(modules map[string]interface{}) error {
	for name, modRaw := range modules {
		mod, ok := modRaw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("module %s is not a mapping", name)
		}

		if err := v.validateModule(name, mod); err != nil {
			return err
		}
	}

	return nil
}

// validateModule validates a single module configuration.
func (v *Validator) validateModule(name string, mod map[string]interface{}) error {
	if name == "" {
		return fmt.Errorf("module name cannot be empty")
	}

	// Check if module has chart config
	if chartRaw, ok := mod["chart"]; ok {
		if chart, ok := chartRaw.(map[string]interface{}); ok {
			// Chart must have either repository or URL
			_, hasRepo := chart["repository"].(string)
			_, hasURL := chart["url"].(string)
			if !hasRepo && !hasURL {
				return fmt.Errorf("module %s: chart must have repository or url", name)
			}
			// Chart name is required when repository is specified
			if repo, hasRepo := chart["repository"].(string); hasRepo && repo != "" {
				if chartName, ok := chart["name"].(string); !ok || chartName == "" {
					if chartName, ok := chart["chart"].(string); !ok || chartName == "" {
						return fmt.Errorf("module %s: chart name is required when repository is specified", name)
					}
				}
			}
		}
	}

	return nil
}
