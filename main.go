package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/krateoplatformops/krateoctl/internal/cmd/gencrd"
	"github.com/krateoplatformops/krateoctl/internal/cmd/genschema"
	"github.com/krateoplatformops/krateoctl/internal/cmd/install"
	"github.com/krateoplatformops/krateoctl/internal/cmd/users"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
)

const (
	appName = "krateoctl"

	// Category names for subcommands
	categoryInstallation = "installation management"
	categoryUtilities    = "utilities"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	tool := subcommands.NewCommander(flag.CommandLine, appName)
	tool.Banner = func(w io.Writer) {
		fmt.Fprintf(w, "┬┌─┬─┐┌─┐┌┬┐┌─┐┌─┐Platform\n")
		fmt.Fprintf(w, "├┴┐├┬┘├─┤ │ ├┤ │ │     Ops\n")
		fmt.Fprintf(w, "┴ ┴┴└─┴ ┴ ┴ └─┘└─┘\n")
		fmt.Fprintf(w, "               CTL (ver: %s, bld: %s)\n\n", version, commit)
	}

	// Installation management commands

	tool.Register(install.Command(), categoryInstallation)

	// Utilities
	tool.Register(genschema.Command(), categoryUtilities)
	tool.Register(gencrd.Command(), categoryUtilities)
	tool.Register(users.AddCommand(), categoryUtilities)

	flag.Parse()

	ctx := context.Background()
	os.Exit(int(tool.Execute(ctx)))
}
