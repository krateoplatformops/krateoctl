package steps

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/krateoplatformops/krateoctl/internal/cache"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	helmconfig "github.com/krateoplatformops/plumbing/helm"
	helm "github.com/krateoplatformops/plumbing/helm/v3"

	"github.com/krateoplatformops/krateoctl/internal/workflows/steps"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

const (
	defaultWaitTimeout = "5m"
)

type ChartHandlerOptions struct {
	Dyn *getter.Getter
	Env *cache.Cache[string, string]
	Log logging.Logger
	Cfg *rest.Config
}

func ChartHandler(opts ChartHandlerOptions) steps.Handler[*steps.ChartResult] {
	hdl := &chartStepHandler{
		env:  opts.Env,
		logr: opts.Log,
		dyn:  opts.Dyn,
		cfg:  opts.Cfg,
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
	logr   logging.Logger
	dyn    *getter.Getter
	cfg    *rest.Config
}

func (r *chartStepHandler) Namespace(ns string) {
	r.ns = ns
}

func (r *chartStepHandler) Op(op steps.Op) {
	r.op = op
}

func (r *chartStepHandler) Handle(ctx context.Context, id string, ext *runtime.RawExtension) (*steps.ChartResult, error) {

	spec := &types.ChartSpec{}
	err := json.Unmarshal(ext.Raw, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal chart spec: %w", err)
	}
	spec.SetDefaults()

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

		fmt.Println("Release name:", releaseName)

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
			release, err = cli.Upgrade(ctx,
				releaseName,
				spec.URL,
				&helmconfig.UpgradeConfig{
					ActionConfig: actionConfig,
				})
			if err != nil {
				return result, fmt.Errorf("failed to upgrade chart: %w", err)
			}
		}

		// if release != nil {
		// 	result.Status = string(release.Info.Status)
		// 	result.ChartVersion = release.Chart.Metadata.Version
		// 	result.AppVersion = release.Chart.Metadata.AppVersion
		// 	result.ReleaseName = release.Name
		// 	result.ChartName = release.Chart.Metadata.Name
		// 	result.Namespace = release.Namespace
		// 	result.Updated = metav1.NewTime(release.Info.LastDeployed.Time)
		// 	result.Revision = release.Version
		// }

		r.logr.Debug(fmt.Sprintf(
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

	r.logr.Debug(fmt.Sprintf(
		"[chart:%s]: uninstall operation completed for release %s",
		id, result.ReleaseName))

	return result, nil
}
