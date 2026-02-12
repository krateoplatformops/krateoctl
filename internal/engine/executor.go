package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	defaultChartTimeout = 5 * time.Minute
)

// Executor builds WorkflowSpec from configuration.
type Executor struct {
	namespace string
}

// NewExecutor creates a new workflow executor.
func NewExecutor(namespace string) *Executor {
	if namespace == "" {
		namespace = "default"
	}
	return &Executor{namespace: namespace}
}

// BuildWorkflowSpec builds a WorkflowSpec from module configuration.
// The module config should have:
//   - "chart": {...} for Helm charts
//   - "objects": [{...}] for K8s objects
//   - "vars": [{...}] for variable exports
func (e *Executor) BuildWorkflowSpec(moduleName string, moduleConfig map[string]interface{}) (*types.WorkflowSpec, error) {
	spec := &types.WorkflowSpec{
		Steps: make([]*types.Step, 0),
	}

	// 1. Add chart step if chart config exists
	if chartCfg, ok := moduleConfig["chart"]; ok {
		if chartMap, ok := chartCfg.(map[string]interface{}); ok {
			step, err := e.buildChartStep(moduleName, chartMap)
			if err != nil {
				return nil, fmt.Errorf("failed to build chart step for module %s: %w", moduleName, err)
			}
			spec.Steps = append(spec.Steps, step)
		}
	}

	// 2. Add object steps if objects exist
	if objsCfg, ok := moduleConfig["objects"]; ok {
		if objsList, ok := objsCfg.([]interface{}); ok {
			steps, err := e.buildObjectSteps(moduleName, objsList)
			if err != nil {
				return nil, fmt.Errorf("failed to build object steps for module %s: %w", moduleName, err)
			}
			spec.Steps = append(spec.Steps, steps...)
		}
	}

	// 3. Add var steps if vars exist
	if varsCfg, ok := moduleConfig["vars"]; ok {
		if varsList, ok := varsCfg.([]interface{}); ok {
			steps, err := e.buildVarSteps(moduleName, varsList)
			if err != nil {
				return nil, fmt.Errorf("failed to build var steps for module %s: %w", moduleName, err)
			}
			spec.Steps = append(spec.Steps, steps...)
		}
	}

	return spec, nil
}

// buildChartStep creates a chart installation step from Helm config.
func (e *Executor) buildChartStep(moduleName string, chartCfg map[string]interface{}) (*types.Step, error) {
	// Extract chart details from config
	repo, _ := chartCfg["repository"].(string)
	chart, _ := chartCfg["chart"].(string)
	if chart == "" {
		chart, _ = chartCfg["name"].(string)
	}
	version, _ := chartCfg["version"].(string)
	namespace, _ := chartCfg["namespace"].(string)
	if namespace == "" {
		namespace = e.namespace
	}

	// Extract values
	var values map[string]any
	if valsCfg, ok := chartCfg["values"]; ok {
		values, _ = valsCfg.(map[string]interface{})
	}
	if values == nil {
		values = make(map[string]any)
	}

	values["namespace"] = namespace

	// Build ChartSpec
	chartSpec := types.ChartSpec{
		Repository:  repo,
		Name:        chart,
		Version:     version,
		ReleaseName: moduleName,
		Values:      values,
		Wait:        true,
		Timeout:     defaultChartTimeout,
	}

	// Convert to RawExtension
	data, err := json.Marshal(chartSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chart spec: %w", err)
	}

	return &types.Step{
		ID:   fmt.Sprintf("%s-chart", moduleName),
		Type: types.TypeChart,
		With: &runtime.RawExtension{Raw: data},
	}, nil
}

// buildObjectSteps creates K8s object creation steps.
func (e *Executor) buildObjectSteps(moduleName string, objsList []interface{}) ([]*types.Step, error) {
	steps := make([]*types.Step, 0, len(objsList))

	for i, objCfg := range objsList {
		objMap, ok := objCfg.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("object at index %d is not a mapping", i)
		}

		// Extract object metadata
		apiVersion, _ := objMap["apiVersion"].(string)
		kind, _ := objMap["kind"].(string)
		if apiVersion == "" || kind == "" {
			return nil, fmt.Errorf("object at index %d missing apiVersion or kind", i)
		}

		// Extract metadata
		var metaRef struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		}

		if metaCfg, ok := objMap["metadata"]; ok {
			if metaMap, ok := metaCfg.(map[string]interface{}); ok {
				if name, ok := metaMap["name"].(string); ok {
					metaRef.Name = name
				}
				if ns, ok := metaMap["namespace"].(string); ok {
					metaRef.Namespace = ns
				} else {
					metaRef.Namespace = e.namespace
				}
			}
		}

		if metaRef.Namespace == "" {
			metaRef.Namespace = e.namespace
		}

		// Ensure metadata.namespace is set
		if metaCfg, ok := objMap["metadata"]; ok {
			if metaMap, ok := metaCfg.(map[string]interface{}); ok {
				metaMap["namespace"] = metaRef.Namespace
			}
		} else {
			objMap["metadata"] = map[string]interface{}{
				"name":      metaRef.Name,
				"namespace": metaRef.Namespace,
			}
		}

		// Convert to RawExtension
		data, err := json.Marshal(objMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal object: %w", err)
		}

		step := &types.Step{
			ID:   fmt.Sprintf("%s-obj-%d", moduleName, i),
			Type: types.TypeObject,
			With: &runtime.RawExtension{Raw: data},
		}

		steps = append(steps, step)
	}

	return steps, nil
}

// buildVarSteps creates variable export steps.
func (e *Executor) buildVarSteps(moduleName string, varsList []interface{}) ([]*types.Step, error) {
	steps := make([]*types.Step, 0, len(varsList))

	for i, varCfg := range varsList {
		varMap, ok := varCfg.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("var at index %d is not a mapping", i)
		}

		// Build Var
		var varData types.Var

		// Extract name
		if name, ok := varMap["name"].(string); ok {
			varData.Data.Name = name
		} else {
			return nil, fmt.Errorf("var at index %d missing name", i)
		}

		// Extract value
		if value, ok := varMap["value"].(string); ok {
			varData.Data.Value = value
		}

		// Extract asString
		if asStr, ok := varMap["asString"].(bool); ok {
			varData.Data.AsString = &asStr
		}

		// Convert to RawExtension
		data, err := json.Marshal(varData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal var: %w", err)
		}

		step := &types.Step{
			ID:   fmt.Sprintf("%s-var-%d", moduleName, i),
			Type: types.TypeVar,
			With: &runtime.RawExtension{Raw: data},
		}

		steps = append(steps, step)
	}

	return steps, nil
}
