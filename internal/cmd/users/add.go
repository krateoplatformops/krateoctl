package users

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/internal/util/flags"
	"github.com/krateoplatformops/krateoctl/internal/util/kube"
	"github.com/krateoplatformops/plumbing/jwtutil"
	"github.com/krateoplatformops/plumbing/signup"
)

func AddCommand() subcommands.Command {
	return &addUserCmd{}
}

const (
	envJwtSignKey = "JWT_SIGN_KEY"
	envNamespce   = "NAMESPACE"
)

type addUserCmd struct {
	username   string
	groups     flags.StringSlice
	jwtSignKey string
	namespace  string
	duration   time.Duration
	serverURL  string
}

func (c *addUserCmd) Name() string { return "add-user" }
func (c *addUserCmd) Synopsis() string {
	return "register a new Krateo user and generate credentials"
}

func (c *addUserCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s\n\n", c.Synopsis())

	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprintf(&wri, "  krateoctl %s [FLAGS]\n\n", c.Name())

	fmt.Fprint(&wri, "ENV VARS:\n\n")
	fmt.Fprintf(&wri, "  %s\tJWT sign key\n", envJwtSignKey)
	fmt.Fprintf(&wri, "  %s\tnamespace for generated user config secret\n\n", envNamespce)

	return wri.String()
}

func (c *addUserCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.namespace, "n", os.Getenv(envNamespce), "namespace for generated user config secret")
	f.StringVar(&c.jwtSignKey, "k", os.Getenv(envJwtSignKey), "JWT sign key")
	f.DurationVar(&c.duration, "d", time.Hour*2, "generated certificate duration")
	f.Var(&c.groups, "g", "groups the user belongs to")
	f.StringVar(&c.serverURL, "s", "https://kubernetes.default.svc",
		"kubernetes api server url for generated kubeconfig")
}

func (c *addUserCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: missing username")
		return subcommands.ExitFailure
	}

	if c.namespace == "" {
		fmt.Fprintln(os.Stderr, "error: empty namespace")
		return subcommands.ExitFailure
	}

	c.username = fs.Args()[0]
	if c.groups == nil {
		c.groups = []string{"devs"}
	}

	cfg, err := kube.RestConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client-go rest.Config: %s\n", err.Error())
		return subcommands.ExitFailure
	}

	if c.serverURL == "" {
		c.serverURL = cfg.Host
	}

	accessToken, err := jwtutil.CreateToken(jwtutil.CreateTokenOptions{
		Username:   c.username,
		Groups:     c.groups,
		SigningKey: c.jwtSignKey,
		Duration:   c.duration,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create jwt: %s\n", err.Error())
		return subcommands.ExitFailure
	}

	fmt.Fprintln(os.Stdout, accessToken)

	caData := base64.StdEncoding.EncodeToString(cfg.CAData)

	_, err = signup.Do(ctx, signup.Options{
		RestConfig:   cfg,
		CAData:       caData,
		ServerURL:    c.serverURL,
		CertDuration: c.duration,
		Namespace:    c.namespace,
		Username:     c.username,
		UserGroups:   c.groups,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create new krateo user: %s\n", err.Error())
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
