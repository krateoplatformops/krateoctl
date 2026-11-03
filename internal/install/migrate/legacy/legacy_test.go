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

func TestFilterMeaninglessKeys(t *testing.T) {
	legacyYAML := `
apiVersion: krateo.io/v1alpha1
kind: KrateoPlatformOps
spec:
  steps:
  - id: install-chart
    type: chart
    with:
      name: example
      repository: https://charts.example.io
      set:
      - name: image.repository
        value: ghcr.io/example/image
      - name: map[]
      - name: service.port
        value: "8080"
      version: 1.0.0
`

	var obj map[string]any
	if err := yaml.Unmarshal([]byte(legacyYAML), &obj); err != nil {
		t.Fatalf("unmarshal legacy yaml: %v", err)
	}

	doc, err := ConvertDocument(obj, "default")
	if err != nil {
		t.Fatalf("ConvertDocument() error = %v", err)
	}

	if len(doc.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(doc.Steps))
	}

	chart := doc.Steps[0]
	values, ok := chart.With["values"].(map[string]any)
	if !ok {
		t.Fatalf("expected values map")
	}

	// Verify that map[] was filtered out
	if _, hasMapKey := values["map[]"]; hasMapKey {
		t.Fatalf("meaningless 'map[]' key should have been filtered out, got values: %v", values)
	}

	// Verify that valid keys are still present
	image, _ := values["image"].(map[string]any)
	if image["repository"].(string) != "ghcr.io/example/image" {
		t.Fatalf("valid key was incorrectly filtered: %v", image)
	}

	service, _ := values["service"].(map[string]any)
	if service["port"].(string) != "8080" {
		t.Fatalf("valid key was incorrectly filtered: %v", service)
	}
}

func TestConvertStringBooleanValues(t *testing.T) {
	legacyYAML := `
apiVersion: krateo.io/v1alpha1
kind: KrateoPlatformOps
spec:
  steps:
  - id: install-chart
    type: chart
    with:
      name: myuserchart
      repository: https://charts.example.com
      version: 1.0.0
      set:
      - name: enabled
        value: "true"
      - name: debug
        value: "false"
      - name: imagePullPolicy
        value: IfNotPresent
`

	var obj map[string]any
	if err := yaml.Unmarshal([]byte(legacyYAML), &obj); err != nil {
		t.Fatalf("unmarshal legacy yaml: %v", err)
	}

	doc, err := ConvertDocument(obj, "krateo-system")
	if err != nil {
		t.Fatalf("ConvertDocument() error = %v", err)
	}

	if len(doc.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(doc.Steps))
	}

	step := doc.Steps[0]
	values, ok := step.With["values"].(map[string]any)
	if !ok {
		t.Fatalf("expected values to be a map, got %T", step.With["values"])
	}

	// Check that string "true" was converted to boolean true
	enabled, ok := values["enabled"].(bool)
	if !ok || !enabled {
		t.Errorf("'true' string should be converted to boolean true, got %T: %v", values["enabled"], values["enabled"])
	}

	// Check that string "false" was converted to boolean false
	debug, ok := values["debug"].(bool)
	if !ok || debug {
		t.Errorf("'false' string should be converted to boolean false, got %T: %v", values["debug"], values["debug"])
	}

	// Check that non-boolean strings remain as strings
	policy, ok := values["imagePullPolicy"].(string)
	if !ok || policy != "IfNotPresent" {
		t.Errorf("non-boolean string should remain as string, got %T: %v", values["imagePullPolicy"], values["imagePullPolicy"])
	}
}

func TestConvertSetArraySyntaxToLists(t *testing.T) {
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
      - name: ingress.hosts[0].host
        value: authn.krateoplatformops.io
      - name: ingress.hosts[0].paths[0].path
        value: /
      - name: ingress.hosts[0].paths[0].pathType
        value: Prefix
      - name: service.ports[0].port
        value: "80"
      version: 0.1.0
`

	var obj map[string]any
	if err := yaml.Unmarshal([]byte(legacyYAML), &obj); err != nil {
		t.Fatalf("unmarshal legacy yaml: %v", err)
	}

	doc, err := ConvertDocument(obj, "krateo-system")
	if err != nil {
		t.Fatalf("ConvertDocument() error = %v", err)
	}

	values, ok := doc.Steps[0].With["values"].(map[string]any)
	if !ok {
		t.Fatalf("expected values map")
	}

	ingress, ok := values["ingress"].(map[string]any)
	if !ok {
		t.Fatalf("expected ingress map, got %T", values["ingress"])
	}

	hosts, ok := ingress["hosts"].([]any)
	if !ok || len(hosts) != 1 {
		t.Fatalf("expected one ingress host entry, got %T: %#v", ingress["hosts"], ingress["hosts"])
	}

	host0, ok := hosts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected host entry to be a map, got %T", hosts[0])
	}

	if got := host0["host"]; got != "authn.krateoplatformops.io" {
		t.Fatalf("unexpected host value: %v", got)
	}

	paths, ok := host0["paths"].([]any)
	if !ok || len(paths) != 1 {
		t.Fatalf("expected one path entry, got %T: %#v", host0["paths"], host0["paths"])
	}

	path0, ok := paths[0].(map[string]any)
	if !ok {
		t.Fatalf("expected path entry to be a map, got %T", paths[0])
	}

	if got := path0["path"]; got != "/" {
		t.Fatalf("unexpected path value: %v", got)
	}
	if got := path0["pathType"]; got != "Prefix" {
		t.Fatalf("unexpected pathType value: %v", got)
	}

	service, ok := values["service"].(map[string]any)
	if !ok {
		t.Fatalf("expected service map, got %T", values["service"])
	}

	ports, ok := service["ports"].([]any)
	if !ok || len(ports) != 1 {
		t.Fatalf("expected one service port entry, got %T: %#v", service["ports"], service["ports"])
	}

	port0, ok := ports[0].(map[string]any)
	if !ok {
		t.Fatalf("expected port entry to be a map, got %T", ports[0])
	}

	if got := port0["port"]; got != "80" {
		t.Fatalf("unexpected port value: %v", got)
	}
}
