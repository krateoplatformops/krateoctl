package config

import (
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"gopkg.in/yaml.v3"
)

// Config represents the fully loaded and validated Krateo configuration.
type Config struct {
	data map[string]any
	doc  *Document
}

// NewConfig creates a new Config from loaded data.
func NewConfig(data map[string]any) (*Config, error) {
	if data == nil {
		data = make(map[string]any)
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

		var with *map[string]any
		if def.With != nil {
			data, err := json.Marshal(def.With)
			if err != nil {
				return nil, fmt.Errorf("step %s: failed to marshal configuration: %w", def.ID, err)
			}
			with = &map[string]any{}
			if err := json.Unmarshal(data, with); err != nil {
				return nil, fmt.Errorf("step %s: failed to unmarshal configuration: %w", def.ID, err)
			}
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
func (c *Config) getAtPath(path []string) (any, error) {
	current := any(c.data)

	for i, key := range path {
		if key == "" {
			continue
		}

		if currentMap, ok := current.(map[string]any); ok {
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
func (c *Config) Set(path []string, value any) error {
	if len(path) == 0 {
		return fmt.Errorf("path cannot be empty")
	}

	// Navigate to parent
	current := any(c.data)
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		if key == "" {
			continue
		}

		if currentMap, ok := current.(map[string]any); ok {
			if next, exists := currentMap[key]; exists {
				current = next
			} else {
				// Create intermediate maps as needed
				newMap := make(map[string]any)
				currentMap[key] = newMap
				current = newMap
			}
		} else {
			return fmt.Errorf("expected mapping at path segment %d (%s), got %T", i, key, current)
		}
	}

	// Set the final value
	if currentMap, ok := current.(map[string]any); ok {
		currentMap[path[len(path)-1]] = value
	} else {
		return fmt.Errorf("expected mapping at final position, got %T", current)
	}

	return nil
}

// Raw returns the underlying configuration map.
func (c *Config) Raw() map[string]any {
	return c.data
}

func (c *Config) componentDefinitions() map[string]ComponentConfig {
	if c.doc == nil {
		return nil
	}

	defs := c.doc.ComponentsDefinition
	overrides := c.doc.Components

	switch {
	case len(defs) == 0 && len(overrides) == 0:
		return nil
	case len(defs) == 0:
		return overrides
	case len(overrides) == 0:
		return defs
	default:
		merged := make(map[string]ComponentConfig, len(defs)+len(overrides))
		for name, comp := range defs {
			merged[name] = comp
		}
		for name, comp := range overrides {
			if _, exists := merged[name]; !exists {
				merged[name] = comp
			}
		}
		return merged
	}
}

func (c *Config) componentOverrides() map[string]ComponentConfig {
	if c.doc == nil || len(c.doc.Components) == 0 {
		return nil
	}
	return c.doc.Components
}

// Add this method
func (c *Config) GetEnabledComponents() (map[string]bool, error) {
	definitions := c.componentDefinitions()
	overrides := c.componentOverrides()

	enabled := make(map[string]bool)
	if len(definitions) == 0 && len(overrides) == 0 {
		return enabled, nil
	}

	for name, comp := range definitions {
		val := true
		if comp.Enabled != nil {
			val = *comp.Enabled
		}
		if overrides != nil {
			if ov, ok := overrides[name]; ok && ov.Enabled != nil {
				val = *ov.Enabled
			}
		}
		enabled[name] = val
	}

	for name, ov := range overrides {
		if _, ok := enabled[name]; ok {
			continue
		}
		val := true
		if ov.Enabled != nil {
			val = *ov.Enabled
		}
		enabled[name] = val
	}

	return enabled, nil
}

// Add this method to get the component that owns a step
func (c *Config) GetComponentForStep(stepID string) (string, error) {
	definitions := c.componentDefinitions()
	if len(definitions) == 0 {
		return "", nil
	}

	for name, comp := range definitions {
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
	definitions := c.componentDefinitions()
	if len(definitions) == 0 {
		return nil
	}

	overrides := c.componentOverrides()

	for _, step := range steps {
		if step.Type != types.TypeChart || step.With == nil {
			continue
		}

		componentName, _ := c.GetComponentForStep(step.ID)
		if componentName == "" {
			continue
		}

		component, ok := definitions[componentName]
		if !ok {
			continue
		}

		var override ComponentConfig
		if overrides != nil {
			if ov, ok := overrides[componentName]; ok {
				override = ov
			}
		}

		defaultsSrc := mergeComponentValueMaps(component.HelmDefaults, override.HelmDefaults)
		stepConfigSrc := mergeStepConfig(component.StepConfig, override.StepConfig)

		if step.With == nil {
			step.With = &map[string]any{}
		}
		withData := *step.With

		stepValues := ensureMap(withData, "values")

		defaultValues, defaultWith, err := splitDefaults(defaultsSrc, componentName)
		if err != nil {
			return err
		}
		if len(defaultValues) > 0 {
			stepValues = mergeConfigs(stepValues, defaultValues)
		}
		if len(defaultWith) > 0 {
			withData = mergeConfigs(withData, defaultWith)
		}

		stepSpecificValues, stepSpecificWith, err := parseStepOverrides(stepConfigSrc, componentName, step.ID)
		if err != nil {
			return err
		}
		if len(stepSpecificValues) > 0 {
			stepValues = mergeConfigs(stepValues, stepSpecificValues)
		}
		if len(stepSpecificWith) > 0 {
			withData = mergeConfigs(withData, stepSpecificWith)
		}

		step.With = &withData
	}

	return nil
}

func splitDefaults(source map[string]any, componentName string) (map[string]any, map[string]any, error) {
	if len(source) == 0 {
		return nil, nil, nil
	}

	values := make(map[string]any)
	var with map[string]any

	for key, val := range source {
		if key == "with" {
			if val == nil {
				continue
			}
			mv, ok := val.(map[string]any)
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

func parseStepOverrides(stepConfig map[string]map[string]any, componentName, stepID string) (map[string]any, map[string]any, error) {
	if len(stepConfig) == 0 {
		return nil, nil, nil
	}

	entry, ok := stepConfig[stepID]
	if !ok || len(entry) == 0 {
		return nil, nil, nil
	}

	var values map[string]any
	var with map[string]any

	if raw, ok := entry["helmValues"]; ok {
		mv, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("component %s stepConfig.%s.helmValues must be a mapping, got %T", componentName, stepID, raw)
		}
		values = mv
	}

	if raw, ok := entry["with"]; ok {
		mv, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("component %s stepConfig.%s.with must be a mapping, got %T", componentName, stepID, raw)
		}
		with = mv
	}

	return values, with, nil
}

func ensureMap(target map[string]any, key string) map[string]any {
	if existing, ok := target[key]; ok {
		if existingMap, ok := existing.(map[string]any); ok {
			return existingMap
		}
	}

	newMap := make(map[string]any)
	target[key] = newMap
	return newMap
}

func mergeComponentValueMaps(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	merged := make(map[string]any)
	if len(base) > 0 {
		mergeConfigs(merged, base)
	}
	if len(override) > 0 {
		mergeConfigs(merged, override)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mergeStepConfig(base, override map[string]map[string]any) map[string]map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	result := make(map[string]map[string]any, len(base)+len(override))
	for key, val := range base {
		if val == nil {
			continue
		}
		result[key] = copyAnyMap(val)
	}

	for key, val := range override {
		if val == nil {
			continue
		}
		if existing, ok := result[key]; ok {
			result[key] = mergeConfigs(existing, val)
			continue
		}
		result[key] = copyAnyMap(val)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func copyAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dup := make(map[string]any, len(src))
	for key, val := range src {
		dup[key] = val
	}
	return dup
}

func decodeDocument(data map[string]any) (*Document, error) {
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
