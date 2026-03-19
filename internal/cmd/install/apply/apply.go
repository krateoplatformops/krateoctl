package apply

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/secrets"
	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/krateoctl/internal/util/remote"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
)

type workflowRunner interface {
	Run(context.Context, *types.Workflow, func(*types.Step) bool, workflows.StepNotifier) []workflows.StepResult[any]
}

type restConfigProvider func() (*rest.Config, error)
type getterFactory func(*rest.Config) (*getter.Getter, error)
type applierFactory func(*rest.Config) (*applier.Applier, error)
type deletorFactory func(*rest.Config) (*deletor.Deletor, error)
type workflowFactory func(workflows.Opts) (workflowRunner, error)
type stateStoreFactory func(*rest.Config, string) (state.Store, error)
type ensureCRDFunc func(context.Context, *rest.Config) error

func Command() subcommands.Command {
	return &applyCmd{}
}

type applyCmd struct {
	configFile     string
	namespace      string
	profile        string
	version        string
	repository     string
	debug          bool
	initSecrets    bool // Hidden utility flag for generating sample secrets
	skipValidation bool // Skip configuration validation

	restConfigFn    restConfigProvider
	getterFactory   getterFactory
	applierFactory  applierFactory
	deletorFactory  deletorFactory
	workflowFactory workflowFactory
	errEvaluator    func([]workflows.StepResult[any]) error
	stateFactory    stateStoreFactory
	ensureCRDFn     ensureCRDFunc
	stateName       string
}

func (c *applyCmd) ensureDeps() {
	if c.restConfigFn == nil {
		c.restConfigFn = kube.RestConfig
	}
	if c.getterFactory == nil {
		c.getterFactory = getter.NewGetter
	}
	if c.applierFactory == nil {
		c.applierFactory = applier.NewApplier
	}
	if c.deletorFactory == nil {
		c.deletorFactory = deletor.NewDeletor
	}
	if c.workflowFactory == nil {
		c.workflowFactory = func(opts workflows.Opts) (workflowRunner, error) {
			return workflows.New(opts)
		}
	}
	if c.errEvaluator == nil {
		c.errEvaluator = func(results []workflows.StepResult[any]) error {
			return workflows.Err(results)
		}
	}
	if c.stateFactory == nil {
		c.stateFactory = func(cfg *rest.Config, namespace string) (state.Store, error) {
			return state.NewStore(cfg, namespace)
		}
	}
	if c.ensureCRDFn == nil {
		c.ensureCRDFn = state.EnsureCRD
	}
	if c.stateName == "" {
		c.stateName = state.DefaultInstallationName
	}
}

func (c *applyCmd) Name() string     { return "apply" }
func (c *applyCmd) Synopsis() string { return "apply configuration changes to cluster" }

func (c *applyCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s. Load the installation config and execute the workflow.\n\n", c.Synopsis())
	fmt.Fprint(&wri, "USAGE:\n  krateoctl install apply [FLAGS]\n\n")
	fmt.Fprint(&wri, "FLAGS:\n")
	fmt.Fprint(&wri, "  --version string      version/tag to fetch from the releases repository (enables remote mode)\n")
	fmt.Fprint(&wri, "  --repository string   GitHub repository URL for releases (default \"https://github.com/krateoplatformops/releases\")\n")
	fmt.Fprintf(&wri, "  --config string       path to local configuration file (default \"%s\", used when --version is not set)\n", shared.DefaultConfigPath)
	fmt.Fprintf(&wri, "  --namespace string    target namespace (default \"%s\")\n", shared.DefaultNamespace)
	fmt.Fprint(&wri, "  --profile string      optional profile name (e.g. dev, prod)\n")
	fmt.Fprint(&wri, "  --skip-validation     skip configuration validation (useful for emergency recovery)\n")
	fmt.Fprint(&wri, "  --debug               enable debug-level logging (can also use KRATEOCTL_DEBUG env var)\n\n")
	fmt.Fprint(&wri, "MODES:\n\n")
	fmt.Fprint(&wri, "  Remote mode: When --version is specified, config is fetched from the releases\n")
	fmt.Fprint(&wri, "               repository instead of local filesystem.\n")
	fmt.Fprint(&wri, "  Local mode:  When --version is not specified, config is read from local files.\n\n")
	fmt.Fprint(&wri, "EXAMPLES:\n\n")
	fmt.Fprint(&wri, "  # Apply from a specific release version (remote mode)\n")
	fmt.Fprint(&wri, "  krateoctl install apply --version v1.0.0\n\n")
	fmt.Fprint(&wri, "  # Apply from a custom repository\n")
	fmt.Fprint(&wri, "  krateoctl install apply --version v1.0.0 --repository https://github.com/myorg/krateo-releases\n\n")
	fmt.Fprint(&wri, "  # Apply using local config file\n")
	fmt.Fprint(&wri, "  krateoctl install apply --config ./my-krateo.yaml\n\n")
	return wri.String()
}

func (c *applyCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.version, "version", "", "version/tag to fetch from the releases repository")
	f.StringVar(&c.repository, "repository", "", "GitHub repository URL for releases")
	f.StringVar(&c.configFile, "config", shared.DefaultConfigPath, "path to local configuration file")
	f.StringVar(&c.namespace, "namespace", shared.DefaultNamespace, "kubernetes namespace for deployment")
	f.StringVar(&c.profile, "profile", "", "optional profile name")
	f.BoolVar(&c.skipValidation, "skip-validation", false, "skip configuration validation")
	f.BoolVar(&c.debug, "debug", false, "enable debug-level logging")
	// Hidden utility flag - not documented in Usage()
	f.BoolVar(&c.initSecrets, "init-secrets", false, "")
}

func (c *applyCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	c.ensureDeps()

	// 1. Initialize UI and Logging
	// Enable debug mode from flag or environment variable
	debugMode := c.debug || os.Getenv(shared.KRATEOCTL_DEBUG_ENV) != ""
	logLevel := ui.LevelInfo
	if debugMode {
		logLevel = ui.LevelDebug
	}

	spin := ui.NewSpinner(os.Stdout)
	l := ui.NewLogger(spin, logLevel)
	defer spin.Stop("")

	var installationStore state.Store

	// Generate a timestamp for unique job names (to avoid conflicts with previously failed jobs)
	jobNameSuffix := time.Now().Format("20060102-150405")

	// 2. Load Configuration
	result, err := shared.LoadConfigAndSteps(config.LoadOptions{
		ConfigPath:        c.configFile,
		UserOverridesPath: shared.DefaultOverridesPath,
		Profile:           c.profile,
		Version:           c.version,
		Repository:        c.repository,
	}, l.Info, c.skipValidation)
	if err != nil {
		l.Error("Failed to load configuration: %v", err)
		return subcommands.ExitFailure
	}

	if len(result.Steps) == 0 {
		l.Info("ℹ No steps configured")
		return subcommands.ExitSuccess
	}

	snapshot, err := state.BuildSnapshot(result.Config, result.Steps)
	if err != nil {
		l.Error("Failed to build installation snapshot: %v", err)
		return subcommands.ExitFailure
	}

	// 3. Setup Kubernetes Connection
	l.Info("\n📡 Connecting to Kubernetes cluster...")
	rc, err := c.restConfigFn()
	if err != nil {
		l.Error("Failed to load kubeconfig: %v", err)
		return subcommands.ExitFailure
	}
	l.Info("✓ Kubernetes connection established")

	if err := c.ensureCRDFn(ctx, rc); err != nil {
		l.Error("Failed to ensure installation CRD: %v", err)
		return subcommands.ExitFailure
	}

	// Initialize sample secrets if requested (hidden utility feature)
	if c.initSecrets {
		l.Info("\n🔐 Initializing sample secrets...")
		if err := secrets.InitializeSecrets(ctx, rc, c.namespace); err != nil {
			l.Error("Failed to initialize secrets: %v", err)
			return subcommands.ExitFailure
		}
		l.Info("✓ Sample secrets created successfully (%s, %s, %s) in namespace '%s'", secrets.KrateoDbSecretName, secrets.KrateoDbUserSecretName, secrets.JWTSecretName, c.namespace)
	}

	installationStore, err = c.stateFactory(rc, c.namespace)
	if err != nil {
		l.Error("Failed to initialize installation state store: %v", err)
		return subcommands.ExitFailure
	}

	// 4. Initialize Workflow
	wf, err := c.initWorkflow(rc, l)
	if err != nil {
		l.Error("Initialization failed: %v", err)
		return subcommands.ExitFailure
	}

	// 4.5. Apply Pre-Upgrade Manifests (if they exist)
	a, err := c.applierFactory(rc)
	if err != nil {
		l.Error("Failed to initialize applier: %v", err)
		return subcommands.ExitFailure
	}

	if err := c.applyLifecycleManifests(ctx, a, l, "pre-upgrade", c.version, c.repository, c.configFile, rc, jobNameSuffix); err != nil {
		l.Error("Failed to apply pre-upgrade manifests: %v", err)
		return subcommands.ExitFailure
	}

	// 5. Execute Workflow
	l.Info("\n⚡ Applying %d steps to namespace '%s'...", len(result.Steps), c.namespace)
	l.Info("═════════════════════════════════════════════════════════════")

	// Start Spinner
	spin.SetPrefix("⚙  ")
	spin.Start()

	// Pass the Progress Reporter (moved to a local helper)
	results := wf.Run(ctx, &types.Workflow{Steps: result.Steps}, func(s *types.Step) bool {
		return s.Skip
	}, c.createProgressReporter(spin, l, len(result.Steps)))

	spin.Stop("")

	// 6. Final Report
	l.Info("═════════════════════════════════════════════════════════════")
	c.printSummary(l, result.Steps, results)

	if err := c.errEvaluator(results); err != nil {
		l.Error("\nWorkflow completed with errors.")
		return subcommands.ExitFailure
	}

	// 6.5. Apply Post-Upgrade Manifests (if they exist)
	if err := c.applyLifecycleManifests(ctx, a, l, "post-upgrade", c.version, c.repository, c.configFile, rc, jobNameSuffix); err != nil {
		l.Error("Failed to apply post-upgrade manifests: %v", err)
		return subcommands.ExitFailure
	}

	if installationStore != nil && snapshot != nil {
		if err := installationStore.Save(ctx, c.stateName, snapshot); err != nil {
			l.Warn("⚠ Unable to persist installation snapshot: %v", err)
		} else {
			l.Info("✓ Installation snapshot saved as %q with apiVersion %q and kind %q in the namespace %q", c.stateName, "krateo.io/v1", "Installation", c.namespace)
		}
	}

	l.Info("✓ Successfully applied %d steps\n", len(result.Steps))
	return subcommands.ExitSuccess
}

// initWorkflow abstracts the creation of the workflow and its clients.
func (c *applyCmd) initWorkflow(rc *rest.Config, l *ui.Logger) (workflowRunner, error) {
	g, err := c.getterFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("getter: %w", err)
	}
	a, err := c.applierFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("applier: %w", err)
	}
	d, err := c.deletorFactory(rc)
	if err != nil {
		return nil, fmt.Errorf("deletor: %w", err)
	}

	return c.workflowFactory(workflows.Opts{
		Getter:    g,
		Applier:   a,
		Deletor:   d,
		Logger:    l.Debug,
		Cfg:       rc,
		Namespace: c.namespace,
	})
}

func (c *applyCmd) createProgressReporter(spin *ui.Spinner, l *ui.Logger, total int) workflows.StepNotifier {
	return func(idx int, step *types.Step, skipped bool) {
		status := "executing"
		if skipped {
			status = "skipped"
		}
		spin.SetSuffix(fmt.Sprintf("step %d/%d - %s (%s)", idx+1, total, step.ID, status))

		l.V(ui.LevelDebug).Info("Processing workflow step: index=%d id=%s type=%s",
			idx+1, step.ID, step.Type)
	}
}

func (c *applyCmd) printSummary(l *ui.Logger, steps []*types.Step, results []workflows.StepResult[any]) {
	for i, step := range steps {
		res := results[i]
		switch {
		case res.ID() == "":
			l.Info("[PEND] %s (%s) not executed", step.ID, step.Type)
		case step.Skip:
			l.Info("[SKIP] %s (%s)", step.ID, step.Type)
		case res.Err() != nil:
			l.Error("%s (%s) failed: %v", step.ID, step.Type, res.Err())
		default:
			l.Info("✓ %s (%s)", step.ID, step.Type)
		}
	}
}

// applyLifecycleManifests loads and applies pre/post-upgrade manifests from local or remote sources.
// It looks for files named {phase}.yaml or {phase}.yml (e.g., pre-upgrade.yaml, post-upgrade.yaml).
// If manifests are not found, it returns without error (non-critical operation).
func (c *applyCmd) applyLifecycleManifests(ctx context.Context, a *applier.Applier, l *ui.Logger, phase string, version, repository, configFile string, rc *rest.Config, jobNameSuffix string) error {
	var manifests []*unstructured.Unstructured
	var err error

	// Determine the path based on mode (remote vs local)
	if version != "" {
		// Remote mode: fetch from repository
		baseRepo := repository
		if baseRepo == "" {
			baseRepo = remote.DefaultRepository
		}
		filename := fmt.Sprintf("%s.yaml", phase)
		manifestsPath := fmt.Sprintf("%s/%s/%s", baseRepo, version, filename)
		l.Info("\n📍 Checking %s manifests from remote: %s", phase, manifestsPath)

		manifests, err = c.loadRemoteManifests(ctx, baseRepo, version, phase)
		if err != nil {
			l.Info("ℹ No %s manifests found (expected)", phase)
			return nil // Not an error - manifests are optional
		}
	} else {
		// Local mode: check for file relative to config file
		configDir := "."
		if configFile != "" {
			configDir = filepath.Dir(configFile)
		}
		manifestsPath := filepath.Join(configDir, fmt.Sprintf("%s.yaml", phase))

		l.Info("\n📍 Checking %s manifests locally: %s", phase, manifestsPath)

		manifests, err = c.loadLocalManifestsFile(manifestsPath)
		if err != nil {
			l.Info("ℹ No %s manifests found (expected)", phase)
			return nil // Not an error - manifests are optional
		}
	}

	if len(manifests) == 0 {
		l.Info("ℹ No %s manifests found", phase)
		return nil
	}

	return c.applyParsedManifests(ctx, a, l, manifests, phase, rc, jobNameSuffix)
}

// loadLocalManifestsFile loads all YAML documents from a single local file using a proper YAML parser.
func (c *applyCmd) loadLocalManifestsFile(filePath string) ([]*unstructured.Unstructured, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist - not an error
		}
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return parseManifests(content, filePath)
}

// loadRemoteManifests loads manifests from a remote repository by fetching a specific file.
func (c *applyCmd) loadRemoteManifests(ctx context.Context, repository, version, phase string) ([]*unstructured.Unstructured, error) {
	filename := fmt.Sprintf("%s.yaml", phase)

	fetcher := remote.NewFetcher()
	opts := remote.FetchOptions{
		Repository: repository,
		Version:    version,
		Filename:   filename,
		Timeout:    remote.DefaultTimeout,
	}

	content, err := fetcher.FetchFile(opts)
	if err != nil {
		// If file not found (404), it's not an error - manifests are optional
		return nil, nil
	}

	return parseManifests(content, fmt.Sprintf("%s/%s", version, filename))
}

// applyParsedManifests applies a list of parsed Kubernetes manifests and waits for Jobs to complete.
func (c *applyCmd) applyParsedManifests(ctx context.Context, a *applier.Applier, l *ui.Logger, manifests []*unstructured.Unstructured, phase string, rc *rest.Config, jobNameSuffix string) error {
	if len(manifests) == 0 {
		return nil
	}

	l.Info("⚡ Applying %d %s manifests...", len(manifests), phase)

	var jobsToWait []*unstructured.Unstructured

	// Apply all manifests
	for _, manifest := range manifests {
		// Substitute template variables in the manifest FIRST (before checking namespace)
		c.substituteTemplateVariables(manifest, c.namespace, jobNameSuffix)

		// Set the namespace if not already set and if applicable
		if manifest.GetNamespace() == "" && !isClusterScoped(manifest.GetKind()) {
			manifest.SetNamespace(c.namespace)
		}

		// Extract GVK and convert to ApplyOptions
		gvk := manifest.GroupVersionKind()
		opts := applier.ApplyOptions{
			GVK:       gvk,
			Namespace: manifest.GetNamespace(),
			Name:      manifest.GetName(),
		}

		// Convert unstructured to map
		content := manifest.UnstructuredContent()

		if err := a.Apply(ctx, content, opts); err != nil {
			return fmt.Errorf("failed to apply %s %s/%s: %w",
				manifest.GetKind(), manifest.GetNamespace(), manifest.GetName(), err)
		}

		l.Info("✓ Applied %s %s/%s", manifest.GetKind(), manifest.GetNamespace(), manifest.GetName())

		// Track Jobs for waiting
		if manifest.GetKind() == "Job" {
			jobsToWait = append(jobsToWait, manifest)
		}
	}

	// Wait for all Jobs to complete before continuing
	if len(jobsToWait) > 0 {
		if err := c.waitForJobs(ctx, l, jobsToWait, rc); err != nil {
			return err
		}
	}

	return nil
}

// waitForJobs waits for a list of Kubernetes Jobs to complete.
func (c *applyCmd) waitForJobs(ctx context.Context, l *ui.Logger, jobs []*unstructured.Unstructured, rc *rest.Config) error {
	g, err := c.getterFactory(rc)
	if err != nil {
		return fmt.Errorf("failed to initialize getter for Job monitoring: %w", err)
	}

	waiter := kube.NewJobWaiter(g)

	l.Info("\n⏳ Waiting for %d Job(s) to complete (max 5 minutes)...", len(jobs))

	for _, job := range jobs {
		if err := waiter.Wait(ctx, job.GetNamespace(), job.GetName()); err != nil {
			return fmt.Errorf("Job %s/%s failed: %w", job.GetNamespace(), job.GetName(), err)
		}
		l.Info("✓ Job %s/%s completed successfully", job.GetNamespace(), job.GetName())
	}

	l.Info("✓ All Jobs completed successfully")
	return nil
}

// substituteTemplateVariables replaces template variables in a manifest.
// Currently supports {{ .Namespace }} variable.
func (c *applyCmd) substituteTemplateVariables(obj *unstructured.Unstructured, namespace string, jobNameSuffix string) {
	content := obj.UnstructuredContent()
	c.substituteInMap(content, namespace, jobNameSuffix)
}

// substituteInMap recursively replaces template variables in a map.
func (c *applyCmd) substituteInMap(m map[string]interface{}, namespace string, jobNameSuffix string) {
	for key, value := range m {
		switch v := value.(type) {
		case string:
			s := strings.ReplaceAll(v, "{{ .Namespace }}", namespace)
			s = strings.ReplaceAll(s, "{{ .JobNameSuffix }}", jobNameSuffix)
			m[key] = s
		case map[string]interface{}:
			c.substituteInMap(v, namespace, jobNameSuffix)
		case []interface{}:
			c.substituteInSlice(v, namespace, jobNameSuffix)
		}
	}
}

// substituteInSlice recursively replaces template variables in a slice.
func (c *applyCmd) substituteInSlice(s []interface{}, namespace string, jobNameSuffix string) {
	for i, value := range s {
		switch v := value.(type) {
		case string:
			str := strings.ReplaceAll(v, "{{ .Namespace }}", namespace)
			str = strings.ReplaceAll(str, "{{ .JobNameSuffix }}", jobNameSuffix)
			s[i] = str
		case map[string]interface{}:
			c.substituteInMap(v, namespace, jobNameSuffix)
		case []interface{}:
			c.substituteInSlice(v, namespace, jobNameSuffix)
		}
	}
}

// isClusterScoped returns true for cluster-scoped Kubernetes resources.
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

// parseManifests parses YAML content into Kubernetes unstructured objects using a proper YAML parser.
// It handles multiple YAML documents separated by --- and properly handles edge cases like
// empty documents, comments, and YAML format variations.
func parseManifests(content []byte, source string) ([]*unstructured.Unstructured, error) {
	var manifests []*unstructured.Unstructured

	// Use Kubernetes' proper YAML decoder which handles multiple documents correctly
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(content), 4096)

	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err != nil {
			// io.EOF signals end of documents
			if err == io.EOF {
				break
			}
			// If it's an actual parsing error, return it
			return nil, fmt.Errorf("failed to parse YAML in %s: %w", source, err)
		}

		// Skip empty objects (empty documents)
		if obj.GetKind() == "" {
			continue
		}

		manifests = append(manifests, obj)
	}

	return manifests, nil
}
