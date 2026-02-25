package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const (
	// DefaultInstallationName is the name used for the installation snapshot resource.
	DefaultInstallationName = "krateoctl"
	// InstallationFinalizer prevents accidental deletion of the installation state.
	InstallationFinalizer = "krateoctl.krateo.io/protect-state"
)

var installationGVR = schema.GroupVersionResource{
	Group:    "krateo.io",
	Version:  "v1",
	Resource: "installations",
}

// Snapshot captures the computed installation configuration (components, steps).
type Snapshot struct {
	ComponentsDefinition map[string]any   `json:"componentsDefinition,omitempty" yaml:"componentsDefinition,omitempty"`
	Steps                []map[string]any `json:"steps,omitempty" yaml:"steps,omitempty"`
}

// Installation is the CR representation persisted to the cluster.
type Installation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InstallationSpec `json:"spec"`
}

// InstallationSpec mirrors the CRD layout (spec.spec).
type InstallationSpec struct {
	Spec Snapshot `json:"spec"`
}

// Store persists and retrieves installation snapshots.
type Store interface {
	Save(ctx context.Context, name string, snapshot *Snapshot) error
	Load(ctx context.Context, name string) (*Snapshot, error)
}

type manager struct {
	client    dynamic.Interface
	namespace string
}

// NewStore builds a Store backed by a dynamic client for the Installation CRD.
func NewStore(cfg *rest.Config, namespace string) (Store, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &manager{client: dyn, namespace: namespace}, nil
}

func (m *manager) resource() dynamic.ResourceInterface {
	return m.client.Resource(installationGVR).Namespace(m.namespace)
}

// Save upserts the provided snapshot into the Installation resource with the given name.
func (m *manager) Save(ctx context.Context, name string, snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("installation snapshot is nil")
	}

	inst := &Installation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Installation",
			APIVersion: "krateo.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  m.namespace,
			Finalizers: []string{InstallationFinalizer},
		},
		Spec: InstallationSpec{Spec: *snapshot},
	}

	existing, err := m.resource().Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		u, convErr := installationToUnstructured(inst)
		if convErr != nil {
			return convErr
		}
		_, createErr := m.resource().Create(ctx, u, metav1.CreateOptions{})
		return createErr
	case err != nil:
		return fmt.Errorf("get installation: %w", err)
	default:
		inst.ObjectMeta.ResourceVersion = existing.GetResourceVersion()
		inst.ObjectMeta.Finalizers = mergeFinalizers(existing.GetFinalizers(), inst.ObjectMeta.Finalizers)
		u, convErr := installationToUnstructured(inst)
		if convErr != nil {
			return convErr
		}
		_, updateErr := m.resource().Update(ctx, u, metav1.UpdateOptions{})
		return updateErr
	}
}

func installationToUnstructured(inst *Installation) (*unstructured.Unstructured, error) {
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(inst)
	if err != nil {
		return nil, fmt.Errorf("convert installation to unstructured: %w", err)
	}
	return &unstructured.Unstructured{Object: obj}, nil
}

func mergeFinalizers(existing, desired []string) []string {
	result := make([]string, 0, len(existing)+len(desired))
	seen := make(map[string]struct{}, len(existing)+len(desired))

	for _, f := range existing {
		if f == "" {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		result = append(result, f)
		seen[f] = struct{}{}
	}

	for _, f := range desired {
		if f == "" {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		result = append(result, f)
		seen[f] = struct{}{}
	}

	return result
}

// Load returns the stored snapshot for the given installation name.
func (m *manager) Load(ctx context.Context, name string) (*Snapshot, error) {
	u, err := m.resource().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var inst Installation
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &inst); err != nil {
		return nil, fmt.Errorf("decode installation: %w", err)
	}

	snap := inst.Spec.Spec
	normalizeSnapshot(&snap)
	return &snap, nil
}

// BuildSnapshot converts the resolved config and steps into a Snapshot object suitable for persistence.
func BuildSnapshot(cfg *config.Config, steps []*types.Step) (*Snapshot, error) {
	snap := &Snapshot{}

	if cfg != nil {
		if doc := cfg.Document(); doc != nil {
			components, err := copyMap(doc.ComponentsDefinition)
			if err != nil {
				return nil, err
			}
			snap.ComponentsDefinition = components
		}
	}

	convertedSteps, err := copySteps(steps)
	if err != nil {
		return nil, err
	}
	snap.Steps = convertedSteps

	return snap, nil
}

func copyMap(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal map value: %w", err)
	}

	if string(data) == "null" {
		return nil, nil
	}

	out := make(map[string]any)
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal map value: %w", err)
	}

	if len(out) == 0 {
		return nil, nil
	}

	return out, nil
}

func copySteps(steps []*types.Step) ([]map[string]any, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(steps)
	if err != nil {
		return nil, fmt.Errorf("marshal steps: %w", err)
	}

	var out []map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal steps: %w", err)
	}

	return out, nil
}

func normalizeSnapshot(snap *Snapshot) {
	if snap == nil {
		return
	}

	if snap.ComponentsDefinition != nil && len(snap.ComponentsDefinition) == 0 {
		snap.ComponentsDefinition = nil
	}
	if len(snap.Steps) == 0 {
		snap.Steps = nil
	}
}
