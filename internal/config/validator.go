package config

import (
	"fmt"
)

// Validator performs validation on the configuration.
type Validator struct {
	config *Config
	logger func(string, ...any) // optional logger for warnings
}

// NewValidator creates a new configuration validator.
func NewValidator(config *Config) *Validator {
	return &Validator{config: config}
}

// WithLogger sets an optional logger for warnings.
func (v *Validator) WithLogger(logger func(string, ...any)) *Validator {
	v.logger = logger
	return v
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

	// Validate component-step relationships
	if err := v.validateComponentSteps(); err != nil {
		return err
	}

	// Validate that StepConfig keys reference valid steps
	if err := v.validateStepConfigReferences(); err != nil {
		return err
	}

	// Validate that all steps are referenced in at least one component (no orphaned steps)
	if err := v.validateOrphanedSteps(); err != nil {
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

// validateComponentSteps validates that steps referenced in components exist
// and that component names in the Components section are actually defined.
// Returns all invalid references in a single error for consistency.
func (v *Validator) validateComponentSteps() error {
	if v.config.doc == nil {
		return nil
	}

	// Build a map of all defined step IDs for quick lookup
	stepIDMap := make(map[string]bool)
	for _, step := range v.config.doc.Steps {
		stepIDMap[step.ID] = true
	}

	// Get all component definitions (merged from ComponentsDefinition and Components)
	definitions := v.config.componentDefinitions()
	if len(definitions) == 0 {
		return nil // No components, nothing to validate
	}

	// Collect all invalid step references
	type invalidRef struct {
		component string
		step      string
	}
	var invalidRefs []invalidRef

	// Check that all steps referenced in components actually exist
	for componentName, comp := range definitions {
		for _, stepID := range comp.Steps {
			if !stepIDMap[stepID] {
				invalidRefs = append(invalidRefs, invalidRef{component: componentName, step: stepID})
			}
		}
	}

	// Return error with all invalid references
	if len(invalidRefs) > 0 {
		if len(invalidRefs) == 1 {
			ref := invalidRefs[0]
			return fmt.Errorf("component %q references step %q which does not exist in the steps list", ref.component, ref.step)
		}

		errMsg := "the following component-step references are invalid:\n"
		for _, ref := range invalidRefs {
			errMsg += fmt.Sprintf("  - component %q references non-existent step %q\n", ref.component, ref.step)
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Error if Components reference names that don't exist in ComponentsDefinition
	// This can happen when a profile overrides a component that doesn't exist in the base config
	if len(v.config.doc.Components) > 0 && v.config.doc.ComponentsDefinition != nil && len(v.config.doc.ComponentsDefinition) > 0 {
		var invalidComponents []string
		for componentName := range v.config.doc.Components {
			if _, exists := v.config.doc.ComponentsDefinition[componentName]; !exists {
				invalidComponents = append(invalidComponents, componentName)
			}
		}
		if len(invalidComponents) > 0 {
			if len(invalidComponents) == 1 {
				return fmt.Errorf("component %q in overrides is not defined in 'componentsDefinition'", invalidComponents[0])
			}
			errMsg := "the following components in overrides are not defined in 'componentsDefinition':\n"
			for _, comp := range invalidComponents {
				errMsg += fmt.Sprintf("  - %s\n", comp)
			}
			return fmt.Errorf("%s", errMsg)
		}
	}

	return nil
}

// validateStepConfigReferences validates that all keys in StepConfig reference actual steps
// that belong to the component. Returns an error if any step config keys don't correspond
// to steps defined in the component's Steps array.
func (v *Validator) validateStepConfigReferences() error {
	if v.config.doc == nil {
		return nil
	}

	// Build a map of all defined step IDs for quick lookup
	stepIDMap := make(map[string]bool)
	for _, step := range v.config.doc.Steps {
		stepIDMap[step.ID] = true
	}

	// Check ComponentsDefinition
	definitions := v.config.doc.ComponentsDefinition
	if len(definitions) == 0 {
		definitions = make(map[string]ComponentConfig)
	}

	// Also check Components (overrides) - merge with definitions for complete picture
	overrides := v.config.doc.Components
	if len(overrides) == 0 {
		overrides = make(map[string]ComponentConfig)
	}

	// Build combined set of all components with their step configs
	allComponents := make(map[string]ComponentConfig)
	for name, comp := range definitions {
		allComponents[name] = comp
	}
	for name, overrideComp := range overrides {
		if existingComp, exists := allComponents[name]; exists {
			// Merge the override with the definition
			// If override has stepConfig keys, we need to validate them against the component's steps
			merged := existingComp
			if len(overrideComp.StepConfig) > 0 {
				if merged.StepConfig == nil {
					merged.StepConfig = make(map[string]map[string]interface{})
				}
				for k, v := range overrideComp.StepConfig {
					merged.StepConfig[k] = v
				}
			}
			allComponents[name] = merged
		} else {
			allComponents[name] = overrideComp
		}
	}

	// Track invalid step config references
	type invalidStepConfig struct {
		component  string
		stepConfig string
		reason     string
	}
	var invalidConfigs []invalidStepConfig

	// Check each component's StepConfig
	for componentName, comp := range allComponents {
		if len(comp.StepConfig) == 0 {
			continue
		}

		// Build set of actual steps in this component
		componentSteps := make(map[string]bool)
		for _, stepID := range comp.Steps {
			componentSteps[stepID] = true
		}

		// Validate each step config key
		for stepConfigKey := range comp.StepConfig {
			// Check if the step config key references a step that exists
			if !stepIDMap[stepConfigKey] {
				invalidConfigs = append(invalidConfigs, invalidStepConfig{
					component:  componentName,
					stepConfig: stepConfigKey,
					reason:     "does not exist in the steps list",
				})
			} else if !componentSteps[stepConfigKey] {
				// Step exists globally but not in this component
				invalidConfigs = append(invalidConfigs, invalidStepConfig{
					component:  componentName,
					stepConfig: stepConfigKey,
					reason:     "is not a step of this component",
				})
			}
		}
	}

	// Return error with all invalid references
	if len(invalidConfigs) > 0 {
		if len(invalidConfigs) == 1 {
			ref := invalidConfigs[0]
			return fmt.Errorf("component %q has invalid stepConfig key %q which %s", ref.component, ref.stepConfig, ref.reason)
		}

		errMsg := "the following stepConfig references are invalid:\n"
		for _, ref := range invalidConfigs {
			errMsg += fmt.Sprintf("  - component %q: stepConfig key %q %s\n", ref.component, ref.stepConfig, ref.reason)
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// validateOrphanedSteps ensures that all steps are referenced in at least one component.
// Returns an error if any steps are orphaned (not referenced in any component).
func (v *Validator) validateOrphanedSteps() error {
	if v.config.doc == nil || len(v.config.doc.Steps) == 0 {
		return nil
	}

	definitions := v.config.componentDefinitions()
	if len(definitions) == 0 {
		// No components defined, all steps are orphaned
		var orphanedSteps []string
		for _, step := range v.config.doc.Steps {
			orphanedSteps = append(orphanedSteps, step.ID)
		}

		if len(orphanedSteps) == 1 {
			return fmt.Errorf("no components defined - step %q has no component assignment", orphanedSteps[0])
		}

		errMsg := "no components defined - the following steps have no component assignment:\n"
		for _, stepID := range orphanedSteps {
			errMsg += fmt.Sprintf("  - %s\n", stepID)
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Build a map of steps referenced in components
	referencedSteps := make(map[string]bool)
	for _, comp := range definitions {
		for _, stepID := range comp.Steps {
			referencedSteps[stepID] = true
		}
	}

	// Find orphaned steps
	var orphanedSteps []string
	for _, step := range v.config.doc.Steps {
		if !referencedSteps[step.ID] {
			orphanedSteps = append(orphanedSteps, step.ID)
		}
	}

	if len(orphanedSteps) > 0 {
		if len(orphanedSteps) == 1 {
			return fmt.Errorf("step %q is not referenced in any component and cannot be executed", orphanedSteps[0])
		}

		errMsg := "the following steps are not referenced in any component and cannot be executed:\n"
		for _, stepID := range orphanedSteps {
			errMsg += fmt.Sprintf("  - %s\n", stepID)
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// logWarning logs a warning message if a logger is available.
func (v *Validator) logWarning(msg string, args ...any) {
	if v.logger != nil {
		v.logger("⚠ "+msg, args...)
	}
}
