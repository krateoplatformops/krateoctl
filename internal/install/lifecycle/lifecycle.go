package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/krateoctl/internal/util/remote"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
)

type GetterFactory func(*rest.Config) (*getter.Getter, error)

type ApplyOptions struct {
	Phase            string
	Version          string
	Repository       string
	ConfigFile       string
	RestConfig       *rest.Config
	JobNameSuffix    string
	InstallationType string
}

type loadOptions struct {
	phase            string
	version          string
	repository       string
	configFile       string
	installationType string
}

type Manager struct {
	namespace     string
	getterFactory GetterFactory
}

func NewManager(namespace string, getterFactory GetterFactory) *Manager {
	return &Manager{
		namespace:     namespace,
		getterFactory: getterFactory,
	}
}

func (m *Manager) Apply(ctx context.Context, applierClient *applier.Applier, logger *ui.Logger, opts ApplyOptions) error {
	manifests, err := m.loadManifests(ctx, logger, loadOptions{
		phase:            opts.Phase,
		version:          opts.Version,
		repository:       opts.Repository,
		configFile:       opts.ConfigFile,
		installationType: opts.InstallationType,
	})
	if err != nil {
		return err
	}
	if len(manifests) == 0 {
		logger.Info("ℹ No %s manifests found", opts.Phase)
		return nil
	}

	logger.Info("⚡ Applying %d %s manifests...", len(manifests), opts.Phase)

	var jobsToWait []*unstructured.Unstructured
	for _, manifest := range manifests {
		substituteTemplateVariables(manifest.UnstructuredContent(), m.namespace, opts.JobNameSuffix)

		if manifest.GetNamespace() == "" && !isClusterScoped(manifest.GetKind()) {
			manifest.SetNamespace(m.namespace)
		}

		opts := applier.ApplyOptions{
			GVK:       manifest.GroupVersionKind(),
			Namespace: manifest.GetNamespace(),
			Name:      manifest.GetName(),
		}

		if err := applierClient.Apply(ctx, manifest.UnstructuredContent(), opts); err != nil {
			return fmt.Errorf("apply %s %s/%s: %w", manifest.GetKind(), manifest.GetNamespace(), manifest.GetName(), err)
		}

		logger.Info("✓ Applied %s %s/%s", manifest.GetKind(), manifest.GetNamespace(), manifest.GetName())
		if manifest.GetKind() == "Job" {
			jobsToWait = append(jobsToWait, manifest)
		}
	}

	if len(jobsToWait) == 0 {
		return nil
	}

	return m.waitForJobs(ctx, logger, jobsToWait, opts.RestConfig)
}

func (m *Manager) loadManifests(ctx context.Context, logger *ui.Logger, opts loadOptions) ([]*unstructured.Unstructured, error) {
	if opts.version != "" {
		baseRepo := opts.repository
		if baseRepo == "" {
			baseRepo = remote.DefaultRepository
		}
		logger.Info("\n📍 Checking %s manifests from remote: %s/%s", opts.phase, baseRepo, opts.version)

		manifests, err := loadRemoteManifests(ctx, baseRepo, opts.version, opts.phase, opts.installationType)
		if err != nil || len(manifests) == 0 {
			logger.Info("ℹ No %s manifests found (expected)", opts.phase)
			return nil, nil
		}
		return manifests, nil
	}

	configDir := "."
	if opts.configFile != "" {
		configDir = filepath.Dir(opts.configFile)
	}
	logger.Info("\n📍 Checking %s manifests locally: %s", opts.phase, configDir)

	manifests, err := loadLocalManifests(configDir, opts.phase, opts.installationType)
	if err != nil || len(manifests) == 0 {
		logger.Info("ℹ No %s manifests found (expected)", opts.phase)
		return nil, nil
	}

	return manifests, nil
}

func loadLocalManifests(configDir, phase, installationType string) ([]*unstructured.Unstructured, error) {
	// Try type-specific file first (e.g., pre-upgrade.nodeport.yaml)
	if installationType != "" && installationType != "nodeport" {
		typeSpecificPath := filepath.Join(configDir, fmt.Sprintf("%s.%s.yaml", phase, installationType))
		if content, err := os.ReadFile(typeSpecificPath); err == nil {
			return parseManifests(content, typeSpecificPath)
		}
		// Fall through to generic file if type-specific doesn't exist
	}

	// For nodeport, also try .kind.yaml variant
	if installationType == "nodeport" {
		kindPath := filepath.Join(configDir, fmt.Sprintf("%s.kind.yaml", phase))
		if content, err := os.ReadFile(kindPath); err == nil {
			return parseManifests(content, kindPath)
		}
		// Fall through to generic file
	}

	// Fallback to generic manifest file
	filePath := filepath.Join(configDir, fmt.Sprintf("%s.yaml", phase))
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read manifest file %s: %w", filePath, err)
	}

	return parseManifests(content, filePath)
}

func loadRemoteManifests(ctx context.Context, repository, version, phase, installationType string) ([]*unstructured.Unstructured, error) {
	fetcher := remote.NewFetcher()

	// Try type-specific file first (e.g., pre-upgrade.nodeport.yaml)
	if installationType != "" && installationType != "nodeport" {
		filename := fmt.Sprintf("%s.%s.yaml", phase, installationType)
		content, err := fetcher.FetchFile(remote.FetchOptions{
			Repository: repository,
			Version:    version,
			Filename:   filename,
			Timeout:    remote.DefaultTimeout,
		})
		if err == nil && len(content) > 0 {
			return parseManifests(content, fmt.Sprintf("%s/%s", version, filename))
		}
		// Fall through to generic file if type-specific doesn't exist or empty
	}

	// For nodeport, also try .kind.yaml variant
	if installationType == "nodeport" {
		filename := fmt.Sprintf("%s.kind.yaml", phase)
		content, err := fetcher.FetchFile(remote.FetchOptions{
			Repository: repository,
			Version:    version,
			Filename:   filename,
			Timeout:    remote.DefaultTimeout,
		})
		if err == nil && len(content) > 0 {
			return parseManifests(content, fmt.Sprintf("%s/%s", version, filename))
		}
		// Fall through to generic file
	}

	// Fallback to generic manifest file
	filename := fmt.Sprintf("%s.yaml", phase)
	content, err := fetcher.FetchFile(remote.FetchOptions{
		Repository: repository,
		Version:    version,
		Filename:   filename,
		Timeout:    remote.DefaultTimeout,
	})
	if err != nil {
		return nil, nil
	}

	_ = ctx
	return parseManifests(content, fmt.Sprintf("%s/%s", version, filename))
}

func (m *Manager) waitForJobs(ctx context.Context, logger *ui.Logger, jobs []*unstructured.Unstructured, rc *rest.Config) error {
	g, err := m.getterFactory(rc)
	if err != nil {
		return fmt.Errorf("initialize getter for Job monitoring: %w", err)
	}

	waiter := kube.NewJobWaiter(g)
	logger.Info("\n⏳ Waiting for %d Job(s) to complete (max 5 minutes)...", len(jobs))

	for _, job := range jobs {
		if err := waiter.Wait(ctx, job.GetNamespace(), job.GetName()); err != nil {
			return fmt.Errorf("Job %s/%s failed: %w", job.GetNamespace(), job.GetName(), err)
		}
		logger.Info("✓ Job %s/%s completed successfully", job.GetNamespace(), job.GetName())
	}

	logger.Info("✓ All Jobs completed successfully")
	return nil
}

func substituteTemplateVariables(value any, namespace string, jobNameSuffix string) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			switch itemValue := item.(type) {
			case string:
				replaced := strings.ReplaceAll(itemValue, "{{ .Namespace }}", namespace)
				replaced = strings.ReplaceAll(replaced, "{{ .JobNameSuffix }}", jobNameSuffix)
				v[key] = replaced
			default:
				substituteTemplateVariables(itemValue, namespace, jobNameSuffix)
			}
		}
	case []any:
		for i, item := range v {
			switch itemValue := item.(type) {
			case string:
				replaced := strings.ReplaceAll(itemValue, "{{ .Namespace }}", namespace)
				replaced = strings.ReplaceAll(replaced, "{{ .JobNameSuffix }}", jobNameSuffix)
				v[i] = replaced
			default:
				substituteTemplateVariables(itemValue, namespace, jobNameSuffix)
			}
		}
	}
}

func isClusterScoped(kind string) bool {
	clusterScopedKinds := map[string]bool{
		"ClusterRole":              true,
		"ClusterRoleBinding":       true,
		"Namespace":                true,
		"CustomResourceDefinition": true,
		"PersistentVolume":         true,
	}
	return clusterScopedKinds[kind]
}

func parseManifests(content []byte, source string) ([]*unstructured.Unstructured, error) {
	var manifests []*unstructured.Unstructured
	decoder := kyaml.NewYAMLOrJSONDecoder(bytes.NewReader(content), 4096)

	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse YAML in %s: %w", source, err)
		}
		if obj.GetKind() == "" {
			continue
		}
		manifests = append(manifests, obj)
	}

	return manifests, nil
}
