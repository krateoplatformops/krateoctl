package install

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/krateoplatformops/krateoctl/internal/cmd/install/apply"
	"github.com/krateoplatformops/krateoctl/internal/cmd/install/plan"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
)

func Command() subcommands.Command {
	return &installCmd{}
}

type installCmd struct{}

func (c *installCmd) Name() string     { return "install" }
func (c *installCmd) Synopsis() string { return "installation workflows (plan/apply)" }

func (c *installCmd) Usage() string {
	w := &bytes.Buffer{}
	fmt.Fprintf(w, "%s\n\n", c.Synopsis())
	fmt.Fprint(w, "USAGE:\n\n")
	fmt.Fprint(w, "  krateoctl install <plan|apply> [FLAGS]\n\n")
	fmt.Fprint(w, "SUBCOMMANDS:\n\n")
	fmt.Fprint(w, "  plan   preview configuration changes\n")
	fmt.Fprint(w, "  apply  apply configuration changes to cluster\n")
	return w.String()
}

func (c *installCmd) SetFlags(f *flag.FlagSet) {
	// No top-level flags for `install` itself; they belong to subcommands.
}

func (c *installCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if fs.NArg() < 1 {
		fmt.Fprint(os.Stderr, c.Usage())
		return subcommands.ExitUsageError
	}

	name := fs.Arg(0)

	var cmd subcommands.Command
	switch name {
	case "plan":
		cmd = plan.Command()
	case "apply":
		cmd = apply.Command()
	default:
		fmt.Fprintf(os.Stderr, "unknown install subcommand %q (expected: plan|apply)\n", name)
		return subcommands.ExitUsageError
	}

	subfs := flag.NewFlagSet(name, flag.ContinueOnError)
	subfs.Usage = func() { fmt.Fprint(os.Stderr, cmd.Usage()) }
	cmd.SetFlags(subfs)

	if err := subfs.Parse(fs.Args()[1:]); err != nil {
		return subcommands.ExitUsageError
	}

	return cmd.Execute(ctx, subfs)
}
