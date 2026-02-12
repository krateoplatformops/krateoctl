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
	// Validate modules section
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

	// Check for circular dependencies
	if err := v.checkCircularDeps(modules); err != nil {
		return err
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

// checkCircularDeps detects circular dependencies between modules.
func (v *Validator) checkCircularDeps(modules map[string]interface{}) error {
	visited := make(map[string]bool)
	path := make(map[string]bool)

	for name := range modules {
		if !visited[name] {
			if err := v.dfs(name, modules, visited, path); err != nil {
				return err
			}
		}
	}

	return nil
}

// dfs performs depth-first search to detect cycles.
func (v *Validator) dfs(name string, modules map[string]interface{}, visited, path map[string]bool) error {
	if path[name] {
		return fmt.Errorf("circular dependency detected: %s", name)
	}

	if visited[name] {
		return nil
	}

	visited[name] = true
	path[name] = true

	modRaw, ok := modules[name]
	if !ok {
		return fmt.Errorf("module %s not found", name)
	}

	mod, ok := modRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("module %s is not a mapping", name)
	}

	// Check depends field
	if depsRaw, ok := mod["depends"]; ok {
		if deps, ok := depsRaw.([]interface{}); ok {
			for _, depRaw := range deps {
				if dep, ok := depRaw.(string); ok {
					if depMod, ok := modules[dep]; ok {
						if depMap, ok := depMod.(map[string]interface{}); ok {
							if enabled, ok := depMap["enabled"].(bool); !ok || enabled {
								if err := v.dfs(dep, modules, visited, path); err != nil {
									return err
								}
							}
						}
					} else {
						return fmt.Errorf("module %s depends on non-existent module %s", name, dep)
					}
				}
			}
		}
	}

	path[name] = false
	return nil
}
