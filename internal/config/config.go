package config

import (
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
)

// Config represents the fully loaded and validated Krateo configuration.
type Config struct {
	data map[string]interface{}
	doc  *Document
}

// NewConfig creates a new Config from loaded data.
func NewConfig(data map[string]interface{}) (*Config, error) {
	if data == nil {
		data = make(map[string]interface{})
	}

	doc, err := decodeDocument(data)
	if err != nil {
		return nil, err
	}

	return &Config{data: data, doc: doc}, nil
}

// Document returns the typed configuration document backing this Config instance.
func (c *Config) Document() *Document {
	return c.doc
}

// GetModules returns the modules section of the config.
// Returns a map[string]ModuleConfig where each key is a module name.
func (c *Config) GetModules() (map[string]ModuleConfig, error) {
	if c.doc == nil || len(c.doc.Modules) == 0 {
		return make(map[string]ModuleConfig), nil
	}

	return c.doc.Modules, nil
}

// GetModule returns a single module's configuration by name.
func (c *Config) GetModule(name string) (ModuleConfig, error) {
	modules, err := c.GetModules()
	if err != nil {
		return ModuleConfig{}, err
	}

	mod, ok := modules[name]
	if !ok {
		return ModuleConfig{}, fmt.Errorf("module %s not found", name)
	}

	return mod, nil
}

// GetSteps returns the steps array from the configuration.
// Steps represent sequential operations: chart installations, variable extractions, etc.
func (c *Config) GetSteps() ([]*types.Step, error) {
	if c.doc == nil || len(c.doc.Steps) == 0 {
		return make([]*types.Step, 0), nil
	}

	steps := make([]*types.Step, 0, len(c.doc.Steps))
	for i, def := range c.doc.Steps {
		if def.ID == "" {
			return nil, fmt.Errorf("step at index %d missing id", i)
		}
		if def.Type == "" {
			return nil, fmt.Errorf("step %s missing type", def.ID)
		}

		var with *runtime.RawExtension
		if def.With != nil {
			data, err := json.Marshal(def.With)
			if err != nil {
				return nil, fmt.Errorf("step %s: failed to marshal configuration: %w", def.ID, err)
			}
			with = &runtime.RawExtension{Raw: data}
		}

		steps = append(steps, &types.Step{
			ID:   def.ID,
			Type: def.Type,
			With: with,
		})
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
	if c.doc == nil || len(c.doc.Components) == 0 {
		return map[string]bool{}, nil
	}

	enabled := make(map[string]bool)
	for name, comp := range c.doc.Components {
		if comp.Enabled == nil {
			enabled[name] = true
			continue
		}
		enabled[name] = *comp.Enabled
	}

	return enabled, nil
}

// Add this method to get the component that owns a step
func (c *Config) GetComponentForStep(stepID string) (string, error) {
	if c.doc == nil || len(c.doc.Components) == 0 {
		return "", nil
	}

	for name, comp := range c.doc.Components {
		for _, step := range comp.Steps {
			if step == stepID {
				return name, nil
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
//	base step with    <- component.helmDefaults.with <- component.stepConfig[stepID].with
//
// where later entries override earlier ones on key conflicts.
func (c *Config) applyComponentOverrides(steps []*types.Step) error {
	if c.doc == nil || len(c.doc.Components) == 0 {
		return nil
	}

	for _, step := range steps {
		if step.Type != types.TypeChart || step.With == nil || len(step.With.Raw) == 0 {
			continue
		}

		componentName, _ := c.GetComponentForStep(step.ID)
		if componentName == "" {
			continue
		}

		component, ok := c.doc.Components[componentName]
		if !ok {
			continue
		}

		withData := make(map[string]interface{})
		if err := json.Unmarshal(step.With.Raw, &withData); err != nil {
			return fmt.Errorf("step %s: failed to unmarshal chart configuration: %w", step.ID, err)
		}

		stepValues := ensureMap(withData, "values")

		defaultValues, defaultWith, err := splitDefaults(component.HelmDefaults, componentName)
		if err != nil {
			return err
		}
		if len(defaultValues) > 0 {
			stepValues = mergeConfigs(stepValues, defaultValues)
		}
		if len(defaultWith) > 0 {
			withData = mergeConfigs(withData, defaultWith)
		}

		stepSpecificValues, stepSpecificWith, err := parseStepOverrides(component.StepConfig, componentName, step.ID)
		if err != nil {
			return err
		}
		if len(stepSpecificValues) > 0 {
			stepValues = mergeConfigs(stepValues, stepSpecificValues)
		}
		if len(stepSpecificWith) > 0 {
			withData = mergeConfigs(withData, stepSpecificWith)
		}

		raw, err := json.Marshal(withData)
		if err != nil {
			return fmt.Errorf("step %s: failed to marshal chart configuration: %w", step.ID, err)
		}

		step.With.Raw = raw
	}

	return nil
}

func splitDefaults(source map[string]interface{}, componentName string) (map[string]interface{}, map[string]interface{}, error) {
	if len(source) == 0 {
		return nil, nil, nil
	}

	values := make(map[string]interface{})
	var with map[string]interface{}

	for key, val := range source {
		if key == "with" {
			if val == nil {
				continue
			}
			mv, ok := val.(map[string]interface{})
			if !ok {
				return nil, nil, fmt.Errorf("component %s helmDefaults.with must be a mapping, got %T", componentName, val)
			}
			with = mv
			continue
		}
		values[key] = val
	}

	if len(values) == 0 {
		values = nil
	}

	return values, with, nil
}

func parseStepOverrides(stepConfig map[string]map[string]interface{}, componentName, stepID string) (map[string]interface{}, map[string]interface{}, error) {
	if len(stepConfig) == 0 {
		return nil, nil, nil
	}

	entry, ok := stepConfig[stepID]
	if !ok || len(entry) == 0 {
		return nil, nil, nil
	}

	var values map[string]interface{}
	var with map[string]interface{}

	if raw, ok := entry["helmValues"]; ok {
		mv, ok := raw.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("component %s stepConfig.%s.helmValues must be a mapping, got %T", componentName, stepID, raw)
		}
		values = mv
	}

	if raw, ok := entry["with"]; ok {
		mv, ok := raw.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("component %s stepConfig.%s.with must be a mapping, got %T", componentName, stepID, raw)
		}
		with = mv
	}

	return values, with, nil
}

func ensureMap(target map[string]interface{}, key string) map[string]interface{} {
	if existing, ok := target[key]; ok {
		if existingMap, ok := existing.(map[string]interface{}); ok {
			return existingMap
		}
	}

	newMap := make(map[string]interface{})
	target[key] = newMap
	return newMap
}

func decodeDocument(data map[string]interface{}) (*Document, error) {
	body, err := yaml.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config data: %w", err)
	}

	var doc Document
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("failed to decode config document: %w", err)
	}

	return &doc, nil
}
