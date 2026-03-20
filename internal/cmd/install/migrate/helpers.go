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

func writeOutputFile(outputPath string, force bool, writeFile fileWriter, data []byte) error {
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("output file %s already exists (use --force to overwrite)", outputPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return writeFile(outputPath, data, 0o644)
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

	doc.ComponentsDefinition = components
	return nil
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
