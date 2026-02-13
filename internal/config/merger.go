package config

import (
	"fmt"
	"strings"
)

// Merge applies CLI flags and functional shortcuts on top of the loaded config
type Merger struct {
	config *Config
}

// NewMerger creates a new merger for applying CLI overrides
func NewMerger(config *Config) *Merger {
	return &Merger{config: config}
}

// ApplyFlags applies CLI flag overrides only
// Supports both granular --set flags and shortcut flags like --openshift
func (m *Merger) ApplyFlags(flags map[string]interface{}) error {
	for key, value := range flags {
		// Handle shortcut flags
		switch key {
		case "openshift":
			if v, ok := value.(bool); ok && v {
				m.applyOpenShiftProfile()
			}
		case "no-finops":
			if v, ok := value.(bool); ok && v {
				m.disableModule("finops")
			}
		default:
			// Handle granular --set flags (e.g., "modules.frontend.enabled=false")
			if err := m.setPath(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyOpenShiftProfile injects OpenShift-specific configuration
func (m *Merger) applyOpenShiftProfile() {
	// Set route.enabled for OpenShift
	if err := m.config.Set([]string{"route", "enabled"}, true); err != nil {
		// Log but don't fail - profiles are optional
	}

	// Set security context for OpenShift
	if err := m.config.Set([]string{"securityContext", "enabled"}, true); err != nil {
		// Log but don't fail
	}
}

// disableModule disables a specific module by name
func (m *Merger) disableModule(name string) error {
	modules, err := m.config.GetModules()
	if err != nil {
		return err
	}

	if mod, ok := modules[name]; ok {
		if modMap, ok := mod.(map[string]interface{}); ok {
			modMap["enabled"] = false
		}
	}

	return nil
}

// setPath sets a value at a dot-separated key path (e.g., "modules.frontend.enabled")
func (m *Merger) setPath(path string, value interface{}) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return fmt.Errorf("invalid path: %s", path)
	}

	return m.config.Set(parts, value)
}
