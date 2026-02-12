package engine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConfigMerger handles layered YAML configuration with strict precedence.
// Layer precedence (highest to lowest):
// 1. CLI Flags
// 2. User Overrides (krateo-overrides.yaml)
// 3. Release Profile
// 4. Hardcoded Defaults
type ConfigMerger struct {
	defaults  *yaml.Node
	profile   *yaml.Node
	overrides *yaml.Node
	cliFlags  *yaml.Node
}

// NewConfigMerger creates a new configuration merger.
func NewConfigMerger() *ConfigMerger {
	return &ConfigMerger{}
}

// LoadDefaults loads the hardcoded default configuration.
func (cm *ConfigMerger) LoadDefaults(data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("failed to load defaults: %w", err)
	}
	cm.defaults = &node
	return nil
}

// LoadProfile loads the release profile.
func (cm *ConfigMerger) LoadProfile(data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("failed to load profile: %w", err)
	}
	cm.profile = &node
	return nil
}

// LoadOverridesFile loads user overrides from krateo-overrides.yaml.
// Returns nil error if file doesn't exist (overrides are optional).
func (cm *ConfigMerger) LoadOverridesFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Overrides file is optional
			return nil
		}
		return fmt.Errorf("failed to read overrides file: %w", err)
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("failed to parse overrides file: %w", err)
	}
	cm.overrides = &node
	return nil
}

// SetCLIFlags sets the CLI flag overrides.
func (cm *ConfigMerger) SetCLIFlags(data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return fmt.Errorf("failed to parse CLI flags: %w", err)
	}
	cm.cliFlags = &node
	return nil
}

// Merge combines all configuration layers in precedence order.
// Returns the fully resolved configuration.
func (cm *ConfigMerger) Merge() (*yaml.Node, error) {
	result := cm.defaults

	// Apply layers in order of precedence
	layers := []*yaml.Node{cm.profile, cm.overrides, cm.cliFlags}
	for _, layer := range layers {
		if layer != nil {
			merged, err := mergeYAMLNodes(result, layer)
			if err != nil {
				return nil, err
			}
			result = merged
		}
	}

	return result, nil
}

// mergeYAMLNodes performs a deep merge of two YAML nodes.
// layer2 (higher precedence) overwrites layer1 values.
// For sequences: layer2 replaces layer1 entirely (atomic list strategy).
// For mappings: keys from both are preserved, layer2 wins on conflicts.
func mergeYAMLNodes(layer1, layer2 *yaml.Node) (*yaml.Node, error) {
	if layer2 == nil {
		return layer1, nil
	}
	if layer1 == nil {
		return layer2, nil
	}

	// For sequences, always replace (atomic list strategy prevents drift)
	if layer2.Kind == yaml.SequenceNode {
		return layer2, nil
	}

	// For mappings, deep merge
	if layer2.Kind == yaml.MappingNode && layer1.Kind == yaml.MappingNode {
		return mergeMapping(layer1, layer2)
	}

	// For scalars and other types, layer2 wins
	return layer2, nil
}

// mergeMapping merges two mapping nodes (objects).
func mergeMapping(layer1, layer2 *yaml.Node) (*yaml.Node, error) {
	result := &yaml.Node{
		Kind:    yaml.MappingNode,
		Content: make([]*yaml.Node, 0),
	}

	// Build map of layer1 keys for quick lookup
	layer1Keys := make(map[string]int) // key -> index in result
	for i := 0; i < len(layer1.Content); i += 2 {
		key := layer1.Content[i].Value
		layer1Keys[key] = len(result.Content)
		result.Content = append(result.Content, layer1.Content[i], layer1.Content[i+1])
	}

	// Merge layer2 keys
	for i := 0; i < len(layer2.Content); i += 2 {
		key := layer2.Content[i].Value
		value := layer2.Content[i+1]

		if idx, exists := layer1Keys[key]; exists {
			// Key exists in both layers
			if layer1.Content[idx+1].Kind == yaml.MappingNode && value.Kind == yaml.MappingNode {
				// Recursively merge nested mappings
				merged, err := mergeMapping(layer1.Content[idx+1], value)
				if err != nil {
					return nil, err
				}
				result.Content[idx+1] = merged
			} else {
				// Scalar or mismatch: layer2 wins
				result.Content[idx+1] = value
			}
		} else {
			// Key only in layer2: append it
			result.Content = append(result.Content, layer2.Content[i], value)
		}
	}

	return result, nil
}
