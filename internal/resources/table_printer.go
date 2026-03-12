package resources

import (
	"fmt"
	"io"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/klog/v2"
)

// TablePrinter decodes server-side Table objects before delegating the rendered
// result to another printer. Non-table responses flow directly through the
// delegate.
type TablePrinter struct {
	Delegate printers.ResourcePrinter
}

// TablePrinter satisfies the printers.ResourcePrinter interface.
func (t *TablePrinter) PrintObj(obj runtime.Object, writer io.Writer) error {
	table, err := decodeIntoTable(obj)
	if err == nil {
		return t.Delegate.PrintObj(table, writer)
	}

	klog.V(2).Infof("Unable to decode server response into a Table. Falling back to hardcoded renderer: %v", err)
	return t.Delegate.PrintObj(obj, writer)
}

var recognizedTableVersions = map[schema.GroupVersionKind]bool{
	metav1beta1.SchemeGroupVersion.WithKind("Table"): true,
	metav1.SchemeGroupVersion.WithKind("Table"):      true,
}

var (
	defaultTablePrinter printers.ResourcePrinter = &TablePrinter{Delegate: &tableDelegate{}}
)

// TablePrinterForDefaultRenderer returns the shared printer that renders tables
// using the internal renderer defined in tableDelegate.
func TablePrinterForDefaultRenderer() printers.ResourcePrinter {
	return defaultTablePrinter
}

func decodeIntoTable(obj runtime.Object) (runtime.Object, error) {
	event, isEvent := obj.(*metav1.WatchEvent)
	if isEvent {
		obj = event.Object.Object
	}

	if !recognizedTableVersions[obj.GetObjectKind().GroupVersionKind()] {
		return nil, fmt.Errorf("attempt to decode non-Table object")
	}

	unstr, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil, fmt.Errorf("attempt to decode non-Unstructured object")
	}

	table := &metav1.Table{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstr.Object, table); err != nil {
		return nil, err
	}

	for i := range table.Rows {
		row := &table.Rows[i]
		if row.Object.Raw == nil || row.Object.Object != nil {
			continue
		}
		converted, err := runtime.Decode(unstructured.UnstructuredJSONScheme, row.Object.Raw)
		if err != nil {
			return nil, err
		}
		row.Object.Object = converted
	}

	if isEvent {
		event.Object.Object = table
		return event, nil
	}
	return table, nil
}

type tableDelegate struct{}

func (tableDelegate) PrintObj(obj runtime.Object, writer io.Writer) error {
	switch o := obj.(type) {
	case *metav1.Table:
		return renderMetaTable(writer, o)
	case *unstructured.UnstructuredList:
		return renderTable(writer, pointerItems(o.Items))
	case *unstructured.Unstructured:
		return renderTable(writer, []*unstructured.Unstructured{o})
	default:
		return fmt.Errorf("unsupported printer object %T", obj)
	}
}

func renderMetaTable(w io.Writer, tableObj *metav1.Table) error {
	if len(tableObj.ColumnDefinitions) == 0 {
		return renderTable(w, nil)
	}

	// Convert column definitions to strings
	headers := make([]string, len(tableObj.ColumnDefinitions))
	for i, col := range tableObj.ColumnDefinitions {
		headers[i] = strings.ToUpper(col.Name)
	}

	// Convert rows to string slices
	rows := make([][]string, len(tableObj.Rows))
	for i, row := range tableObj.Rows {
		cells := make([]string, len(row.Cells))
		for j, cell := range row.Cells {
			cells[j] = fmt.Sprint(cell)
		}
		rows[i] = cells
	}

	return printTable(w, headers, rows)
}

func printTable(w io.Writer, headers []string, rows [][]string) error {
	if len(headers) == 0 {
		return nil
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print headers
	for i, header := range headers {
		fmt.Fprintf(w, "%-*s", widths[i], header)
		if i < len(headers)-1 {
			fmt.Fprint(w, "  ")
		}
	}
	fmt.Fprint(w, "\n")

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			fmt.Fprintf(w, "%-*s", widths[i], cell)
			if i < len(widths)-1 {
				fmt.Fprint(w, "  ")
			}
		}
		fmt.Fprint(w, "\n")
	}

	return nil
}

func pointerItems(items []unstructured.Unstructured) []*unstructured.Unstructured {
	out := make([]*unstructured.Unstructured, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}
