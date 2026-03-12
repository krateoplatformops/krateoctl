package resources

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/aquasecurity/table"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

type OutputFormat string

const (
	FormatTable OutputFormat = "table"
	FormatYAML  OutputFormat = "yaml"
	FormatJSON  OutputFormat = "json"
	FormatName  OutputFormat = "name"
)

var (
	tablePrinter = TablePrinterForDefaultRenderer()
)

func NormalizeOutputFormat(requested string, fallback OutputFormat) (OutputFormat, error) {
	value := strings.TrimSpace(strings.ToLower(requested))
	if value == "" {
		return fallback, nil
	}

	switch value {
	case "table", "wide":
		return FormatTable, nil
	case "yaml":
		return FormatYAML, nil
	case "json":
		return FormatJSON, nil
	case "name":
		return FormatName, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", requested)
	}
}

// stripManagedFields removes metadata.managedFields from a deep copy of the
// object so it is never included in YAML/JSON output.
func stripManagedFields(obj map[string]any) map[string]any {
	out := make(map[string]any, len(obj))
	for k, v := range obj {
		out[k] = v
	}
	if meta, ok := out["metadata"].(map[string]any); ok {
		cleanMeta := make(map[string]any, len(meta))
		for k, v := range meta {
			if k == "managedFields" {
				continue
			}
			cleanMeta[k] = v
		}
		out["metadata"] = cleanMeta
	}
	return out
}

func WriteList(w io.Writer, list *unstructured.UnstructuredList, format OutputFormat) error {
	switch format {
	case FormatTable:
		return tablePrinter.PrintObj(list, w)
	case FormatYAML:
		return encodeYAML(w, stripManagedFields(list.Object))
	case FormatJSON:
		return encodeJSON(w, stripManagedFields(list.Object))
	case FormatName:
		for _, item := range list.Items {
			fmt.Fprintf(w, "%s/%s\n", strings.ToLower(item.GetKind()), item.GetName())
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func WriteResource(w io.Writer, obj *unstructured.Unstructured, format OutputFormat) error {
	switch format {
	case FormatTable:
		return tablePrinter.PrintObj(obj, w)
	case FormatYAML:
		return encodeYAML(w, stripManagedFields(obj.Object))
	case FormatJSON:
		return encodeJSON(w, stripManagedFields(obj.Object))
	case FormatName:
		fmt.Fprintf(w, "%s/%s\n", strings.ToLower(obj.GetKind()), obj.GetName())
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func renderTable(w io.Writer, items []*unstructured.Unstructured) error {
	if len(items) == 0 {
		tbl := table.New(w)
		tbl.SetBorders(false)
		tbl.SetHeaders("NAMESPACE", "NAME", "VERSION", "KIND")
		tbl.Render()
		return nil
	}

	objs := make([]*unstructured.Unstructured, len(items))
	copy(objs, items)
	sort.Slice(objs, func(i, j int) bool {
		if objs[i].GetNamespace() == objs[j].GetNamespace() {
			return objs[i].GetName() < objs[j].GetName()
		}
		return objs[i].GetNamespace() < objs[j].GetNamespace()
	})

	tbl := table.New(w)
	tbl.SetBorders(false)
	tbl.SetHeaders("NAMESPACE", "NAME", "VERSION", "KIND")

	for _, item := range objs {
		ns := item.GetNamespace()
		if ns == "" {
			ns = "<cluster>"
		}
		version := item.GetLabels()[compositionVersionLabel]
		tbl.AddRow(ns, item.GetName(), version, item.GetKind())
	}

	tbl.Render()
	return nil
}

func encodeYAML(w io.Writer, payload any) error {
	data, err := yaml.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, string(data))
	return err
}

func encodeJSON(w io.Writer, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprint(w, string(data))
	return err
}
