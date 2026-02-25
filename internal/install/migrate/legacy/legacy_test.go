package legacy

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestConvertDocument(t *testing.T) {
	legacyYAML := `
apiVersion: krateo.io/v1alpha1
kind: KrateoPlatformOps
spec:
  steps:
  - id: install-authn
    type: chart
    with:
      name: authn
      repository: https://charts.krateo.io
      set:
      - name: image.repository
        value: ghcr.io/legacy/authn
      - name: service.nodePort
        value: "30082"
      wait: true
      version: 0.1.0
  - id: create-secret
    type: object
    with:
      apiVersion: v1
      kind: Secret
      metadata:
        name: test-secret
      set:
      - name: type
        value: Opaque
      - name: stringData.password
        value: changeme
`

	var obj map[string]any
	if err := yaml.Unmarshal([]byte(legacyYAML), &obj); err != nil {
		t.Fatalf("unmarshal legacy yaml: %v", err)
	}

	doc, err := ConvertDocument(obj, "krateo-system")
	if err != nil {
		t.Fatalf("ConvertDocument() error = %v", err)
	}

	if len(doc.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(doc.Steps))
	}

	chart := doc.Steps[0]
	if chart.ID != "install-authn" {
		t.Fatalf("unexpected chart id %s", chart.ID)
	}

	repo, _ := chart.With["repo"].(string)
	if repo != "authn" {
		t.Fatalf("expected repo authn, got %s", repo)
	}

	values, _ := chart.With["values"].(map[string]any)
	image, _ := values["image"].(map[string]any)
	if image["repository"].(string) != "ghcr.io/legacy/authn" {
		t.Fatalf("unexpected image repository %v", image["repository"])
	}

	service, _ := values["service"].(map[string]any)
	if service["nodePort"].(string) != "30082" {
		t.Fatalf("unexpected nodePort %v", service["nodePort"])
	}

	objectStep := doc.Steps[1]
	metadata, _ := objectStep.With["metadata"].(map[string]any)
	if metadata["namespace"].(string) != "krateo-system" {
		t.Fatalf("object namespace mismatch: %v", metadata["namespace"])
	}

	stringData, _ := objectStep.With["stringData"].(map[string]any)
	if stringData["password"].(string) != "changeme" {
		t.Fatalf("stringData not converted: %v", stringData)
	}
}
