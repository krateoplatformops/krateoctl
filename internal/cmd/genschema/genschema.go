package genschema

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/krateoplatformops/krateoctl/internal/schema"
	"github.com/krateoplatformops/krateoctl/internal/subcommands"
	"gopkg.in/yaml.v3"

	"github.com/aquasecurity/table"
)

func Command() subcommands.Command {
	return &genSchemaCmd{}
}

const (
	envYamlFile  = "YAML_FILE"
	envOutputDir = "OUTPUTDIR"
)

type genSchemaCmd struct {
	yamlFile       string
	jsonSchemaFile string
}

func (c *genSchemaCmd) Name() string { return "gen-schema" }
func (c *genSchemaCmd) Synopsis() string {
	return "generate a json schema from a yaml file with annotations"
}

func (c *genSchemaCmd) Usage() string {
	wri := bytes.Buffer{}
	fmt.Fprintf(&wri, "%s\n\n", c.Synopsis())

	fmt.Fprint(&wri, "USAGE:\n\n")
	fmt.Fprintf(&wri, "  krateoctl %s [FLAGS] <path/to/yaml/file>\n\n", c.Name())

	fmt.Fprint(&wri, "ARGUMENTS:\n\n")
	fmt.Fprint(&wri, "  <path/to/yaml/file>   annotated yaml file from which to generate a json schema\n\n")

	fmt.Fprint(&wri, "ENV VARS:\n\n")
	fmt.Fprintf(&wri, "  %s\tannotated yaml file from which to generate a json schema\n", envYamlFile)
	fmt.Fprintf(&wri, "  %s\tgenerated json schema output dir (optional)\n\n", envOutputDir)

	fmt.Fprintf(&wri, "ANNOTATIONS:\n\n")
	fmt.Fprintf(&wri, "  The jsonschema must be between two entries of # @schema :\n\n")

	fmt.Fprintf(&wri, "    # @schema\n")
	fmt.Fprintf(&wri, "    # your: annotation\n")
	fmt.Fprintf(&wri, "    # @schema\n")
	fmt.Fprintf(&wri, "    # you can add comment here as well\n")
	fmt.Fprintf(&wri, "    foo: bar\n\n")

	fmt.Fprintf(&wri, "Available annotations:\n\n")

	tbl := table.New(&wri)
	tbl.SetColumnMaxWidth(34)
	tbl.SetBorders(false)
	//tbl.SetRowLines(false)
	tbl.SetHeaders("Annotation", "Description", "Value(s)")

	tbl.AddRow("type",
		"Defines the jsonschema-type. Supports multiple values like [string, integer] as shortcut to anyOf",
		"object, array, string, number, integer, boolean, null")
	tbl.AddRow("title", "Defines the title field of the object", "string")
	tbl.AddRow("description", "Defines the description field of the object", "string")
	tbl.AddRow("default", "Sets the default value, shown first in the IDE", "string")
	tbl.AddRow("pattern", "Regex pattern to test the value", "string")
	tbl.AddRow("required", "Adds key to required list", "true, false, or array")
	tbl.AddRow("deprecated", "Marks the key as deprecated", "true or false")
	tbl.AddRow("items", "Schema describing array items", "object")
	tbl.AddRow("enum", "Multiple allowed values", "array of string")
	tbl.AddRow("const", "Single allowed value", "string")
	tbl.AddRow("examples", "Example values for the user", "array")
	tbl.AddRow("minimum", "Minimum numeric value", "integer")
	tbl.AddRow("exclusiveMinimum", "Exclusive minimum (can't be used with 'minimum')", "integer")
	tbl.AddRow("maximum", "Maximum numeric value", "integer")
	tbl.AddRow("exclusiveMaximum", "Exclusive maximum (can't be used with 'maximum')", "integer")
	tbl.AddRow("multipleOf", "Value must be multiple of this", "integer")
	tbl.AddRow("additionalProperties", "Allow unknown keys in maps", "true or false")
	tbl.AddRow("anyOf", "Any one of array of schemas", "array")
	tbl.AddRow("oneOf", "Exactly one schema must apply", "array")
	tbl.AddRow("allOf", "All listed schemas must apply", "array")
	tbl.AddRow("minLength", "Minimum string length", "integer (must be ≤ 'maxLength')")
	tbl.AddRow("maxLength", "Maximum string length", "integer (must be ≥ 'minLength')")
	tbl.AddRow("minItems", "Minimum array length", "integer (must be ≤ 'maxItems')")
	tbl.AddRow("maxItems", "Maximum array length", "integer (must be ≥ 'minItems')")

	tbl.Render()

	fmt.Fprintf(&wri, "\n\n")

	return wri.String()
}

func (c *genSchemaCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.jsonSchemaFile, "output", os.Getenv(envOutputDir),
		"generated json schema output dir (optional)")
}

func (c *genSchemaCmd) Execute(_ context.Context, fs *flag.FlagSet, _ ...any) subcommands.ExitStatus {
	if fs.NArg() == 0 {
		c.yamlFile = os.Getenv(envYamlFile)
	} else {
		c.yamlFile = fs.Args()[0]
	}
	if c.yamlFile == "" {
		fmt.Fprintln(os.Stderr, "error: missing input yaml file")
		return subcommands.ExitFailure
	}

	if c.jsonSchemaFile == "" {
		dir := filepath.Dir(c.yamlFile)
		base := filepath.Base(c.yamlFile)
		ext := filepath.Ext(c.yamlFile)
		c.jsonSchemaFile = filepath.Join(dir,
			fmt.Sprintf("%s.schema.json", strings.TrimSuffix(base, ext)))
	}

	err := os.MkdirAll(filepath.Dir(c.jsonSchemaFile), os.ModePerm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: unable to create destination dir: %v\n", err)
		return subcommands.ExitFailure
	}

	content, err := os.ReadFile(c.yamlFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	var values yaml.Node
	err = yaml.Unmarshal(content, &values)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	res := schema.FromYAML(filepath.Dir(c.yamlFile), &values, nil)

	sch, err := res.ToJson()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	err = os.WriteFile(c.jsonSchemaFile, sch, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}
