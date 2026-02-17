package config

import (
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/apimachinery/pkg/runtime"
)

// Config represents the fully loaded and validated Krateo configuration.
type Config struct {
	data map[string]interface{}
}

// NewConfig creates a new Config from loaded data.
func NewConfig(data map[string]interface{}) *Config {
	return &Config{data: data}
}

// GetModules returns the modules section of the config.
// Returns a map[string]interface{} where each key is a module name.
func (c *Config) GetModules() (map[string]interface{}, error) {
	modulesRaw, ok := c.data["modules"]
	if !ok {
		return make(map[string]interface{}), nil
	}

	modules, ok := modulesRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("modules must be a mapping, got %T", modulesRaw)
	}

	return modules, nil
}

// GetModule returns a single module's configuration by name.
func (c *Config) GetModule(name string) (map[string]interface{}, error) {
	modules, err := c.GetModules()
	if err != nil {
		return nil, err
	}

	modRaw, ok := modules[name]
	if !ok {
		return nil, fmt.Errorf("module %s not found", name)
	}

	mod, ok := modRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("module %s is not a mapping", name)
	}

	return mod, nil
}

// GetSteps returns the steps array from the configuration.
// Steps represent sequential operations: chart installations, variable extractions, etc.
func (c *Config) GetSteps() ([]*types.Step, error) {
	stepsRaw, ok := c.data["steps"]
	if !ok {
		return make([]*types.Step, 0), nil
	}

	stepsList, ok := stepsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("steps must be an array, got %T", stepsRaw)
	}

	steps := make([]*types.Step, 0, len(stepsList))
	for i, stepRaw := range stepsList {
		stepMap, ok := stepRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("step at index %d is not a mapping", i)
		}

		step := &types.Step{}

		// Extract id
		if id, ok := stepMap["id"].(string); ok {
			step.ID = id
		} else {
			return nil, fmt.Errorf("step at index %d missing id", i)
		}

		// Extract type
		if stepType, ok := stepMap["type"].(string); ok {
			step.Type = types.StepType(stepType)
		} else {
			return nil, fmt.Errorf("step %s missing type", step.ID)
		}

		// Extract with (configuration) - marshal to JSON for RawExtension
		if withData, ok := stepMap["with"]; ok {
			data, err := json.Marshal(withData)
			if err != nil {
				return nil, fmt.Errorf("step %s: failed to marshal configuration: %w", step.ID, err)
			}
			step.With = &runtime.RawExtension{Raw: data}
		}

		steps = append(steps, step)
	}

	return steps, nil
}

// GetBool safely extracts a boolean value from a nested path.
func (c *Config) GetBool(path []string, defaultVal bool) bool {
	val, err := c.getAtPath(path)
	if err != nil {
		return defaultVal
	}

	if bVal, ok := val.(bool); ok {
		return bVal
	}

	return defaultVal
}

// GetString safely extracts a string value from a nested path.
func (c *Config) GetString(path []string, defaultVal string) string {
	val, err := c.getAtPath(path)
	if err != nil {
		return defaultVal
	}

	if sVal, ok := val.(string); ok {
		return sVal
	}

	return defaultVal
}

// getAtPath navigates through nested maps following a path.
func (c *Config) getAtPath(path []string) (interface{}, error) {
	current := interface{}(c.data)

	for i, key := range path {
		if key == "" {
			continue
		}

		if currentMap, ok := current.(map[string]interface{}); ok {
			if next, exists := currentMap[key]; exists {
				current = next
			} else {
				return nil, fmt.Errorf("path not found at key: %s", key)
			}
		} else {
			return nil, fmt.Errorf("expected mapping at path segment %d (%s), got %T", i, key, current)
		}
	}

	return current, nil
}

// Set navigates through nested maps and sets a value at a path.
func (c *Config) Set(path []string, value interface{}) error {
	if len(path) == 0 {
		return fmt.Errorf("path cannot be empty")
	}

	// Navigate to parent
	current := interface{}(c.data)
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		if key == "" {
			continue
		}

		if currentMap, ok := current.(map[string]interface{}); ok {
			if next, exists := currentMap[key]; exists {
				current = next
			} else {
				// Create intermediate maps as needed
				newMap := make(map[string]interface{})
				currentMap[key] = newMap
				current = newMap
			}
		} else {
			return fmt.Errorf("expected mapping at path segment %d (%s), got %T", i, key, current)
		}
	}

	// Set the final value
	if currentMap, ok := current.(map[string]interface{}); ok {
		currentMap[path[len(path)-1]] = value
	} else {
		return fmt.Errorf("expected mapping at final position, got %T", current)
	}

	return nil
}

// Raw returns the underlying configuration map.
func (c *Config) Raw() map[string]interface{} {
	return c.data
}

// Add this method
func (c *Config) GetEnabledComponents() (map[string]bool, error) {
	componentsRaw, ok := c.data["components"].(map[string]interface{})
	if !ok {
		return map[string]bool{}, nil // No components defined
	}

	enabled := make(map[string]bool)
	for name, compRaw := range componentsRaw {
		if compMap, ok := compRaw.(map[string]interface{}); ok {
			if enabledVal, ok := compMap["enabled"].(bool); ok {
				enabled[name] = enabledVal
			} else {
				enabled[name] = true // Default to enabled
			}
		}
	}

	return enabled, nil
}

// Add this method to get the component that owns a step
func (c *Config) GetComponentForStep(stepID string) (string, error) {
	componentsRaw, ok := c.data["components"].(map[string]interface{})
	if !ok {
		return "", nil
	}

	for componentName, compRaw := range componentsRaw {
		if compMap, ok := compRaw.(map[string]interface{}); ok {
			if stepsRaw, ok := compMap["steps"].([]interface{}); ok {
				for _, s := range stepsRaw {
					if stepName, ok := s.(string); ok && stepName == stepID {
						return componentName, nil
					}
				}
			}
		}
	}

	return "", nil
}

// Add this method
func (c *Config) GetActiveSteps() ([]*types.Step, error) {
	steps, err := c.GetSteps()
	if err != nil {
		return nil, err
	}

	enabledComponents, err := c.GetEnabledComponents()
	if err != nil {
		return nil, err
	}

	// Mark steps based on component enablement
	for i, step := range steps {
		componentName, _ := c.GetComponentForStep(step.ID)
		if componentName != "" {
			if enabled, exists := enabledComponents[componentName]; exists && !enabled {
				steps[i].Skip = true
			}
		}
	}

	// Apply component-level overrides (values/charts) to chart steps
	if err := c.applyComponentOverrides(steps); err != nil {
		return nil, err
	}

	return steps, nil
}

// applyComponentOverrides merges component-level overrides into chart steps.
// Structure supported in configuration (typically via krateo-overrides.yaml):
//
// components:
//
//	<componentName>:
//	  enabled: true|false
//	  helmDefaults:       # optional defaults applied to all chart steps of this component
//	    ...
//	  stepConfig:         # optional per-step overrides keyed by step id
//	    <stepID>:
//	      helmValues:
//	        ...
//
// Merge order for a given chart step is:
//
//	base step values <- component.helmDefaults <- component.stepConfig[stepID].helmValues
//
// where later entries override earlier ones on key conflicts.
func (c *Config) applyComponentOverrides(steps []*types.Step) error {
	componentsRaw, ok := c.data["components"]
	if !ok {
		return nil
	}

	components, ok := componentsRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("components must be a mapping, got %T", componentsRaw)
	}

	for _, step := range steps {
		// Only chart steps have Helm values to override
		if step.Type != types.TypeChart || step.With == nil || len(step.With.Raw) == 0 {
			continue
		}

		componentName, _ := c.GetComponentForStep(step.ID)
		if componentName == "" {
			continue
		}

		compRaw, exists := components[componentName]
		if !exists {
			continue
		}

		compMap, ok := compRaw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("component %s must be a mapping, got %T", componentName, compRaw)
		}

		// Optional component-wide default values for all chart steps
		var compDefaults map[string]interface{}
		if v, ok := compMap["helmDefaults"]; ok {
			mv, ok := v.(map[string]interface{})
			if !ok {
				return fmt.Errorf("component %s helmDefaults must be a mapping, got %T", componentName, v)
			}
			compDefaults = mv
		}

		// spew.Dump(compMap)
		// Optional per-step overrides keyed by step ID
		var stepHelmValues map[string]interface{}
		if scRaw, ok := compMap["stepConfig"]; ok {
			scMap, ok := scRaw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("component %s stepConfig must be a mapping, got %T", componentName, scRaw)
			}

			if entryRaw, ok := scMap[step.ID]; ok {
				entryMap, ok := entryRaw.(map[string]interface{})
				if !ok {
					return fmt.Errorf("component %s stepConfig.%s must be a mapping, got %T", componentName, step.ID, entryRaw)
				}

				if v, ok := entryMap["helmValues"]; ok {
					mv, ok := v.(map[string]interface{})
					if !ok {
						return fmt.Errorf("component %s stepConfig.%s.helmValues must be a mapping, got %T", componentName, step.ID, v)
					}
					stepHelmValues = mv
				}
			}
		}

		// If there are no values to apply, skip
		if compDefaults == nil && stepHelmValues == nil {
			continue
		}

		// Unmarshal the step's current chart configuration
		withData := make(map[string]interface{})
		if err := json.Unmarshal(step.With.Raw, &withData); err != nil {
			return fmt.Errorf("step %s: failed to unmarshal chart configuration: %w", step.ID, err)
		}

		stepValues, _ := withData["values"].(map[string]interface{})
		if stepValues == nil {
			stepValues = make(map[string]interface{})
		}

		merged := stepValues
		if compDefaults != nil {
			merged = mergeConfigs(merged, compDefaults)
		}
		if stepHelmValues != nil {
			merged = mergeConfigs(merged, stepHelmValues)
		}

		withData["values"] = merged

		raw, err := json.Marshal(withData)
		if err != nil {
			return fmt.Errorf("step %s: failed to marshal chart configuration: %w", step.ID, err)
		}

		// fmt.Println("Applying overrides for step", step.ID)
		// fmt.Println("Raw merged configuration:\n", string(raw))
		step.With.Raw = raw
	}

	return nil
}
