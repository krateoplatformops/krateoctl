package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadOptions configures how configuration is loaded.
type LoadOptions struct {
	// ConfigPath is the path to the main krateo.yaml
	ConfigPath string
	// UserOverridesPath is the path to krateo-overrides.yaml (optional)
	UserOverridesPath string
}

// Loader handles loading configuration from files.
type Loader struct {
	opts LoadOptions
}

// NewLoader creates a new configuration loader.
func NewLoader(opts LoadOptions) *Loader {
	return &Loader{opts: opts}
}

// Load reads and parses configuration from krateo.yaml and optional overrides.
// Returns a map[string]interface{} representing the merged configuration.
func (l *Loader) Load() (map[string]interface{}, error) {
	// Load main config file
	config, err := l.loadFile(l.opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", l.opts.ConfigPath, err)
	}

	// Load and merge user overrides if file exists
	if l.opts.UserOverridesPath != "" {
		if _, err := os.Stat(l.opts.UserOverridesPath); err == nil {
			overrides, err := l.loadFile(l.opts.UserOverridesPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load overrides from %s: %w", l.opts.UserOverridesPath, err)
			}
			config = mergeConfigs(config, overrides)
		}
	}

	return config, nil
}

// loadFile reads and parses a YAML file into a map.
func (l *Loader) loadFile(path string) (map[string]interface{}, error) {
	if path == "" {
		return make(map[string]interface{}), nil
	}

	// Resolve relative paths
	if !filepath.IsAbs(path) {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %s: %w", path, err)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse YAML from %s: %w", path, err)
	}

	return data, nil
}

// mergeConfigs recursively merges override config into base config.
// Arrays are replaced (atomic strategy), objects are merged recursively.
func mergeConfigs(base, override map[string]interface{}) map[string]interface{} {
	for key, val := range override {
		if baseVal, exists := base[key]; exists {
			// Both are maps - merge recursively
			if baseMap, ok := baseVal.(map[string]interface{}); ok {
				if overrideMap, ok := val.(map[string]interface{}); ok {
					base[key] = mergeConfigs(baseMap, overrideMap)
					continue
				}
			}
		}
		// Replace for scalars, arrays, or type mismatches
		base[key] = val
	}
	return base
}
