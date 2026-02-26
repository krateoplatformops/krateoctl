package legacy

import (
	"fmt"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ConvertDocument builds a krateoctl configuration document out of a legacy
// KrateoPlatformOps custom resource specification. The legacy object is
// expected to contain a spec.steps array that mirrors the old controller plan.
func ConvertDocument(obj map[string]any, defaultNamespace string) (*config.Document, error) {
	if obj == nil {
		return nil, fmt.Errorf("legacy resource is empty")
	}

	stepsRaw, found, err := unstructured.NestedSlice(obj, "spec", "steps")
	if err != nil {
		return nil, fmt.Errorf("read spec.steps: %w", err)
	}
	if !found || len(stepsRaw) == 0 {
		return nil, fmt.Errorf("legacy resource missing spec.steps")
	}

	converted := make([]config.StepDefinition, 0, len(stepsRaw))
	for idx, raw := range stepsRaw {
		stepMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("step %d is not an object", idx)
		}

		step, err := convertStep(stepMap, defaultNamespace)
		if err != nil {
			return nil, fmt.Errorf("convert step %d: %w", idx, err)
		}
		converted = append(converted, *step)
	}

	doc := &config.Document{
		Steps: converted,
	}

	return doc, nil
}

func convertStep(raw map[string]any, defaultNamespace string) (*config.StepDefinition, error) {
	id, _ := raw["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("step id is empty")
	}

	typStr, _ := raw["type"].(string)
	if typStr == "" {
		return nil, fmt.Errorf("step %s missing type", id)
	}
	stepType := types.StepType(typStr)
	switch stepType {
	case types.TypeChart, types.TypeObject, types.TypeVar:
	default:
		return nil, fmt.Errorf("step %s has unsupported type %q", id, typStr)
	}

	withMap, _ := raw["with"].(map[string]any)
	normalized, err := normalizeWith(stepType, withMap, defaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("step %s: %w", id, err)
	}

	return &config.StepDefinition{ID: id, Type: stepType, With: normalized}, nil
}

func normalizeWith(stepType types.StepType, src map[string]any, defaultNamespace string) (map[string]any, error) {
	if len(src) == 0 {
		return nil, nil
	}

	dst := cloneMap(src)

	var setMap map[string]any
	var err error

	if rawSet, ok := dst["set"]; ok {
		setMap, err = parseSet(rawSet)
		if err != nil {
			return nil, err
		}
		delete(dst, "set")
	}

	switch stepType {
	case types.TypeChart:
		normalizeChartFields(dst)
		if len(setMap) > 0 {
			values := ensureMap(dst, "values")
			mergeMaps(values, setMap)
		}
	case types.TypeObject:
		if defaultNamespace != "" {
			meta := ensureMap(dst, "metadata")
			if _, ok := meta["namespace"]; !ok {
				meta["namespace"] = defaultNamespace
			}
		}
		if len(setMap) > 0 {
			mergeMaps(dst, setMap)
		}
	default:
		// vars or other steps do not need additional processing.
	}

	if len(dst) == 0 {
		return nil, nil
	}

	return dst, nil
}

func normalizeChartFields(dst map[string]any) {
	if repoURL, ok := dst["repository"].(string); ok && repoURL != "" {
		dst["url"] = repoURL
		delete(dst, "repository")
	}

	if chartName, ok := dst["name"].(string); ok && chartName != "" {
		dst["repo"] = chartName
		if _, hasRelease := dst["releaseName"]; !hasRelease {
			dst["releaseName"] = chartName
		}
		delete(dst, "name")
	}
}

func parseSet(raw any) (map[string]any, error) {
	if raw == nil {
		return nil, nil
	}

	entries, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("set must be a list, got %T", raw)
	}

	result := make(map[string]any)
	for idx, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("set entry %d is not an object", idx)
		}

		key, _ := entryMap["name"].(string)
		if key == "" || !isValidKey(key) {
			continue
		}
		assignNested(result, key, entryMap["value"])
	}

	if len(result) == 0 {
		return nil, nil
	}

	return result, nil
}

// isValidKey checks if a key is valid for migration (not empty, not just brackets, etc.)
func isValidKey(key string) bool {
	// Skip keys that are just "map[]" or similar meaningless entries
	if key == "map[]" || key == "[]" {
		return false
	}
	// Skip keys that contain [] without a proper field name (e.g., just brackets)
	if strings.HasPrefix(key, "[]") || strings.HasSuffix(key, "[]") && !strings.Contains(strings.TrimSuffix(key, "[]"), ".") {
		return false
	}
	return true
}

func assignNested(target map[string]any, dottedKey string, value any) {
	parts := strings.Split(dottedKey, ".")
	current := target
	for i, part := range parts {
		if part == "" {
			continue
		}

		if i == len(parts)-1 {
			current[part] = value
			return
		}

		next, ok := current[part]
		if !ok {
			child := make(map[string]any)
			current[part] = child
			current = child
			continue
		}

		child, ok := next.(map[string]any)
		if !ok {
			child = make(map[string]any)
			current[part] = child
		}
		current = child
	}
}

func ensureMap(target map[string]any, key string) map[string]any {
	existing, ok := target[key]
	if ok {
		if existingMap, ok := existing.(map[string]any); ok {
			return existingMap
		}
	}

	created := make(map[string]any)
	target[key] = created
	return created
}

func mergeMaps(dst, src map[string]any) {
	if dst == nil || src == nil {
		return
	}

	for key, val := range src {
		if existing, ok := dst[key]; ok {
			existingMap, okExisting := existing.(map[string]any)
			valMap, okVal := val.(map[string]any)
			if okExisting && okVal {
				mergeMaps(existingMap, valMap)
				continue
			}
		}
		dst[key] = val
	}
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, val := range src {
		dst[key] = cloneValue(val)
	}
	return dst
}

func cloneValue(val any) any {
	switch typed := val.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = cloneValue(v)
		}
		return out
	default:
		return val
	}
}
