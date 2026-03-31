package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/aquasecurity/table"
	"github.com/krateoplatformops/krateoctl/internal/diff"
	"github.com/krateoplatformops/krateoctl/internal/install/state"
	"github.com/krateoplatformops/krateoctl/internal/ui"
	"github.com/krateoplatformops/krateoctl/internal/workflows/types"
)

type diffStepSummary struct {
	ID   string         `json:"id"`
	Type string         `json:"type"`
	Skip bool           `json:"skip,omitempty"`
	With map[string]any `json:"with,omitempty"`
}

type diffRow struct {
	ID      string
	Status  string
	Summary string
}

func (c *planCmd) renderDiff(l *ui.Logger, w io.Writer, leftLabel string, leftBytes []byte, rightLabel string, rightBytes []byte, left any, right any) error {
	format, err := normalizeDiffFormat(c.diffFormat)
	if err != nil {
		return err
	}

	switch format {
	case "table":
		leftSummaries, err := summarizeDiffSteps(left)
		if err != nil {
			return err
		}

		rightSummaries, err := summarizeDiffSteps(right)
		if err != nil {
			return err
		}

		rows, changed := buildDiffRows(leftSummaries, rightSummaries)
		rows = filterChangedRows(rows)
		if !changed || len(rows) == 0 {
			l.Info("✓ Computed plan matches %s", leftLabel)
			return nil
		}

		l.Warn("⚠️  Step diff summary:")
		return renderDiffTable(w, rows)
	default:
		delta := diff.Diff(leftLabel, leftBytes, rightLabel, rightBytes)
		if len(delta) == 0 {
			l.Info("✓ Computed plan matches %s", leftLabel)
			return nil
		}

		l.Warn("⚠️  Differences vs %s:\n%s", leftLabel, diff.Colorize(delta))
		return nil
	}
}

func filterChangedRows(rows []diffRow) []diffRow {
	if len(rows) == 0 {
		return nil
	}

	out := make([]diffRow, 0, len(rows))
	for _, row := range rows {
		if row.Status == "unchanged" {
			continue
		}
		out = append(out, row)
	}
	return out
}

func normalizeDiffFormat(value string) (string, error) {
	format := strings.TrimSpace(strings.ToLower(value))
	if format == "" {
		format = "unified"
	}

	switch format {
	case "unified", "table":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported diff format %q", value)
	}
}

func summarizeDiffSteps(value any) ([]diffStepSummary, error) {
	switch v := value.(type) {
	case []*types.Step:
		out := make([]diffStepSummary, 0, len(v))
		for _, step := range v {
			out = append(out, summarizeTypedStep(step))
		}
		return out, nil
	case []map[string]any:
		out := make([]diffStepSummary, 0, len(v))
		for _, step := range v {
			out = append(out, summarizeMapStep(step))
		}
		return out, nil
	case *state.Snapshot:
		if v == nil {
			return nil, nil
		}
		out := make([]diffStepSummary, 0, len(v.Steps))
		for _, step := range v.Steps {
			out = append(out, summarizeMapStep(step))
		}
		return out, nil
	case state.Snapshot:
		out := make([]diffStepSummary, 0, len(v.Steps))
		for _, step := range v.Steps {
			out = append(out, summarizeMapStep(step))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported diff input %T", value)
	}
}

func summarizeTypedStep(step *types.Step) diffStepSummary {
	if step == nil {
		return diffStepSummary{}
	}

	summary := diffStepSummary{
		ID:   step.ID,
		Type: string(step.Type),
		Skip: step.Skip,
	}
	if step.With != nil {
		summary.With = cloneStringMap(*step.With)
	}
	return summary
}

func summarizeMapStep(step map[string]any) diffStepSummary {
	if step == nil {
		return diffStepSummary{}
	}

	summary := diffStepSummary{}
	if id, ok := step["id"].(string); ok {
		summary.ID = id
	}
	if typ, ok := step["type"].(string); ok {
		summary.Type = typ
	}
	if skip, ok := step["skip"].(bool); ok {
		summary.Skip = skip
	}
	if with := asStringMap(step["with"]); with != nil {
		summary.With = with
	}
	return summary
}

func asStringMap(value any) map[string]any {
	switch v := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return cloneStringMap(v)
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[fmt.Sprint(key)] = normalizeValue(val)
		}
		return out
	default:
		return nil
	}
}

func cloneStringMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = normalizeValue(v)
	}
	return out
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneStringMap(v)
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[fmt.Sprint(key)] = normalizeValue(val)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = normalizeValue(v[i])
		}
		return out
	case []map[string]any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = cloneStringMap(v[i])
		}
		return out
	default:
		return v
	}
}

func buildDiffRows(left, right []diffStepSummary) ([]diffRow, bool) {
	leftByID := make(map[string]diffStepSummary, len(left))
	rightByID := make(map[string]diffStepSummary, len(right))
	leftIndex := make(map[string]int, len(left))
	rightIndex := make(map[string]int, len(right))
	order := make([]string, 0, len(left)+len(right))
	seen := make(map[string]bool, len(left)+len(right))

	for i, step := range left {
		id := step.ID
		if id == "" {
			id = "<missing>"
		}
		if _, ok := leftByID[id]; !ok {
			leftByID[id] = step
			leftIndex[id] = i
		}
		if !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}

	for i, step := range right {
		id := step.ID
		if id == "" {
			id = "<missing>"
		}
		if _, ok := rightByID[id]; !ok {
			rightByID[id] = step
			rightIndex[id] = i
		}
		if !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}

	rows := make([]diffRow, 0, len(order))
	changed := false

	for _, id := range order {
		leftStep, leftOK := leftByID[id]
		rightStep, rightOK := rightByID[id]

		row := diffRow{ID: id}
		switch {
		case leftOK && rightOK:
			leftDigest, _ := canonicalDigest(leftStep)
			rightDigest, _ := canonicalDigest(rightStep)
			if leftDigest == rightDigest {
				if leftIndex[id] == rightIndex[id] {
					row.Status = "unchanged"
				} else {
					row.Status = "moved"
					changed = true
				}
			} else {
				row.Status = "modified"
				changed = true
			}
			row.Summary = summarizeStepDiff(leftStep, rightStep, row.Status)
		case leftOK:
			row.Status = "removed"
			row.Summary = "step removed from the original plan"
			changed = true
		default:
			row.Status = "added"
			row.Summary = "new step added in the computed plan"
			changed = true
		}

		rows = append(rows, row)
	}

	return rows, changed
}

func canonicalDigest(step diffStepSummary) (string, error) {
	data, err := json.Marshal(step)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func compactValue(value any, depth int) string {
	const maxLen = 120

	var out string
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		limit := len(keys)
		if limit > 4 {
			limit = 4
		}

		parts := make([]string, 0, limit+1)
		for i := 0; i < limit; i++ {
			key := keys[i]
			parts = append(parts, fmt.Sprintf("%s=%s", key, compactValue(v[key], depth+1)))
		}
		if len(keys) > limit {
			parts = append(parts, "...")
		}
		out = "{" + strings.Join(parts, ", ") + "}"
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		limit := len(v)
		if limit > 3 {
			limit = 3
		}

		parts := make([]string, 0, limit+1)
		for i := 0; i < limit; i++ {
			parts = append(parts, compactValue(v[i], depth+1))
		}
		if len(v) > limit {
			parts = append(parts, "...")
		}
		out = "[" + strings.Join(parts, ", ") + "]"
	case []string:
		if len(v) == 0 {
			return "[]"
		}
		limit := len(v)
		if limit > 3 {
			limit = 3
		}

		parts := make([]string, 0, limit+1)
		for i := 0; i < limit; i++ {
			parts = append(parts, compactValue(v[i], depth+1))
		}
		if len(v) > limit {
			parts = append(parts, "...")
		}
		out = "[" + strings.Join(parts, ", ") + "]"
	case string:
		out = v
	case fmt.Stringer:
		out = v.String()
	default:
		out = fmt.Sprint(v)
	}

	out = strings.TrimSpace(out)
	if len(out) > maxLen {
		out = out[:maxLen-3] + "..."
	}
	return out
}

func summarizeStepDiff(left, right diffStepSummary, status string) string {
	switch status {
	case "moved":
		return "same step, but its position changed"
	case "modified":
		parts := make([]string, 0, 4)
		if left.Type != right.Type && (left.Type != "" || right.Type != "") {
			parts = append(parts, fmt.Sprintf("type changed %s -> %s", quoteOrDash(left.Type), quoteOrDash(right.Type)))
		}
		if left.Skip != right.Skip {
			if right.Skip {
				parts = append(parts, "skip enabled")
			} else {
				parts = append(parts, "skip disabled")
			}
		}
		if changes := diffValues("with", left.With, right.With, 0); len(changes) > 0 {
			parts = append(parts, changes...)
		}
		if len(parts) == 0 {
			return "content changed"
		}
		if len(parts) > 3 {
			parts = append(parts[:3], fmt.Sprintf("+%d more changes", len(parts)-3))
		}
		return strings.Join(parts, "; ")
	default:
		return "changed"
	}
}

func diffValues(path string, left, right any, depth int) []string {
	if depth > 2 {
		if reflect.DeepEqual(left, right) {
			return nil
		}
		return []string{fmt.Sprintf("%s changed", path)}
	}

	if reflect.DeepEqual(left, right) {
		return nil
	}

	leftMap, leftIsMap := toStringMap(left)
	rightMap, rightIsMap := toStringMap(right)
	if leftIsMap || rightIsMap {
		if !leftIsMap {
			return []string{fmt.Sprintf("%s added %s", path, compactValue(right, depth))}
		}
		if !rightIsMap {
			return []string{fmt.Sprintf("%s removed %s", path, compactValue(left, depth))}
		}

		keys := make([]string, 0, len(leftMap)+len(rightMap))
		seen := make(map[string]bool, len(leftMap)+len(rightMap))
		for k := range leftMap {
			if !seen[k] {
				keys = append(keys, k)
				seen[k] = true
			}
		}
		for k := range rightMap {
			if !seen[k] {
				keys = append(keys, k)
				seen[k] = true
			}
		}
		sort.Strings(keys)

		changes := make([]string, 0, 4)
		for _, key := range keys {
			childPath := path + "." + key
			childLeft, lOk := leftMap[key]
			childRight, rOk := rightMap[key]
			switch {
			case lOk && rOk:
				changes = append(changes, diffValues(childPath, childLeft, childRight, depth+1)...)
			case lOk:
				changes = append(changes, fmt.Sprintf("%s removed %s", childPath, compactValue(childLeft, depth+1)))
			default:
				changes = append(changes, fmt.Sprintf("%s added %s", childPath, compactValue(childRight, depth+1)))
			}
			if len(changes) >= 4 {
				if len(keys) > 4 {
					changes = append(changes[:4], fmt.Sprintf("+%d more changes", len(keys)-4))
				}
				break
			}
		}
		return changes
	}

	leftList, leftIsList := toAnySlice(left)
	rightList, rightIsList := toAnySlice(right)
	if leftIsList || rightIsList {
		if !leftIsList {
			return []string{fmt.Sprintf("%s added %s", path, compactValue(right, depth))}
		}
		if !rightIsList {
			return []string{fmt.Sprintf("%s removed %s", path, compactValue(left, depth))}
		}
		if reflect.DeepEqual(leftList, rightList) {
			return nil
		}
		return []string{fmt.Sprintf("%s changed %s -> %s", path, compactValue(left, depth), compactValue(right, depth))}
	}

	return []string{fmt.Sprintf("%s changed %s -> %s", path, compactValue(left, depth), compactValue(right, depth))}
}

func toStringMap(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[fmt.Sprint(key)] = normalizeValue(val)
		}
		return out, true
	default:
		return nil, false
	}
}

func toAnySlice(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func quoteOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return compactValue(value, 0)
}

func renderDiffTable(w io.Writer, rows []diffRow) error {
	tbl := table.New(w)
	tbl.SetBorders(false)
	tbl.SetHeaders("STEP", "CHANGE", "SUMMARY")

	for _, row := range rows {
		tbl.AddRow(row.ID, row.Status, row.Summary)
	}

	tbl.Render()
	return nil
}
