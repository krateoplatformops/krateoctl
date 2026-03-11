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

	// Warn if Components reference names that don't exist in ComponentsDefinition
	if len(v.config.doc.Components) > 0 && len(v.config.doc.ComponentsDefinition) > 0 {
		for componentName := range v.config.doc.Components {
			if _, exists := v.config.doc.ComponentsDefinition[componentName]; !exists {
				v.logWarning("component %q in 'components' is not defined in 'componentsDefinition'", componentName)
			}
		}
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
