package resources

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const (
	compositionGroup        = "composition.krateo.io"
	compositionCategory     = "compositions"
	compositionCRDGroup     = "apiextensions.k8s.io"
	compositionCRDVersion   = "v1"
	compositionCRDResource  = "customresourcedefinitions"
	compositionVersionLabel = "krateo.io/composition-version"
)

type compositionGVR struct {
	gvr        schema.GroupVersionResource
	namespaced bool
}

// CompositionsManager discovers every CRD in the composition.krateo.io group
// that carries the "compositions" category and provides unified List/Get/Patch
// operations across all of them.
type CompositionsManager struct {
	dynamicClient dynamic.Interface
	resources     []compositionGVR
}

func NewCompositionsManager(ctx context.Context, cfg *rest.Config) (*CompositionsManager, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	mgr := &CompositionsManager{dynamicClient: dyn}
	if err := mgr.discoverCRDs(ctx); err != nil {
		return nil, err
	}

	return mgr, nil
}

// discoverCRDs lists all CRDs, retains those in composition.krateo.io whose
// spec.names.categories contains "compositions", and resolves a served version
// for each one.
func (m *CompositionsManager) discoverCRDs(ctx context.Context) error {
	crdGVR := schema.GroupVersionResource{
		Group:    compositionCRDGroup,
		Version:  compositionCRDVersion,
		Resource: compositionCRDResource,
	}

	list, err := m.dynamicClient.Resource(crdGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing CRDs: %w", err)
	}

	for _, item := range list.Items {
		group, _, _ := unstructured.NestedString(item.Object, "spec", "group")
		if group != compositionGroup {
			continue
		}

		categories, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "names", "categories")
		if !containsString(categories, compositionCategory) {
			continue
		}

		plural, _, _ := unstructured.NestedString(item.Object, "spec", "names", "plural")
		if plural == "" {
			continue
		}

		scope, _, _ := unstructured.NestedString(item.Object, "spec", "scope")
		namespaced := !strings.EqualFold(scope, "Cluster")

		labelVersion := item.GetLabels()[compositionVersionLabel]
		version, err := pickServedVersion(&item, labelVersion)
		if err != nil {
			continue
		}

		m.resources = append(m.resources, compositionGVR{
			gvr: schema.GroupVersionResource{
				Group:    compositionGroup,
				Version:  version,
				Resource: plural,
			},
			namespaced: namespaced,
		})
	}

	if len(m.resources) == 0 {
		return fmt.Errorf("no CRDs found with category %q in group %q", compositionCategory, compositionGroup)
	}

	return nil
}

// pickServedVersion returns the best version to use for a CRD:
//  1. served version matching labelVersion
//  2. storage:true version
//  3. first served version
func pickServedVersion(crd *unstructured.Unstructured, labelVersion string) (string, error) {
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil {
		return "", fmt.Errorf("reading CRD versions: %w", err)
	}

	var firstServed, storageVersion string

	for _, raw := range versions {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := entry["name"].(string)
		served, _ := entry["served"].(bool)
		storage, _ := entry["storage"].(bool)

		if name == "" || !served {
			continue
		}

		if firstServed == "" {
			firstServed = name
		}
		if storage && storageVersion == "" {
			storageVersion = name
		}
		if labelVersion != "" && strings.EqualFold(name, labelVersion) {
			return name, nil
		}
	}

	if storageVersion != "" {
		return storageVersion, nil
	}
	if firstServed != "" {
		return firstServed, nil
	}
	return "", fmt.Errorf("no served versions found for CRD %s", crd.GetName())
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// refetchAtLabelVersion re-fetches obj at the version stored in its
// krateo.io/composition-version label, if that version differs from the one
// used to retrieve it in the first place. This ensures the caller always sees
// the object at the version it was originally created at rather than the CRD
// storage version.
func (m *CompositionsManager) refetchAtLabelVersion(ctx context.Context, crd compositionGVR, namespace string, obj *unstructured.Unstructured) *unstructured.Unstructured {
	labelVer := obj.GetLabels()[compositionVersionLabel]
	if labelVer == "" || strings.EqualFold(labelVer, crd.gvr.Version) {
		return obj
	}

	targetGVR := schema.GroupVersionResource{
		Group:    crd.gvr.Group,
		Version:  labelVer,
		Resource: crd.gvr.Resource,
	}

	var ri dynamic.ResourceInterface
	if crd.namespaced {
		ri = m.dynamicClient.Resource(targetGVR).Namespace(namespace)
	} else {
		ri = m.dynamicClient.Resource(targetGVR)
	}

	if refetched, err := ri.Get(ctx, obj.GetName(), metav1.GetOptions{}); err == nil {
		return refetched
	}
	return obj
}

func (m *CompositionsManager) resourceInterface(crd compositionGVR, namespace string, allNamespaces bool) dynamic.ResourceInterface {
	ri := m.dynamicClient.Resource(crd.gvr)
	if !crd.namespaced {
		return ri
	}

	if allNamespaces {
		return ri.Namespace(corev1.NamespaceAll)
	}

	return ri.Namespace(namespace)
}

// List aggregates results from all discovered composition GVRs.
// Each item is re-fetched at the version indicated by its krateo.io/composition-version
// label so the returned apiVersion always matches what was originally applied.
func (m *CompositionsManager) List(ctx context.Context, namespace string, allNamespaces bool, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	combined := &unstructured.UnstructuredList{}
	combined.SetAPIVersion("v1")
	combined.SetKind("List")

	for _, crd := range m.resources {
		ri := m.resourceInterface(crd, namespace, allNamespaces)
		list, err := ri.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("listing %s: %w", crd.gvr.Resource, err)
		}
		ns := namespace
		if allNamespaces {
			ns = ""
		}
		for i := range list.Items {
			itemNS := list.Items[i].GetNamespace()
			if itemNS == "" {
				itemNS = ns
			}
			combined.Items = append(combined.Items, *m.refetchAtLabelVersion(ctx, crd, itemNS, &list.Items[i]))
		}
	}

	return combined, nil
}

// Get searches all discovered composition GVRs for a resource with the given name.
// Returns an error if the same name exists in more than one CRD kind, instructing
// the caller to use TYPE/NAME (e.g. githubscaffoldinglifecycles/foo) instead.
func (m *CompositionsManager) Get(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	type hit struct {
		crd compositionGVR
		obj *unstructured.Unstructured
	}

	var hits []hit
	for _, crd := range m.resources {
		ri := m.resourceInterface(crd, namespace, false)
		obj, err := ri.Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			hits = append(hits, hit{crd, m.refetchAtLabelVersion(ctx, crd, namespace, obj)})
		}
	}

	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("composition %q not found in namespace %q", name, namespace)
	case 1:
		return hits[0].obj, nil
	default:
		var kinds []string
		for _, h := range hits {
			kinds = append(kinds, h.crd.gvr.Resource)
		}
		return nil, fmt.Errorf(
			"ambiguous: %q exists in multiple CRD kinds (%s); use TYPE/NAME to be specific",
			name, strings.Join(kinds, ", "),
		)
	}
}

// GetByResource fetches a resource by explicit plural type and name, e.g.
// GetByResource(ctx, ns, "githubscaffoldinglifecycles", "foo").
func (m *CompositionsManager) GetByResource(ctx context.Context, namespace, plural, name string) (*unstructured.Unstructured, error) {
	for _, crd := range m.resources {
		if !strings.EqualFold(crd.gvr.Resource, plural) {
			continue
		}
		ri := m.resourceInterface(crd, namespace, false)
		obj, err := ri.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return m.refetchAtLabelVersion(ctx, crd, namespace, obj), nil
	}
	return nil, fmt.Errorf("no CRD with plural %q found in group %q", plural, compositionGroup)
}

// Patch finds the composition by name across all GVRs and patches it at the
// version indicated by the resource's krateo.io/composition-version label.
// Returns an error if the name is ambiguous; use PatchByResource for explicit lookup.
func (m *CompositionsManager) Patch(ctx context.Context, namespace, name string, patchType types.PatchType, patch []byte) (*unstructured.Unstructured, error) {
	type hit struct {
		crd compositionGVR
	}

	var hits []hit
	for _, crd := range m.resources {
		ri := m.resourceInterface(crd, namespace, false)
		if _, err := ri.Get(ctx, name, metav1.GetOptions{}); err == nil {
			hits = append(hits, hit{crd})
		}
	}

	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("composition %q not found in namespace %q", name, namespace)
	case 1:
		return m.patchAt(ctx, hits[0].crd, namespace, name, patchType, patch)
	default:
		var kinds []string
		for _, h := range hits {
			kinds = append(kinds, h.crd.gvr.Resource)
		}
		return nil, fmt.Errorf(
			"ambiguous: %q exists in multiple CRD kinds (%s); use TYPE/NAME to be specific",
			name, strings.Join(kinds, ", "),
		)
	}
}

// PatchByResource patches a resource by explicit plural type and name.
func (m *CompositionsManager) PatchByResource(ctx context.Context, namespace, plural, name string, patchType types.PatchType, patch []byte) (*unstructured.Unstructured, error) {
	for _, crd := range m.resources {
		if !strings.EqualFold(crd.gvr.Resource, plural) {
			continue
		}
		return m.patchAt(ctx, crd, namespace, name, patchType, patch)
	}
	return nil, fmt.Errorf("no CRD with plural %q found in group %q", plural, compositionGroup)
}

func (m *CompositionsManager) patchAt(ctx context.Context, crd compositionGVR, namespace, name string, patchType types.PatchType, patch []byte) (*unstructured.Unstructured, error) {
	// Try to patch at the version the resource was originally created at.
	obj, err := m.resourceInterface(crd, namespace, false).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	labelVer := obj.GetLabels()[compositionVersionLabel]
	targetGVR := crd.gvr
	if labelVer != "" && !strings.EqualFold(labelVer, crd.gvr.Version) {
		targetGVR.Version = labelVer
	}

	var ri dynamic.ResourceInterface
	if crd.namespaced {
		ri = m.dynamicClient.Resource(targetGVR).Namespace(namespace)
	} else {
		ri = m.dynamicClient.Resource(targetGVR)
	}
	return ri.Patch(ctx, name, patchType, patch, metav1.PatchOptions{})
}
