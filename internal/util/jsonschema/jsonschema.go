package jsonschema

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/krateoplatformops/plumbing/maps"
)

//go:embed assets/*.json
var extras embed.FS

const (
	apiRefKey                = "apiRef"
	widgetDataTemplateKey    = "widgetDataTemplate"
	resourcesRefsKey         = "resourcesRefs"
	resourcesRefsTemplateKey = "resourcesRefsTemplate"
)

func ExtractKindAndVersion(schema map[string]any) (kind, version string, err error) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		err = fmt.Errorf("missing 'properties' field")
		return
	}

	getDefault := func(key string) string {
		prop, ok := properties[key]
		if !ok {
			return ""
		}

		propMap, ok := prop.(map[string]any)
		if !ok {
			return ""
		}

		if def, ok := propMap["default"]; ok {
			if strVal, ok := def.(string); ok {
				return strVal
			}
		}

		return ""
	}

	kind = getDefault("kind")
	version = getDefault("version")
	if version == "" {
		apiVersion := getDefault("apiVersion")
		idx := strings.LastIndexByte(apiVersion, '/')
		if idx > 0 {
			version = apiVersion[idx+1:]
		}
	}
	if version == "" {
		version = "v1alpha1"
	}

	return
}

func ExtractSpec(in map[string]any) (out map[string]any, err error) {
	res, ok, err := maps.NestedMap(in, "properties", "spec")
	if err != nil {
		return map[string]any{}, err
	}
	if !ok {
		return map[string]any{}, fmt.Errorf("properties.spec not found in JSON schema")
	}

	err = insertExtras(fmt.Sprintf("%s.json", apiRefKey), res, "properties", apiRefKey)
	if err != nil {
		return map[string]any{}, err
	}

	err = insertExtras(fmt.Sprintf("%s.json", widgetDataTemplateKey), res, "properties", widgetDataTemplateKey)
	if err != nil {
		return map[string]any{}, err
	}

	err = insertExtras(fmt.Sprintf("%s.json", resourcesRefsKey), res, "properties", resourcesRefsKey)
	if err != nil {
		return map[string]any{}, err
	}

	err = insertExtras(fmt.Sprintf("%s.json", resourcesRefsTemplateKey), res, "properties", resourcesRefsTemplateKey)
	if err != nil {
		return map[string]any{}, err
	}

	if required, ok := res["required"].([]any); ok {
		var newRequired []any
		for _, v := range required {
			if str, ok := v.(string); ok && str != "kind" && str != "apiVersion" {
				newRequired = append(newRequired, v)
			}
		}
		res["required"] = newRequired
	}

	return res, nil
}

func insertExtras(filename string, into map[string]any, fields ...string) error {
	data, err := extras.ReadFile(fmt.Sprintf("assets/%s", filename))
	if err != nil {
		return err
	}

	tmp := map[string]any{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	return maps.SetNestedField(into, tmp, fields...)
}
