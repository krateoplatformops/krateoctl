package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/cache"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/workflows/steps"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ steps.Handler[*steps.ObjectResult] = (*objStepHandler)(nil)

func ObjectHandler(app *applier.Applier, del *deletor.Deletor, env *cache.Cache[string, string], logr logging.Logger) steps.Handler[*steps.ObjectResult] {
	return &objStepHandler{
		app: app, del: del, env: env,
		subst: func(k string) string {
			if v, ok := env.Get(k); ok {
				return v
			}

			return "$" + k
		},
		logr: logr,
	}
}

type objStepHandler struct {
	app   *applier.Applier
	del   *deletor.Deletor
	env   *cache.Cache[string, string]
	ns    string
	op    steps.Op
	subst func(k string) string
	logr  logging.Logger
}

func (r *objStepHandler) Namespace(ns string) {
	r.ns = ns
}

func (r *objStepHandler) Op(op steps.Op) {
	r.op = op
}

func (r *objStepHandler) Handle(ctx context.Context, id string, ext *runtime.RawExtension) (*steps.ObjectResult, error) {
	uns, err := r.toUnstructured(id, ext)
	if err != nil {
		return nil, err
	}

	gv, err := schema.ParseGroupVersion(uns.GetAPIVersion())
	if err != nil {
		return nil, err
	}

	result := &steps.ObjectResult{
		APIVersion: uns.GetAPIVersion(),
		Kind:       uns.GetKind(),
		Name:       uns.GetName(),
		Namespace:  uns.GetNamespace(),
	}

	if r.op == steps.Delete {
		result.Operation = "delete"
		err := r.del.Delete(ctx, deletor.DeleteOptions{
			GVK:       gv.WithKind(uns.GetKind()),
			Namespace: uns.GetNamespace(),
			Name:      uns.GetName(),
		})
		if apierrors.IsNotFound(err) {
			err = nil
		}
		return result, err
	}

	result.Operation = "apply"
	err = r.app.Apply(ctx, uns.Object, applier.ApplyOptions{
		GVK:       gv.WithKind(uns.GetKind()),
		Namespace: uns.GetNamespace(),
		Name:      uns.GetName(),
	})

	return result, err
}

func (r *objStepHandler) toUnstructured(id string, ext *runtime.RawExtension) (*unstructured.Unstructured, error) {
	res := types.Object{}
	err := json.Unmarshal(ext.Raw, &res)
	if err != nil {
		return nil, err
	}

	namespace := res.Metadata.Namespace
	if len(namespace) == 0 {
		namespace = r.ns
	}

	src := map[string]any{
		"apiVersion": res.APIVersion,
		"kind":       res.Kind,
		"metadata": map[string]any{
			"name":      res.Metadata.Name,
			"namespace": namespace,
		},
	}

	mergeMaps(src, res.BodyFields)

	r.logr.Debug(fmt.Sprintf("DBG [object:%s]: %v", id, src))

	return &unstructured.Unstructured{Object: src}, nil
}

func mergeMaps(dest, src map[string]any) {
	for k, v := range src {
		// If both are maps, recurse to merge them deep
		if srcMap, ok := v.(map[string]any); ok {
			if dstMap, ok := dest[k].(map[string]any); ok {
				mergeMaps(dstMap, srcMap)
				continue
			}
		}
		// Otherwise, overwrite or set (using your NoCopy logic for speed
		// or SetNestedField for safety)
		dest[k] = v
	}
}
