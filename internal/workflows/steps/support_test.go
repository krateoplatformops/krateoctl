package steps

import (
	"fmt"
	"testing"
)

func TestStrval(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "bytes",
			input:    []byte("world"),
			expected: "world",
		},
		{
			name:     "error",
			input:    fmt.Errorf("test error"),
			expected: "test error",
		},
		{
			name:     "integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "nil",
			input:    nil,
			expected: "<nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Strval(tt.input)
			if result != tt.expected {
				t.Errorf("Strval() got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDeriveReleaseName(t *testing.T) {
	tests := []struct {
		name     string
		repoUrl  string
		expected string
	}{
		{
			name:     "krateo-bff chart",
			repoUrl:  "https://raw.githubusercontent.com/matteogastaldello/private-charts/main/krateo-bff-0.18.1.tgz",
			expected: "krateo-bff",
		},
		{
			name:     "simple chart with version",
			repoUrl:  "mychart-1.2.3.tgz",
			expected: "mychart",
		},
		{
			name:     "chart without version",
			repoUrl:  "mychart.tgz",
			expected: "mychart",
		},
		{
			name:     "chart with multiple dashes",
			repoUrl:  "my-cool-chart-2.0.0.tgz",
			expected: "my-cool-chart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveReleaseName(tt.repoUrl)
			if result != tt.expected {
				t.Errorf("DeriveReleaseName() got %q, want %q", result, tt.expected)
			}
		})
	}
}
