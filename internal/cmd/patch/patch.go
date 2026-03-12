package patch

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/resources"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	helpers "github.com/krateoplatformops/krateoctl/internal/util/cmd"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func Command() subcommands.Command {
	return &patchCmd{}
}

type patchCmd struct {
	namespace string
	patch     string
	patchFile string
	patchType string
	output    string
}

func (c *patchCmd) Name() string {
	return "patch"
}

func (c *patchCmd) Synopsis() string {
	return "patch Krateo compositions with version-awareness"
}

func (c *patchCmd) Usage() string {
	return `patch <resource> [TYPE/NAME | NAME] [FLAGS]

Available resources:
  compositions    patch a composition resource with automatic version discovery

Arguments:
  NAME            name of the composition to patch; if two CRD kinds share the
                  same name you will get an error — use TYPE/NAME to be specific
  TYPE/NAME       patch a resource by its exact CRD plural type, e.g.:
                    githubscaffoldinglifecycles/my-composition

Flags:
`
}

func (c *patchCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.namespace, "namespace", "", "namespace where the resource lives")
	f.StringVar(&c.namespace, "n", "", "namespace where the resource lives")
	f.StringVar(&c.patch, "patch", "", "inline patch data")
	f.StringVar(&c.patch, "p", "", "inline patch data")
	f.StringVar(&c.patchFile, "file", "", "path to patch data file")
	f.StringVar(&c.patchFile, "f", "", "path to patch data file")
	f.StringVar(&c.patchType, "type", "merge", "patch type (merge|json|strategic)")
	f.StringVar(&c.output, "output", "", "output format: yaml, json, name")
}

func (c *patchCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	interspersed := helpers.IntersperseFlags(fs.Args())
	local := flag.NewFlagSet(fs.Name(), flag.ContinueOnError)
	local.SetOutput(fs.Output())
	c.SetFlags(local)
	local.Usage = func() {
		fmt.Fprint(local.Output(), c.Usage())
		local.PrintDefaults()
	}

	if err := local.Parse(interspersed); err != nil {
		if err == flag.ErrHelp {
			return subcommands.ExitSuccess
		}
		fmt.Fprintln(os.Stderr, err)
		return subcommands.ExitUsageError
	}

	if local.NArg() < 1 {
		fmt.Fprint(os.Stderr, c.Usage())
		return subcommands.ExitUsageError
	}

	sub := local.Arg(0)
	switch sub {
	case "compositions":
		return c.patchCompositions(ctx, local.Args()[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown resource %q\n", sub)
		return subcommands.ExitUsageError
	}
}

func (c *patchCmd) patchCompositions(ctx context.Context, args []string) subcommands.ExitStatus {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "error: composition name is required")
		return subcommands.ExitUsageError
	}

	arg := args[0]

	cfg, err := kube.RestConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build kubeconfig: %v\n", err)
		return subcommands.ExitFailure
	}

	namespace := c.namespace
	if namespace == "" {
		namespace, err = kube.DefaultNamespace()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: unable to determine namespace: %v\n", err)
			return subcommands.ExitFailure
		}
	}

	manager, err := resources.NewCompositionsManager(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	patchData, err := c.resolvePatchData()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitUsageError
	}

	patchType, err := resolvePatchType(c.patchType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitUsageError
	}

	var obj *unstructured.Unstructured
	if plural, name, ok := strings.Cut(arg, "/"); ok {
		// TYPE/NAME form: explicit CRD plural given
		obj, err = manager.PatchByResource(ctx, namespace, plural, name, patchType, patchData)
	} else {
		obj, err = manager.Patch(ctx, namespace, arg, patchType, patchData)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	format, err := resources.NormalizeOutputFormat(c.output, resources.FormatYAML)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitUsageError
	}

	if err := resources.WriteResource(os.Stdout, obj, format); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

func (c *patchCmd) resolvePatchData() ([]byte, error) {
	if c.patch != "" && c.patchFile != "" {
		return nil, fmt.Errorf("only one of --patch or --file may be provided")
	}

	if c.patch != "" {
		return []byte(c.patch), nil
	}

	if c.patchFile != "" {
		return os.ReadFile(c.patchFile)
	}

	return nil, fmt.Errorf("either --patch or --file is required")
}

func resolvePatchType(kind string) (types.PatchType, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "merge":
		return types.MergePatchType, nil
	case "json":
		return types.JSONPatchType, nil
	case "strategic":
		return types.StrategicMergePatchType, nil
	default:
		return "", fmt.Errorf("unsupported patch type %q", kind)
	}
}
