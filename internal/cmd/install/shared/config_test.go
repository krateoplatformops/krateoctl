package shared

import "testing"

func TestBuildLoadResultAppliesNamespaceTemplate(t *testing.T) {
	result, err := BuildLoadResult(map[string]any{
		"componentsDefinition": map[string]any{
			"backend": map[string]any{
				"steps": []any{"extract-authn-lb-ip", "install-frontend"},
			},
		},
		"steps": []any{
			map[string]any{
				"id":   "extract-authn-lb-ip",
				"type": "var",
				"with": map[string]any{
					"name": "AUTHN_IP",
					"valueFrom": map[string]any{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata": map[string]any{
							"name":      "authn",
							"namespace": "{{ .Namespace }}",
						},
						"selector": ".status.loadBalancer.ingress[0].ip",
					},
				},
			},
			map[string]any{
				"id":   "install-frontend",
				"type": "chart",
				"with": map[string]any{
					"releaseName": "frontend",
					"namespace":   "{{ .Namespace }}",
					"values": map[string]any{
						"config": map[string]any{
							"DB_HOST": "pg-cluster-rw.{{ .Namespace }}.svc.cluster.local",
						},
					},
				},
			},
		},
	}, "demo-system", nil, false)
	if err != nil {
		t.Fatalf("BuildLoadResult() error = %v", err)
	}

	if got := (*result.Steps[0].With)["valueFrom"].(map[string]any)["metadata"].(map[string]any)["namespace"]; got != "demo-system" {
		t.Fatalf("var step namespace = %v, want demo-system", got)
	}

	if got := (*result.Steps[1].With)["namespace"]; got != "demo-system" {
		t.Fatalf("chart step namespace = %v, want demo-system", got)
	}

	values := (*result.Steps[1].With)["values"].(map[string]any)
	config := values["config"].(map[string]any)
	if got := config["DB_HOST"]; got != "pg-cluster-rw.demo-system.svc.cluster.local" {
		t.Fatalf("chart values host = %v, want pg-cluster-rw.demo-system.svc.cluster.local", got)
	}
}
