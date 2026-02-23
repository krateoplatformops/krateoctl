package config

import "github.com/krateoplatformops/krateoctl/internal/workflows/types"

// Document represents the structured krateo.yaml configuration.
type Document struct {
	Global     map[string]interface{}     `json:"global,omitempty" yaml:"global,omitempty"`
	Modules    map[string]ModuleConfig    `json:"modules,omitempty" yaml:"modules,omitempty"`
	Components map[string]ComponentConfig `json:"components,omitempty" yaml:"components,omitempty"`
	Steps      []StepDefinition           `json:"steps,omitempty" yaml:"steps,omitempty"`
}

// ModuleConfig describes a single module entry in the configuration.
type ModuleConfig struct {
	Enabled *bool        `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Chart   *ModuleChart `json:"chart,omitempty" yaml:"chart,omitempty"`
}

// ModuleChart contains the chart coordinates for a module.
type ModuleChart struct {
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
	URL        string `json:"url,omitempty" yaml:"url,omitempty"`
	Name       string `json:"name,omitempty" yaml:"name,omitempty"`
	Chart      string `json:"chart,omitempty" yaml:"chart,omitempty"`
	Namespace  string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}

// ComponentConfig captures the metadata and overrides for a logical component.
type ComponentConfig struct {
	Description  string                            `json:"description,omitempty" yaml:"description,omitempty"`
	Enabled      *bool                             `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Steps        []string                          `json:"steps,omitempty" yaml:"steps,omitempty"`
	HelmDefaults map[string]interface{}            `json:"helmDefaults,omitempty" yaml:"helmDefaults,omitempty"`
	StepConfig   map[string]map[string]interface{} `json:"stepConfig,omitempty" yaml:"stepConfig,omitempty"`
}

// StepDefinition represents a single workflow step as defined in krateo.yaml.
type StepDefinition struct {
	ID   string                 `json:"id" yaml:"id"`
	Type types.StepType         `json:"type" yaml:"type"`
	With map[string]interface{} `json:"with,omitempty" yaml:"with,omitempty"`
}
