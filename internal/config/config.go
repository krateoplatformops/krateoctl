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
