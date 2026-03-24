package migrate

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"

	_ "embed"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/applier"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/deletor"
	"github.com/krateoplatformops/krateoctl/internal/dynamic/getter"
	"github.com/krateoplatformops/krateoctl/internal/install/migrate/legacy"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/krateoctl/internal/workflows"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	helmconfig "github.com/krateoplatformops/plumbing/helm"
	helmv3 "github.com/krateoplatformops/plumbing/helm/v3"
)

type kubeClientFactory func(*rest.Config) (kubernetes.Interface, error)
type stateStoreFactory func(*rest.Config, string) (state.Store, error)
type ensureCRDFunc func(context.Context, *rest.Config) error
type workflowRunner interface {
	Run(context.Context, *types.Workflow, func(*types.Step) bool, workflows.StepNotifier) []workflows.StepResult[any]
}

type sharedWorkflowRunner struct {
	workflowRunner
}

type getterFactory func(*rest.Config) (*getter.Getter, error)
type applierFactory func(*rest.Config) (*applier.Applier, error)
type deletorFactory func(*rest.Config) (*deletor.Deletor, error)
type workflowFactory func(workflows.Opts) (workflowRunner, error)

// CommandFullSpecs builds the "install migrate-full" subcommand entrypoint.
func CommandFullSpecs() subcommands.Command {
	return &migrateFullSpecsCmd{}
}

type migrateFullSpecsCmd struct {
	installType         string
	name                string
	namespace           string
	outputPath          string
	installerNamespace  string
	installerRelease    string
	installerCRDRelease string
	force               bool
	debug               bool

	restConfigFn      restConfigProvider
	dynamicFactory    dynamicFactory
	writeFile         fileWriter
	kubeClientFactory kubeClientFactory
	stateFactory      stateStoreFactory
	ensureCRDFn       ensureCRDFunc
	getterFactory     getterFactory
	applierFactory    applierFactory
	deletorFactory    deletorFactory
	workflowFactory   workflowFactory
	errEvaluator      func([]workflows.StepResult[any]) error
}

func (c *migrateFullSpecsCmd) Name() string { return "migrate-full" }

func (c *migrateFullSpecsCmd) Synopsis() string {
	return "fully migrate a legacy KrateoPlatformOps resource with automatic cutover"
}

func (c *migrateFullSpecsCmd) Usage() string {
	buf := &bytes.Buffer{}
	buf.WriteString(c.Synopsis())
	buf.WriteString("\n\nUSAGE:\n\n")
	buf.WriteString("  krateoctl install migrate-full [FLAGS]\n\n")
	buf.WriteString("FLAGS:\n\n")
	fmt.Fprintf(buf, "  --namespace string\n        namespace that contains the KrateoPlatformOps resource (default \"%s\")\n", shared.DefaultNamespace)
	buf.WriteString("  --name string\n        name of the KrateoPlatformOps resource (default \"krateo\")\n")
	buf.WriteString("  --output string\n        optional path to save the generated krateo.yaml before applying it\n")
	buf.WriteString("  --type string\n        installation type: nodeport, loadbalancer, or ingress (default \"nodeport\")\n")
	buf.WriteString("  --installer-namespace string\n        namespace where the installer is deployed (default: same as --namespace)\n")
	buf.WriteString("  --installer-release string\n        Helm release name for the installer (default \"installer\")\n")
	buf.WriteString("  --installer-crd-release string\n        Helm release name for the installer CRD (default \"installer-crd\")\n")
	buf.WriteString("  --force\n        overwrite the output file if it already exists\n")
	buf.WriteString("  --debug\n        enable debug-level logging (can also use KRATEOCTL_DEBUG env var)\n\n")
	buf.WriteString("PREREQUISITES:\n\n")
	buf.WriteString("  Use this command only for Krateo 2.7.0 installations managed by the installer controller.\n\n")
	buf.WriteString("WHAT THIS COMMAND DOES:\n\n")
	buf.WriteString("  1. Reads the legacy KrateoPlatformOps resource from the cluster.\n")
	buf.WriteString("  2. Converts it into a new krateo.yaml file.\n")
	buf.WriteString("     If --output is set, the file is also saved to disk.\n")
	buf.WriteString("  3. Scales the old installer down.\n")
	buf.WriteString("  4. Applies the new krateo.yaml configuration automatically.\n")
	buf.WriteString("  5. Deletes the old KrateoPlatformOps resource.\n")
	buf.WriteString("  6. Uninstalls the old installer Helm releases.\n\n")
	buf.WriteString("WHEN TO USE IT:\n\n")
	buf.WriteString("  Use this command when you want the full migration to be handled automatically.\n")
	buf.WriteString("  It performs the cutover steps for you after generating krateo.yaml.\n\n")
	buf.WriteString("WHEN TO USE SIMPLE MIGRATE INSTEAD:\n\n")
	buf.WriteString("  Use krateoctl install migrate when you want to generate the file first, review it,\n")
	buf.WriteString("  and run plan/apply manually in separate steps.\n\n")
	buf.WriteString("EXAMPLES:\n\n")
	buf.WriteString("  # Run the full automatic migration\n")
	buf.WriteString("  krateoctl install migrate-full --type nodeport\n\n")
	buf.WriteString("  # Run the full automatic migration and also save the generated file\n")
	buf.WriteString("  krateoctl install migrate-full --type nodeport --output ./krateo.yaml\n\n")
	buf.WriteString("  # Run the full migration when installer releases use custom names\n")
	buf.WriteString("  krateoctl install migrate-full --type ingress --installer-release my-installer --installer-crd-release my-installer-crd\n")

	return buf.String()
}

func (c *migrateFullSpecsCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.namespace, "namespace", shared.DefaultNamespace, "namespace that contains the KrateoPlatformOps resource")
	f.StringVar(&c.name, "name", "krateo", "name of the KrateoPlatformOps resource")
	f.StringVar(&c.outputPath, "output", "", "optional path to save the generated krateo.yaml")
	f.StringVar(&c.installType, "type", "nodeport", "installation type: nodeport, loadbalancer, or ingress")
	f.StringVar(&c.installerNamespace, "installer-namespace", "", "namespace where the installer is deployed")
	f.StringVar(&c.installerRelease, "installer-release", "installer", "Helm release name for the installer")
	f.StringVar(&c.installerCRDRelease, "installer-crd-release", "installer-crd", "Helm release name for the installer CRD")
	f.BoolVar(&c.force, "force", false, "overwrite the output file if it already exists")
	f.BoolVar(&c.debug, "debug", false, "enable debug-level logging")
}

func (c *migrateFullSpecsCmd) ensureDeps() {
	if c.restConfigFn == nil {
		c.restConfigFn = kube.RestConfig
	}
	if c.dynamicFactory == nil {
		c.dynamicFactory = func(cfg *rest.Config) (dynamic.Interface, error) {
			return dynamic.NewForConfig(cfg)
		}
	}
	if c.writeFile == nil {
		c.writeFile = os.WriteFile
	}
	if c.kubeClientFactory == nil {
		c.kubeClientFactory = func(cfg *rest.Config) (kubernetes.Interface, error) {
			return kubernetes.NewForConfig(cfg)
		}
	}
	if c.stateFactory == nil {
		c.stateFactory = shared.DefaultStateStoreFactory
	}
	if c.ensureCRDFn == nil {
		c.ensureCRDFn = state.EnsureCRD
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
	c.namespace = shared.EnsureNamespace(c.namespace)
	if c.installerNamespace == "" {
		c.installerNamespace = c.namespace
	}
}

func (c *migrateFullSpecsCmd) Execute(ctx context.Context, _ *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	c.ensureDeps()

	// Enable debug mode from flag or environment variable
	logger := shared.NewLogger(os.Stderr, c.debug || os.Getenv(shared.KRATEOCTL_DEBUG_ENV) != "")

	rc, err := c.restConfigFn()
	if err != nil {
		logger.Error("Failed to load kubeconfig: %v", err)
		return subcommands.ExitFailure
	}

	dyn, err := c.dynamicFactory(rc)
	if err != nil {
		logger.Error("Failed to initialize dynamic client: %v", err)
		return subcommands.ExitFailure
	}

	// Step 1: Fetch legacy resource
	var legacyObj *unstructured.Unstructured
	logger.Info("Step 1/6: Fetching legacy KrateoPlatformOps CR...")
	legacyObj, err = fetchLegacyResource(ctx, dyn, c.namespace, c.name)
	if err != nil {
		logger.Error("Failed to read KrateoPlatformOps resource: %v", err)
		return subcommands.ExitFailure
	}
	logger.Info("✓ Found KrateoPlatformOps: %s/%s", c.namespace, c.name)

	// Step 2: Convert to new format
	logger.Info("Step 2/6: Converting to new format...")
	doc, err := legacy.ConvertDocument(legacyObj.Object, c.namespace)
	if err != nil {
		logger.Error("Failed to convert legacy spec: %v", err)
		return subcommands.ExitFailure
	}

	if err := applyDefaultComponents(doc, c.installType); err != nil {
		logger.Error("Failed to load components definition: %v", err)
		return subcommands.ExitFailure
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		logger.Error("Failed to marshal converted configuration: %v", err)
		return subcommands.ExitFailure
	}

	if c.outputPath != "" {
		if err := writeOutputFile(writeOutputOptions{
			outputPath: c.outputPath,
			force:      c.force,
			writeFile:  c.writeFile,
			data:       data,
		}); err != nil {
			logger.Error("Failed to write %s: %v", c.outputPath, err)
			return subcommands.ExitFailure
		}
		logger.Info("✓ Generated new configuration: %s", c.outputPath)
	} else {
		logger.Info("✓ Generated new configuration in memory")
	}

	// Step 3: Scale installer to 0
	logger.Info("Step 3/6: Scaling installer to 0...")
	if err := c.scaleInstallerController(ctx, rc, 0); err != nil {
		logger.Error("Failed to scale installer: %v", err)
		return subcommands.ExitFailure
	}
	logger.Info("✓ Scaled installer to 0")

	// Step 4: Apply new configuration (run full install apply workflow)
	logger.Info("Step 4/6: Applying new krateo.yaml configuration...")

	// Ensure CRD exists
	if err := c.ensureCRDFn(ctx, rc); err != nil {
		logger.Error("Failed to ensure Installation CRD: %v", err)
		return subcommands.ExitFailure
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		logger.Error("Failed to parse generated configuration: %v", err)
		return subcommands.ExitFailure
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	// Load and validate the generated config directly from memory.
	result, err := shared.BuildLoadResult(raw, logger.Debug, false)
	if err != nil {
		logger.Error("Failed to load generated configuration: %v", err)
		return subcommands.ExitFailure
	}

	if len(result.Steps) == 0 {
		logger.Info("ℹ No steps configured")
	} else {
		logger.Info("⚡ Executing %d steps...", len(result.Steps))

		execResult, err := shared.ExecuteWorkflow(ctx, rc, shared.ExecuteWorkflowOptions{
			Namespace: c.namespace,
			StateName: state.DefaultInstallationName,
			Logger:    logger,
			Result:    result,
			ProgressReporter: func(idx int, step *types.Step, skipped bool) {
				status := "executing"
				if skipped {
					status = "skipped"
				}
				logger.Debug("Processing step %d/%d: %s (%s)", idx+1, len(result.Steps), step.ID, status)
			},
			SaveState: false,
		}, shared.WorkflowDeps{
			GetterFactory:  shared.GetterFactory(c.getterFactory),
			ApplierFactory: shared.ApplierFactory(c.applierFactory),
			DeletorFactory: shared.DeletorFactory(c.deletorFactory),
			WorkflowFactory: func(opts workflows.Opts) (shared.WorkflowRunner, error) {
				wf, err := c.workflowFactory(opts)
				if err != nil {
					return nil, err
				}
				return sharedWorkflowRunner{workflowRunner: wf}, nil
			},
			ErrEvaluator: shared.ErrEvaluator(c.errEvaluator),
			StateFactory: shared.StateStoreFactory(c.stateFactory),
		})
		if err != nil {
			logger.Error("Workflow execution failed: %v", err)
			if execResult != nil {
				shared.LogWorkflowResults(logger, result.Steps, execResult.Results)
			}
			return subcommands.ExitFailure
		}

		store, err := c.stateFactory(rc, c.namespace)
		if err != nil {
			logger.Error("Failed to initialize installation state store: %v", err)
			return subcommands.ExitFailure
		}
		if err := store.Save(ctx, state.DefaultInstallationName, execResult.Snapshot); err != nil {
			logger.Warn("⚠ Unable to persist installation snapshot: %v", err)
		} else {
			logger.Info("✓ Saved installation snapshot: %s/%s", c.namespace, state.DefaultInstallationName)
		}
		shared.LogWorkflowResults(logger, result.Steps, execResult.Results)
		logger.Info("✓ Successfully applied %d steps", len(result.Steps))
	}

	// Step 5: Remove finalizer and delete old CR
	logger.Info("Step 5/6: Removing finalizer and deleting old KrateoPlatformOps CR...")
	if err := c.removeFinalizer(ctx, dyn); err != nil {
		logger.Error("Failed to remove finalizer: %v", err)
		return subcommands.ExitFailure
	}
	logger.Info("✓ Removed finalizer")

	if err := c.deleteLegacyResource(ctx, dyn); err != nil {
		logger.Error("Failed to delete old KrateoPlatformOps CR: %v", err)
		return subcommands.ExitFailure
	}
	logger.Info("✓ Deleted old KrateoPlatformOps CR")

	// Step 6: Uninstall old installer
	logger.Info("Step 6/6: Uninstalling old installer charts...")
	if err := c.uninstallOldInstaller(ctx, rc); err != nil {
		logger.Error("Failed to uninstall old installer: %v", err)
		logger.Error("ℹ You can manually uninstall with:")
		logger.Error("  helm uninstall %s -n %s", c.installerRelease, c.installerNamespace)
		logger.Error("  helm uninstall %s -n %s", c.installerCRDRelease, c.installerNamespace)
		return subcommands.ExitFailure
	}
	logger.Info("✓ Uninstalled old installer and CRD charts")

	logger.Info("")
	logger.Info("========================================================================")
	logger.Info("✓ Migration completed successfully!")
	logger.Info("========================================================================")

	return subcommands.ExitSuccess
}

func (c *migrateFullSpecsCmd) scaleInstallerController(ctx context.Context, cfg *rest.Config, replicas int32) error {
	clientset, err := c.kubeClientFactory(cfg)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	deploymentsClient := clientset.AppsV1().Deployments(c.installerNamespace)
	deployment, err := deploymentsClient.Get(ctx, "installer", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get installer deployment: %w", err)
	}

	deployment.Spec.Replicas = &replicas
	_, err = deploymentsClient.Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update installer deployment replicas: %w", err)
	}

	return nil
}

func (c *migrateFullSpecsCmd) removeFinalizer(ctx context.Context, dyn dynamic.Interface) error {
	// Try both GVRs to remove finalizer
	var lastErr error
	for _, gvr := range legacyGVRs {
		obj, err := dyn.Resource(gvr).Namespace(c.namespace).Get(ctx, c.name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			continue
		}

		// Remove all finalizers
		if obj.GetFinalizers() != nil {
			obj.SetFinalizers(nil)
			_, err := dyn.Resource(gvr).Namespace(c.namespace).Update(ctx, obj, metav1.UpdateOptions{})
			return err
		}
		return nil
	}
	return lastErr
}

func (c *migrateFullSpecsCmd) deleteLegacyResource(ctx context.Context, dyn dynamic.Interface) error {
	var lastErr error
	for _, gvr := range legacyGVRs {
		err := dyn.Resource(gvr).Namespace(c.namespace).Delete(ctx, c.name, metav1.DeleteOptions{})
		switch {
		case err == nil:
			return nil
		case apierrors.IsNotFound(err):
			return nil // Already deleted, not an error
		case meta.IsNoMatchError(err):
			lastErr = err
			continue
		default:
			return err
		}
	}
	return lastErr
}

func (c *migrateFullSpecsCmd) uninstallOldInstaller(ctx context.Context, cfg *rest.Config) error {
	// Create helm client for the installer namespace
	cli, err := helmv3.NewClient(cfg, helmv3.WithNamespace(c.installerNamespace))
	if err != nil {
		return fmt.Errorf("failed to create helm client: %w", err)
	}

	// Uninstall the installer chart
	err = cli.Uninstall(ctx, c.installerRelease, &helmconfig.UninstallConfig{
		IgnoreNotFound: true,
	})
	if err != nil {
		return fmt.Errorf("failed to uninstall installer chart: %w", err)
	}

	// Uninstall the installer CRD chart
	err = cli.Uninstall(ctx, c.installerCRDRelease, &helmconfig.UninstallConfig{
		IgnoreNotFound: true,
	})
	if err != nil {
		return fmt.Errorf("failed to uninstall installer-crd chart: %w", err)
	}

	return nil
}
