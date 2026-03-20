package migrate

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	_ "embed"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/config"
	"github.com/krateoplatformops/krateoctl/internal/install/migrate/legacy"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

type restConfigProvider func() (*rest.Config, error)
type dynamicFactory func(*rest.Config) (dynamic.Interface, error)
type fileWriter func(string, []byte, os.FileMode) error

//go:embed assets/componentsDefinition.yaml
var componentsDefinitionYAML []byte

//go:embed assets/componentsDefinition-loadbalancer.yaml
var componentsDefinitionLoadbalancerYAML []byte

//go:embed assets/componentsDefinition-ingress.yaml
var componentsDefinitionIngressYAML []byte

var legacyGVRs = []schema.GroupVersionResource{
	{Group: "krateo.io", Version: "v1alpha1", Resource: "krateoplatformops"},
	{Group: "krateo.io", Version: "v1alpha1", Resource: "krateoplatformopses"},
}

// Command builds the "install migrate" subcommand entrypoint.
func Command() subcommands.Command {
	return &migrateCmd{}
}

type migrateCmd struct {
	installType string
	name        string
	namespace   string
	outputPath  string
	force       bool
	debug       bool

	restConfigFn   restConfigProvider
	dynamicFactory dynamicFactory
	writeFile      fileWriter
}

func (c *migrateCmd) Name() string { return "migrate" }
func (c *migrateCmd) Synopsis() string {
	return "convert a legacy KrateoPlatformOps resource into krateo.yaml"
}

func (c *migrateCmd) Usage() string {
	buf := &bytes.Buffer{}
	buf.WriteString(c.Synopsis())
	buf.WriteString("\n\nUSAGE:\n\n")
	buf.WriteString("  krateoctl install migrate [FLAGS]\n\n")
	buf.WriteString("FLAGS:\n\n")
	buf.WriteString("  --type string\n        installation type: nodeport, loadbalancer, or ingress (default \"nodeport\")\n")
	fmt.Fprintf(buf, "  --namespace string\n        namespace that contains the KrateoPlatformOps resource (default \"%s\")\n", shared.DefaultNamespace)
	buf.WriteString("  --name string\n        name of the KrateoPlatformOps resource (default \"krateo\")\n")
	fmt.Fprintf(buf, "  --output string\n        path to write the generated krateo.yaml (default \"%s\")\n", shared.DefaultConfigPath)
	buf.WriteString("  --force\n        overwrite the output file if it already exists\n")
	buf.WriteString("  --debug\n        enable debug-level logging (can also use KRATEOCTL_DEBUG env var)\n")

	return buf.String()
}

func (c *migrateCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.installType, "type", "nodeport", "installation type: nodeport, loadbalancer, or ingress")
	f.StringVar(&c.namespace, "namespace", shared.DefaultNamespace, "namespace that contains the KrateoPlatformOps resource")
	f.StringVar(&c.name, "name", "krateo", "name of the KrateoPlatformOps resource")
	f.StringVar(&c.outputPath, "output", shared.DefaultConfigPath, "path to write the generated krateo.yaml")
	f.BoolVar(&c.force, "force", false, "overwrite the output file if it already exists")
	f.BoolVar(&c.debug, "debug", false, "enable debug-level logging")
}

func (c *migrateCmd) ensureDeps() {
	if c.namespace == "" {
		c.namespace = shared.DefaultNamespace
	}
	if c.outputPath == "" {
		c.outputPath = shared.DefaultConfigPath
	}
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
}

func (c *migrateCmd) Execute(ctx context.Context, _ *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	c.ensureDeps()

	// Enable debug mode from flag or environment variable
	logLevel := ui.LevelInfo
	if c.debug || os.Getenv(shared.KRATEOCTL_DEBUG_ENV) != "" {
		logLevel = ui.LevelDebug
	}
	logger := ui.NewLogger(os.Stderr, logLevel)

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

	logger.Info("Fetching legacy KrateoPlatformOps from cluster: %s/%s", c.namespace, c.name)
	legacyObj, err := c.fetchLegacyResource(ctx, dyn)
	if err != nil {
		logger.Error("Failed to read KrateoPlatformOps resource: %v", err)
		return subcommands.ExitFailure
	}

	doc, err := legacy.ConvertDocument(legacyObj.Object, c.namespace)
	if err != nil {
		logger.Error("Failed to convert legacy spec: %v", err)
		return subcommands.ExitFailure
	}

	if err := c.applyDefaultComponents(doc, c.installType); err != nil {
		logger.Error("Failed to load components definition: %v", err)
		return subcommands.ExitFailure
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		logger.Error("Failed to marshal converted configuration: %v", err)
		return subcommands.ExitFailure
	}

	if err := c.writeOutput(data); err != nil {
		logger.Error("Failed to write %s: %v", c.outputPath, err)
		return subcommands.ExitFailure
	}

	logger.Info("✓ Migrated legacy steps into %s", c.outputPath)
	logger.Info("ℹ Review the generated file, then plan/apply with krateoctl. When ready, remove the old controller and KrateoPlatformOps CR manually.")
	return subcommands.ExitSuccess
}

func (c *migrateCmd) fetchLegacyResource(ctx context.Context, dyn dynamic.Interface) (*unstructured.Unstructured, error) {
	var lastErr error
	for _, gvr := range legacyGVRs {
		obj, err := dyn.Resource(gvr).Namespace(c.namespace).Get(ctx, c.name, metav1.GetOptions{})
		switch {
		case err == nil:
			return obj, nil
		case apierrors.IsNotFound(err):
			lastErr = err
			continue
		case meta.IsNoMatchError(err):
			lastErr = err
			continue
		default:
			return nil, err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("KrateoPlatformOps %s/%s not found", c.namespace, c.name)
	}
	return nil, lastErr
}

func (c *migrateCmd) writeOutput(data []byte) error {
	if !c.force {
		if _, err := os.Stat(c.outputPath); err == nil {
			return fmt.Errorf("output file %s already exists (use -force to overwrite)", c.outputPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	dir := filepath.Dir(c.outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return c.writeFile(c.outputPath, data, 0o644)
}

func (c *migrateCmd) applyDefaultComponents(doc *config.Document, installType string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	var componentData []byte
	switch installType {
	case "loadbalancer":
		componentData = componentsDefinitionLoadbalancerYAML
	case "ingress":
		componentData = componentsDefinitionIngressYAML
	case "nodeport", "":
		componentData = componentsDefinitionYAML
	default:
		return fmt.Errorf("unknown installation type: %s (expected: nodeport, loadbalancer, or ingress)", installType)
	}

	components, err := loadComponentsDefinition(componentData)
	if err != nil {
		return err
	}

	doc.ComponentsDefinition = components
	return nil
}

func loadComponentsDefinition(data []byte) (map[string]config.ComponentConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("components definition asset is empty")
	}

	var payload struct {
		ComponentsDefinition map[string]config.ComponentConfig `json:"componentsDefinition" yaml:"componentsDefinition"`
	}

	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse components definition: %w", err)
	}

	if len(payload.ComponentsDefinition) == 0 {
		return nil, fmt.Errorf("components definition asset missing entries")
	}

	return payload.ComponentsDefinition, nil
}
