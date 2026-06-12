package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/krateoplatformops/krateoctl/internal/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

func fetchLegacyResource(ctx context.Context, dyn dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	var lastErr error
	for _, gvr := range legacyGVRs {
		obj, err := dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		switch {
		case err == nil:
			return obj, nil
		case apierrors.IsNotFound(err):
			lastErr = err
			continue
		case meta.IsNoMatchError(err):
			lastErr = err
			continue
		default:
			return nil, err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("KrateoPlatformOps %s/%s not found", namespace, name)
	}
	return nil, lastErr
}

type writeOutputOptions struct {
	outputPath string
	force      bool
	writeFile  fileWriter
	data       []byte
}

func writeOutputFile(opts writeOutputOptions) error {
	if !opts.force {
		if _, err := os.Stat(opts.outputPath); err == nil {
			return fmt.Errorf("output file %s already exists (use --force to overwrite)", opts.outputPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	dir := filepath.Dir(opts.outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	data := opts.data
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return opts.writeFile(opts.outputPath, data, 0o644)
}

func applyDefaultComponents(doc *config.Document, installType string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	var componentData []byte
	switch installType {
	case "loadbalancer":
		componentData = componentsDefinitionLoadbalancerYAML
	case "ingress":
		componentData = componentsDefinitionIngressYAML
	case "nodeport", "":
		componentData = componentsDefinitionYAML
	default:
		return fmt.Errorf("unknown installation type: %s (expected: nodeport, loadbalancer, or ingress)", installType)
	}

	components, err := loadComponentsDefinition(componentData)
	if err != nil {
		return err
	}

	doc.ComponentsDefinition = filterComponentsDefinition(components, doc.Steps)
	return nil
}

func filterComponentsDefinition(components map[string]config.ComponentConfig, steps []config.StepDefinition) map[string]config.ComponentConfig {
	if len(components) == 0 {
		return nil
	}

	stepIDs := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		stepIDs[step.ID] = struct{}{}
	}

	filtered := make(map[string]config.ComponentConfig, len(components))
	for name, component := range components {
		keptSteps := make([]string, 0, len(component.Steps))
		for _, stepID := range component.Steps {
			if _, ok := stepIDs[stepID]; ok {
				keptSteps = append(keptSteps, stepID)
			}
		}

		if len(keptSteps) == 0 {
			continue
		}

		filteredComponent := component
		filteredComponent.Steps = keptSteps

		if len(component.StepConfig) > 0 {
			stepConfig := make(map[string]map[string]interface{}, len(component.StepConfig))
			for stepID, configValues := range component.StepConfig {
				if _, ok := stepIDs[stepID]; !ok {
					continue
				}

				stepConfig[stepID] = configValues
			}
			if len(stepConfig) > 0 {
				filteredComponent.StepConfig = stepConfig
			} else {
				filteredComponent.StepConfig = nil
			}
		}

		filtered[name] = filteredComponent
	}

	if len(filtered) == 0 {
		return nil
	}

	return filtered
}

func loadComponentsDefinition(data []byte) (map[string]config.ComponentConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("components definition asset is empty")
	}

	var payload struct {
		ComponentsDefinition map[string]config.ComponentConfig `json:"componentsDefinition" yaml:"componentsDefinition"`
	}

	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse components definition: %w", err)
	}

	if len(payload.ComponentsDefinition) == 0 {
		return nil, fmt.Errorf("components definition asset missing entries")
	}

	return payload.ComponentsDefinition, nil
}
