package migrate

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"

	_ "embed"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/shared"
	"github.com/krateoplatformops/krateoctl/internal/install/migrate/legacy"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"

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
	legacyObj, err := fetchLegacyResource(ctx, dyn, c.namespace, c.name)
	if err != nil {
		logger.Error("Failed to read KrateoPlatformOps resource: %v", err)
		return subcommands.ExitFailure
	}

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

	if err := writeOutputFile(writeOutputOptions{
		outputPath: c.outputPath,
		force:      c.force,
		writeFile:  c.writeFile,
		data:       data,
	}); err != nil {
		logger.Error("Failed to write %s: %v", c.outputPath, err)
		return subcommands.ExitFailure
	}

	logger.Info("✓ Migrated legacy steps into %s", c.outputPath)
	logger.Info("ℹ Review the generated file, then plan/apply with krateoctl. When ready, remove the old controller and KrateoPlatformOps CR manually.")
	return subcommands.ExitSuccess
}
