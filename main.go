package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/krateoplatformops/krateoctl/internal/cmd/gencrd"
	"github.com/krateoplatformops/krateoctl/internal/cmd/genschema"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
)

const (
	appName = "krateoctl"
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
	tool.Register(genschema.Command(), "")
	tool.Register(gencrd.Command(), "")

	flag.Parse()

	ctx := context.Background()
	os.Exit(int(tool.Execute(ctx)))
}
