package gencrd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/krateoplatformops/crdgen/v2"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"github.com/krateoplatformops/krateoctl/jsonschema"
)

func Command() subcommands.Command {
	return &genCRDCmd{}
}

const (
	envJSONSchema       = "JSONSCHEMA"
	envOutputDir        = "OUTPUTDIR"
	envAllowedResources = "ALLOWED_RESOURCES"
)

type genCRDCmd struct {
	jsonSchemaFile   string
	crdFile          string
	allowedResources string
}

func (c *genCRDCmd) Name() string { return "gen-widget" }
func (c *genCRDCmd) Synopsis() string {
	return "generate a Krateo Widget CRD from a widget json schema"
}

func (c *genCRDCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s\n\n", c.Synopsis())

	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprintf(&wri, "  krateoctl %s [FLAGS] <path/to/jsonschema>\n\n", c.Name())

	fmt.Fprint(&wri, "ARGUMENTS:\n\n")
	fmt.Fprint(&wri, "  <path/to/jsonschema>   json schema file from which to generate the Widget CRD\n\n")

	fmt.Fprint(&wri, "ENV VARS:\n\n")
	fmt.Fprintf(&wri, "  %s\tjson schema file from which to generate the Widget CRD\n", envJSONSchema)
	fmt.Fprintf(&wri, "  %s\tgenerated Widget CRD output dir (optional)\n\n", envOutputDir)

	return wri.String()
}

func (c *genCRDCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.crdFile, "output", os.Getenv(envOutputDir), "generated Widget CRD output dir (optional)")
	f.StringVar(&c.allowedResources, "allowed-resources", os.Getenv(envAllowedResources),
		"comma separated list of allowed resources under 'resourceRefs' items (optional)")
}

func (c *genCRDCmd) Execute(ctx context.Context, fs *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if fs.NArg() == 0 {
		c.jsonSchemaFile = os.Getenv(envJSONSchema)
	} else {
		c.jsonSchemaFile = fs.Args()[0]
	}
	if c.jsonSchemaFile == "" {
		fmt.Fprintln(os.Stderr, "error: missing input jsonschema file")
		return subcommands.ExitFailure
	}

	if c.crdFile == "" {
		dir := filepath.Dir(c.jsonSchemaFile)
		base := filepath.Base(c.jsonSchemaFile)
		ext := filepath.Ext(c.jsonSchemaFile)
		c.crdFile = filepath.Join(dir,
			fmt.Sprintf("%s.crd.yaml", strings.TrimSuffix(base, ext)))
	}

	log.Printf("input json schema: %s\n", c.jsonSchemaFile)

	err := os.MkdirAll(filepath.Dir(c.crdFile), os.ModePerm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to create destination dir: %v\n", err)
		return subcommands.ExitFailure
	}
	log.Printf("output widget crd: %s\n", c.crdFile)

	content, err := os.ReadFile(c.jsonSchemaFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	src := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&src); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	kind, version, err := jsonschema.ExtractKindAndVersion(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to extract kind and version from JSON Schema: %v\n", err)
		return subcommands.ExitFailure
	}

	allowedResources, err := jsonschema.ExtractAllowedResources(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to extract allowedResources from JSON Schema: %v\n", err)
		return subcommands.ExitFailure
	}

	spec, err := jsonschema.ExtractSpec(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to extract spec from JSON Schema: %v\n", err)
		return subcommands.ExitFailure
	}

	if c.allowedResources != "" {
		allowedResources := strings.Split(c.allowedResources, ",")
		for i, el := range allowedResources {
			allowedResources[i] = strings.TrimSpace(el)
		}
	}

	if len(allowedResources) > 0 {
		err = jsonschema.SetAllowedResources(spec, allowedResources)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: unable to inject allowed resources into JSON Schema: %v\n", err)
			return subcommands.ExitFailure
		}
	}

	dat, err := json.Marshal(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to convert extracted spec to JSON: %v\n", err)
		return subcommands.ExitFailure
	}

	os.Setenv("KEEP_CODE", "1")

	opts := crdgen.Options{
		Group:        widgetsGroup,
		Version:      version,
		Kind:         kind,
		Categories:   []string{"widgets", "krateo"},
		SpecSchema:   []byte(dat),
		StatusSchema: []byte(preserveUnknownFields),
	}

	res, err := crdgen.Generate(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to generate CRD: %v\n", err)
		return subcommands.ExitFailure
	}

	err = os.WriteFile(c.crdFile, res, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
