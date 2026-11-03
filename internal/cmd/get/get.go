package get

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Command() subcommands.Command {
	return &getCmd{}
}

type getCmd struct {
	namespace     string
	allNamespaces bool
	output        string
	selector      string
	fieldSelector string
}

func (c *getCmd) Name() string {
	return "get"
}

func (c *getCmd) Synopsis() string {
	return "retrieve Krateo compositions and related data"
}

func (c *getCmd) Usage() string {
	return `get <resource> [TYPE/NAME | NAME] [FLAGS]

Available resources:
  compositions    list or get composition resources with automatic version discovery
                  across all CRDs in the composition.krateo.io group

Arguments:
  NAME            get a single composition by name; if two CRD kinds share the
                  same name you will get an error — use TYPE/NAME to be explicit
  TYPE/NAME       get a resource by its exact CRD plural type, e.g.:
                    githubscaffoldinglifecycles/my-composition

Flags:
`
}

func (c *getCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.namespace, "namespace", "", "namespace to query")
	f.StringVar(&c.namespace, "n", "", "namespace to query")
	f.BoolVar(&c.allNamespaces, "A", false, "list across all namespaces")
	f.StringVar(&c.output, "o", "", "output format: yaml, json, name")
	f.StringVar(&c.selector, "l", "", "label selector (same as -l)")
	f.StringVar(&c.fieldSelector, "field-selector", "", "field selector for filtering")
}

func (c *getCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...any) subcommands.ExitStatus {
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
		return c.getCompositions(ctx, local.Args()[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown resource %q\n", sub)
		return subcommands.ExitUsageError
	}
}

func (c *getCmd) getCompositions(ctx context.Context, args []string) subcommands.ExitStatus {
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "error: only one resource name may be provided")
		return subcommands.ExitUsageError
	}

	var name string
	if len(args) == 1 {
		name = args[0]
	}

	if name != "" && c.allNamespaces {
		fmt.Fprintln(os.Stderr, "error: cannot request a specific resource while listing all namespaces")
		return subcommands.ExitUsageError
	}

	cfg, err := kube.RestConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build kubeconfig: %v\n", err)
		return subcommands.ExitFailure
	}

	namespace := c.namespace
	if namespace == "" && !c.allNamespaces {
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

	format, err := resources.NormalizeOutputFormat(c.output, resources.FormatTable)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitUsageError
	}

	if name != "" {
		var obj *unstructured.Unstructured
		if plural, resourceName, ok := strings.Cut(name, "/"); ok {
			// TYPE/NAME form: explicit CRD plural given
			obj, err = manager.GetByResource(ctx, namespace, plural, resourceName)
		} else {
			obj, err = manager.Get(ctx, namespace, name)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return subcommands.ExitFailure
		}

		if err := resources.WriteResource(os.Stdout, obj, format); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return subcommands.ExitFailure
		}

		return subcommands.ExitSuccess
	}

	list, err := manager.List(ctx, namespace, c.allNamespaces, metav1.ListOptions{
		LabelSelector: c.selector,
		FieldSelector: c.fieldSelector,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	if err := resources.WriteList(os.Stdout, list, format); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
