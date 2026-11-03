package shared

import (
	"fmt"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
)

const (
	DefaultConfigPath    = "krateo.yaml"
	DefaultOverridesPath = "krateo-overrides.yaml"
	DefaultNamespace     = "krateo-system"

	KRATEOCTL_DEBUG_ENV = "KRATEOCTL_DEBUG"
)

// LoadResult contains the validated configuration and the resolved workflow steps.
type LoadResult struct {
	Config        *config.Config
	Steps         []*types.Step
	OriginalSteps []*types.Step
}

// LoadConfigAndSteps loads the Krateo configuration, validates it (unless skipped), and resolves the active steps.
// The optional logger is used to display validation warnings.
func LoadConfigAndSteps(opts config.LoadOptions, namespace string, logger func(string, ...any), skipValidation bool) (*LoadResult, error) {
	if opts.Namespace == "" {
		opts.Namespace = namespace
	}

	loader := config.NewLoader(opts)

	data, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("Failed to load configuration: %w", err)
	}

	return BuildLoadResult(data, namespace, logger, skipValidation)
}

// BuildLoadResult validates raw configuration data and resolves the active steps.
func BuildLoadResult(data map[string]any, namespace string, logger func(string, ...any), skipValidation bool) (*LoadResult, error) {
	applyNamespaceTemplate(data, namespace)

	cfg, err := config.NewConfig(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to build configuration: %w", err)
	}

	if !skipValidation {
		validator := config.NewValidator(cfg)
		if logger != nil {
			validator.WithLogger(logger)
		}
		if err := validator.Validate(); err != nil {
			return nil, fmt.Errorf("Configuration validation failed: %w", err)
		}
	}

	steps, err := cfg.GetActiveSteps()
	if err != nil {
		return nil, fmt.Errorf("Failed to get steps: %w", err)
	}

	originalSteps, err := cfg.GetSteps()
	if err != nil {
		return nil, fmt.Errorf("Failed to get original steps: %w", err)
	}

	return &LoadResult{
		Config:        cfg,
		Steps:         steps,
		OriginalSteps: originalSteps,
	}, nil
}

func applyNamespaceTemplate(value any, namespace string) any {
	if namespace == "" {
		return value
	}

	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = applyNamespaceTemplate(item, namespace)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = applyNamespaceTemplate(item, namespace)
		}
		return v
	case string:
		return replaceNamespaceTemplateTokens(v, namespace)
	default:
		return value
	}
}

func replaceNamespaceTemplateTokens(value, namespace string) string {
	replaced := strings.ReplaceAll(value, "{{ .Namespace }}", namespace)
	replaced = strings.ReplaceAll(replaced, "{{.Namespace}}", namespace)
	replaced = strings.ReplaceAll(replaced, "{{ .Namespace}}", namespace)
	replaced = strings.ReplaceAll(replaced, "{{.Namespace }}", namespace)
	return replaced
}
