package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/cache"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/expand"
	helmconfig "github.com/krateoplatformops/plumbing/helm"
	helm "github.com/krateoplatformops/plumbing/helm/v3"

	"github.com/krateoplatformops/krateoctl/internal/workflows/steps"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/client-go/rest"
)

type ChartHandlerOptions struct {
	Dyn    *getter.Getter
	Env    *cache.Cache[string, string]
	Cfg    *rest.Config
	Logger func(string, ...any)
}

func ChartHandler(opts ChartHandlerOptions) steps.Handler[*steps.ChartResult] {
	hdl := &chartStepHandler{
		env:    opts.Env,
		dyn:    opts.Dyn,
		cfg:    opts.Cfg,
		logger: opts.Logger,
	}
	hdl.subst = func(k string) string {
		if v, ok := hdl.env.Get(k); ok {
			return v
		}

		return "$" + k
	}

	return hdl
}

var _ steps.Handler[*steps.ChartResult] = (*chartStepHandler)(nil)

type chartStepHandler struct {
	env    *cache.Cache[string, string]
	ns     string
	op     steps.Op
	subst  func(k string) string
	render bool
	logger func(string, ...any)
	dyn    *getter.Getter
	cfg    *rest.Config
}

func (r *chartStepHandler) Namespace(ns string) {
	r.ns = ns
}

func (r *chartStepHandler) Op(op steps.Op) {
	r.op = op
}

func (r *chartStepHandler) Handle(ctx context.Context, id string, ext *map[string]any) (*steps.ChartResult, error) {

	spec := &types.ChartSpec{}
	data, err := json.Marshal(ext)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chart step input: %w", err)
	}

	err = json.Unmarshal(data, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal chart step input: %w", err)
	}
	spec.SetDefaults()
	if expanded := r.expandValues(spec.Values); expanded != nil {
		if valuesMap, ok := expanded.(map[string]any); ok {
			spec.Values = valuesMap
		}
	}

	namespace := r.ns
	if spec.Namespace != "" {
		namespace = spec.Namespace
	}
	cli, err := helm.NewClient(r.cfg,
		helm.WithNamespace(namespace),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create helm client: %w", err)
	}

	result := &steps.ChartResult{}

	if r.op != steps.Delete {
		result.Operation = "install/upgrade"

		var releaseName string

		if spec.URL != "" {
			releaseName = steps.DeriveReleaseName(spec.URL)
		}
		if spec.Repo != "" {
			releaseName = spec.Repo
		}
		if spec.ReleaseName != "" {
			releaseName = spec.ReleaseName
		}

		release, err := cli.GetRelease(ctx, releaseName, &helmconfig.GetConfig{})
		if err != nil {
			return nil, fmt.Errorf("failed to get release: %w", err)
		}

		actionConfig := &helmconfig.ActionConfig{
			ChartVersion:          spec.Version,
			ChartName:             spec.Repo,
			Values:                spec.Values,
			Wait:                  spec.Wait,
			InsecureSkipTLSverify: spec.InsecureSkipTLSVerify,
			Timeout:               spec.Timeout.Duration,
		}
		if release == nil {
			release, err = cli.Install(ctx,
				releaseName,
				spec.URL,
				&helmconfig.InstallConfig{
					ActionConfig: actionConfig,
				})
			if err != nil {
				return result, fmt.Errorf("failed to install chart: %w", err)
			}
		} else {
			if release.Status == helmconfig.StatusPendingInstall || release.Status == helmconfig.StatusPendingUpgrade || release.Status == helmconfig.StatusPendingRollback {
				release, err = cli.Rollback(ctx, releaseName, &helmconfig.RollbackConfig{
					ReleaseVersion: release.Revision,
				})
				if err != nil {
					return result, fmt.Errorf("failed to rollback release %s: %w", releaseName, err)
				}
			}
			release, err = cli.Upgrade(ctx,
				releaseName,
				spec.URL,
				&helmconfig.UpgradeConfig{
					ActionConfig: actionConfig,
					MaxHistory:   *spec.MaxHistory,
				})
			if err != nil {
				return result, fmt.Errorf("failed to upgrade chart: %w", err)
			}
		}

		r.logger(fmt.Sprintf(
			"[chart:%s]: %s operation completed for release %s",
			id, result.Operation, result.ReleaseName))

		return result, nil
	}

	result.Operation = "uninstall"

	err = cli.Uninstall(ctx, spec.ReleaseName, &helmconfig.UninstallConfig{
		IgnoreNotFound: true,
	})
	if err != nil {
		return result, fmt.Errorf("failed to uninstall chart: %w", err)
	}

	result.Status = "uninstalled"

	r.logger(fmt.Sprintf(
		"[chart:%s]: uninstall operation completed for release %s",
		id, result.ReleaseName))

	return result, nil
}

// expandValues walks Helm values and resolves ${VAR} placeholders via the shared cache.
func (r *chartStepHandler) expandValues(val any) any {
	switch v := val.(type) {
	case map[string]any:
		for key, elem := range v {
			v[key] = r.expandValues(elem)
		}
		return v
	case []any:
		for i, elem := range v {
			v[i] = r.expandValues(elem)
		}
		return v
	case string:
		return expand.Expand(v, "", r.subst)
	default:
		return val
	}
}
