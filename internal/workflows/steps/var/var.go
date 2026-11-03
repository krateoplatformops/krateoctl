package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/cache"
	"github.com/krateoplatformops/krateoctl/internal/dynamic"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/expand"
	"github.com/krateoplatformops/krateoctl/internal/workflows/steps"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ steps.Handler[*steps.VarResult] = (*varStepHandler)(nil)

func VarHandler(dyn *getter.Getter, env *cache.Cache[string, string], logger func(string, ...any)) steps.Handler[*steps.VarResult] {
	return &varStepHandler{
		dyn:    dyn,
		env:    env,
		logger: logger,
		subst: func(k string) string {
			if v, ok := env.Get(k); ok {
				return v
			}

			return "$" + k
		},
	}
}

type varStepHandler struct {
	dyn    *getter.Getter
	env    *cache.Cache[string, string]
	logger func(string, ...any)
	ns     string
	subst  func(k string) string
	op     steps.Op
}

func (r *varStepHandler) Op(op steps.Op) {
	r.op = op
}

func (r *varStepHandler) Namespace(ns string) {
	r.ns = ns
}

func (r *varStepHandler) Handle(ctx context.Context, id string, ext *map[string]any) (*steps.VarResult, error) {
	res := types.Var{}
	data, err := json.Marshal(ext)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chart step input: %w", err)
	}

	err = json.Unmarshal(data, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal var step input: %w", err)
	}

	result := &steps.VarResult{
		Name: res.Name,
	}

	if len(res.Value) > 0 {
		val := expand.Expand(res.Value, "", r.subst)
		r.env.Set(res.Name, val)
		result.Value = val

		r.logger(fmt.Sprintf(
			" step (id: %s), type: var (name: %s, value: %s)",
			id, res.Name, val))
	} else {
		r.logger(fmt.Sprintf(
			" step (id: %s), type: var (name: %s) with.Value is empty", id, res.Name))
	}

	if res.ValueFrom == nil {
		r.logger(fmt.Sprintf(" step (id: %s), type: var (name: %s), with.valueFrom is empty", id, res.Name))
		return result, nil
	}

	gv, err := schema.ParseGroupVersion(res.ValueFrom.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API version: %w", err)
	}

	namespace := res.ValueFrom.Metadata.Namespace
	if len(namespace) == 0 {
		namespace = r.ns
	}

	name := res.ValueFrom.Metadata.Name

	obj, err := r.dyn.Get(ctx, getter.GetOptions{
		Name:      name,
		Namespace: namespace,
		GVK:       gv.WithKind(res.ValueFrom.Kind),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	val, err := dynamic.Extract(ctx, obj, res.ValueFrom.Selector)
	if val != nil {
		valStr := steps.Strval(val)
		r.env.Set(res.Name, valStr)
		result.Value = valStr
	}

	return result, err
}
