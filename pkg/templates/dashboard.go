// Dashboard template: renders a CSV-driven summary site with stat cards,
// per-numeric-column charts, and a full data table. The CSV format is
// documented in docs/specs/templates.md.
//
// CSV contract (at v1):
//   - First row is the header (column names; required).
//   - Each subsequent row is a record. We tolerate ragged rows (missing
//     trailing cells become empty strings) so an agent's quick `csv.Writer`
//     output doesn't fail validation if it omits a final empty field.
//   - Column type is inferred per-column by scanning all values:
//       * date    — every non-empty value parses as YYYY-MM-DD or RFC 3339
//       * number  — every non-empty value parses as a Go float64
//       * string  — fallback when neither holds
//   - Categorical "x-axis" detection: we pick the first date column as the
//     x-axis if one exists; otherwise the first column (regardless of type)
//     is used as the x-axis label.
//   - One line chart per numeric (non-x-axis) column.
//
// Empty CSVs (header only, no rows) render with no charts and a "no data"
// note in the table section. Single-row CSVs render charts as a single
// data point (no line — just a stat card and the table).

package templates

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"html/template"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// DashboardTitle defaults are used when the CLI does not supply a `--title`
// override. The deliberate brevity keeps the rendered <title> short.
const (
	DashboardDefaultTitle    = "Dashboard"
	DashboardDefaultSubtitle = ""
)

// MaxDashboardCSVRows caps the row count accepted at parse time. Without it,
// a CSV with millions of rows is fully materialized into [][]string before
// MaxTableRows-style truncation, which would OOM the renderer on inputs that
// fit under the upload-bytes cap but expand on parse. Empirically a 50,000-row
// CSV is already an unusable dashboard — this is a hard parser ceiling, not a
// display knob.
const MaxDashboardCSVRows = 50_000

// DashboardOptions tweaks the rendered output. Zero value is fine.
type DashboardOptions struct {
	// Title overrides the default page title.
	Title string
	// Subtitle is shown beneath the title; empty hides the line.
	Subtitle string
	// MaxTableRows caps how many rows are rendered in the HTML table; the
	// raw data.csv is always written so a viewer can download the full
	// dataset. Zero = no cap (render all rows).
	MaxTableRows int
}

// columnKind enumerates the inferred type of a CSV column. Drives chart
// emission (numeric only) and stat-card formatting.
type columnKind int

const (
	colString columnKind = iota
	colNumber
	colDate
)

func (k columnKind) String() string {
	switch k {
	case colNumber:
		return "number"
	case colDate:
		return "date"
	default:
		return "string"
	}
}

type column struct {
	Name string
	Kind columnKind
	// Numeric values; len == row count for numeric columns. NaN where the
	// row's cell was empty (so charts can skip the gap).
	Numeric []float64
	// Date values; len == row count for date columns; zero-valued where the
	// cell was empty.
	Dates []time.Time
	// String values; always populated regardless of kind so the data table
	// can render the raw cell text without re-formatting.
	Strings []string
}

type dashboardRenderModel struct {
	Title            string
	Subtitle         string
	Stats            []dashboardStat
	Charts           []dashboardChart
	Columns          []string
	Rows             [][]string
	RowCount         int
	RowsShown        int
	Truncated        bool
	GeneratorVersion string
	GeneratedAt      string
}

type dashboardStat struct {
	Label string
	Value string
}

type dashboardChart struct {
	Title string
	SVG   template.HTML // pre-rendered SVG; the template marks it safe via this type
}

// ParseDashboardCSV reads the CSV bytes and returns a typed table. It is
// exposed (not just internal) so the CLI can validate input via `bv
// dashboard --validate` style flows in v1.x.
func ParseDashboardCSV(input []byte) ([]column, [][]string, error) {
	r := csv.NewReader(bytes.NewReader(input))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err == io.EOF {
		return nil, nil, errors.New("templates: dashboard CSV is empty (need at least a header row)")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("templates: dashboard parse header: %w", err)
	}
	if len(header) == 0 {
		return nil, nil, errors.New("templates: dashboard CSV header has no columns")
	}
	for i, h := range header {
		if strings.TrimSpace(h) == "" {
			return nil, nil, fmt.Errorf("templates: dashboard CSV header column %d is empty", i)
		}
	}
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("templates: dashboard parse row %d: %w", len(rows)+1, err)
		}
		if len(rows) >= MaxDashboardCSVRows {
			return nil, nil, fmt.Errorf(
				"templates: dashboard CSV exceeds %d-row parse limit",
				MaxDashboardCSVRows,
			)
		}
		// Pad ragged rows so every row matches header length.
		for len(rec) < len(header) {
			rec = append(rec, "")
		}
		// Truncate over-long rows; in practice csv.Reader with
		// FieldsPerRecord=-1 doesn't emit them shorter than header but a
		// future change to the parser would silently break the table layout
		// if we didn't enforce.
		if len(rec) > len(header) {
			rec = rec[:len(header)]
		}
		rows = append(rows, rec)
	}
	cols := inferColumns(header, rows)
	return cols, rows, nil
}

// inferColumns walks every non-empty value in each column to decide the kind.
// "All non-empty values are dates" → date; else "all non-empty values parse
// as numbers" → number; else string. An empty column (every row blank) is
// classified as string.
func inferColumns(header []string, rows [][]string) []column {
	cols := make([]column, len(header))
	for i, name := range header {
		cols[i].Name = name
		cols[i].Strings = make([]string, len(rows))
		cols[i].Numeric = make([]float64, len(rows))
		cols[i].Dates = make([]time.Time, len(rows))
		anyNonEmpty := false
		allDate := true
		allNumber := true
		for j, row := range rows {
			val := strings.TrimSpace(row[i])
			cols[i].Strings[j] = row[i]
			if val == "" {
				cols[i].Numeric[j] = math.NaN()
				continue
			}
			anyNonEmpty = true
			if t, ok := parseDate(val); ok {
				cols[i].Dates[j] = t
			} else {
				allDate = false
			}
			if n, err := strconv.ParseFloat(val, 64); err == nil {
				cols[i].Numeric[j] = n
			} else {
				cols[i].Numeric[j] = math.NaN()
				allNumber = false
			}
		}
		switch {
		case !anyNonEmpty:
			cols[i].Kind = colString
		case allDate:
			cols[i].Kind = colDate
		case allNumber:
			cols[i].Kind = colNumber
		default:
			cols[i].Kind = colString
		}
	}
	return cols
}

// parseDate accepts YYYY-MM-DD or full RFC 3339; returns the parsed time and
// ok=true on success.
func parseDate(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// xAxis returns the index of the column to use as the chart x-axis (first
// date column; else first column). Returns -1 if no columns at all.
func xAxisIndex(cols []column) int {
	for i, c := range cols {
		if c.Kind == colDate {
			return i
		}
	}
	if len(cols) > 0 {
		return 0
	}
	return -1
}

// buildStats computes summary cards for each numeric column. We pick at most
// 4 columns so a wide CSV doesn't generate a wall of cards. Selection order
// matches column order; the stat shown is the LAST non-NaN value (i.e. "the
// most recent number"), which is the most useful summary for time-series
// agent output.
func buildStats(cols []column, xIdx int) []dashboardStat {
	var stats []dashboardStat
	for i, c := range cols {
		if i == xIdx || c.Kind != colNumber {
			continue
		}
		latest := math.NaN()
		for _, v := range c.Numeric {
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				latest = v
			}
		}
		if math.IsNaN(latest) {
			continue
		}
		stats = append(stats, dashboardStat{
			Label: c.Name,
			Value: formatStatValue(latest),
		})
		if len(stats) >= 4 {
			break
		}
	}
	return stats
}

func formatStatValue(v float64) string {
	// Defense-in-depth: buildStats already filters NaN/Inf, but a future
	// caller routing here directly would otherwise hit `int64(+Inf)` (which
	// is implementation-defined and wraps to MIN/MAX_INT64) inside
	// formatThousands. Surface a literal infinity glyph instead.
	if math.IsNaN(v) {
		return "—"
	}
	if math.IsInf(v, 1) {
		return "∞"
	}
	if math.IsInf(v, -1) {
		return "-∞"
	}
	if math.Abs(v) >= 10000 {
		return formatThousands(v)
	}
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func formatThousands(v float64) string {
	// Use a simple group-by-3 inserter on the integer portion. Locale-free
	// (always ',') because the rendered output is meant to be globally
	// readable. v1.x can pluralize per --locale.
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	int64Part := int64(v)
	frac := v - float64(int64Part)
	s := strconv.FormatInt(int64Part, 10)
	var b strings.Builder
	b.WriteString(sign)
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac >= 0.005 {
		fracStr := strconv.FormatFloat(frac, 'f', 2, 64)
		b.WriteString(fracStr[1:]) // skip leading "0"
	}
	return b.String()
}

// buildCharts emits one chart per numeric column (excluding the x-axis col).
// We cap at 8 charts to keep bundle size predictable.
func buildCharts(cols []column, xIdx int) []dashboardChart {
	var charts []dashboardChart
	if xIdx < 0 || len(cols) == 0 {
		return charts
	}
	x := cols[xIdx]
	xLabels := make([]string, len(x.Strings))
	copy(xLabels, x.Strings)
	for i, c := range cols {
		if i == xIdx || c.Kind != colNumber {
			continue
		}
		svg := renderLineChartSVG(c.Name, xLabels, c.Numeric)
		charts = append(charts, dashboardChart{
			Title: c.Name,
			SVG:   template.HTML(svg), //nolint:gosec — rendered server-side from typed numeric input
		})
		if len(charts) >= 8 {
			break
		}
	}
	return charts
}

// RenderDashboard reads CSV bytes and writes the rendered site files into
// outDir:
//
//	outDir/
//	  index.html
//	  data.csv
//	  assets/
//	    styles.css
//
// The original CSV is copied so a viewer can download the full dataset (the
// rendered table may be capped via DashboardOptions.MaxTableRows).
func RenderDashboard(input []byte, outDir string, g Generator, opts DashboardOptions) (int, error) {
	cols, rows, err := ParseDashboardCSV(input)
	if err != nil {
		return 0, err
	}

	maxRows := opts.MaxTableRows
	rowsShown := len(rows)
	truncated := false
	displayRows := rows
	if maxRows > 0 && len(rows) > maxRows {
		displayRows = rows[:maxRows]
		rowsShown = maxRows
		truncated = true
	}

	xIdx := xAxisIndex(cols)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Name
	}
	model := dashboardRenderModel{
		Title:            firstNonEmpty(opts.Title, DashboardDefaultTitle),
		Subtitle:         firstNonEmpty(opts.Subtitle, DashboardDefaultSubtitle),
		Stats:            buildStats(cols, xIdx),
		Charts:           buildCharts(cols, xIdx),
		Columns:          header,
		Rows:             displayRows,
		RowCount:         len(rows),
		RowsShown:        rowsShown,
		Truncated:        truncated,
		GeneratorVersion: g.version(),
		GeneratedAt:      g.generatedAt(),
	}

	var html bytes.Buffer
	if err := dashboardTmpl.Execute(&html, model); err != nil {
		return 0, fmt.Errorf("templates: render dashboard: %w", err)
	}
	if err := writeFile(outDir, "index.html", html.Bytes()); err != nil {
		return 0, err
	}
	if err := writeFile(outDir, "data.csv", input); err != nil {
		return 0, err
	}
	if err := writeAsset(outDir, "assets/styles.css"); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
