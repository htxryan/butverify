package templates

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDashboardCSV_InfersTypes(t *testing.T) {
	in := []byte("date,users,note\n2026-04-01,10,first\n2026-04-02,20,second\n")
	cols, rows, err := ParseDashboardCSV(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 || len(rows) != 2 {
		t.Fatalf("cols=%d rows=%d", len(cols), len(rows))
	}
	if cols[0].Kind != colDate {
		t.Errorf("date col kind=%s", cols[0].Kind)
	}
	if cols[1].Kind != colNumber {
		t.Errorf("users col kind=%s", cols[1].Kind)
	}
	if cols[2].Kind != colString {
		t.Errorf("note col kind=%s", cols[2].Kind)
	}
}

func TestParseDashboardCSV_RejectsEmpty(t *testing.T) {
	_, _, err := ParseDashboardCSV(nil)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty error, got %v", err)
	}
}

func TestParseDashboardCSV_RejectsBlankHeader(t *testing.T) {
	_, _, err := ParseDashboardCSV([]byte("a,,c\n1,2,3\n"))
	if err == nil || !strings.Contains(err.Error(), "header column 1 is empty") {
		t.Errorf("expected empty-header error, got %v", err)
	}
}

func TestParseDashboardCSV_TolerantRagged(t *testing.T) {
	in := []byte("a,b,c\n1,2,3\n4,5\n")
	cols, rows, err := ParseDashboardCSV(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: %d", len(rows))
	}
	if rows[1][2] != "" {
		t.Errorf("ragged row should pad: %v", rows[1])
	}
	_ = cols
}

func TestParseDashboardCSV_NumericWithBlanks(t *testing.T) {
	in := []byte("date,score\n2026-04-01,1\n2026-04-02,\n2026-04-03,3\n")
	cols, _, err := ParseDashboardCSV(in)
	if err != nil {
		t.Fatal(err)
	}
	if cols[1].Kind != colNumber {
		t.Errorf("blanks should not disqualify numeric: %s", cols[1].Kind)
	}
	if !math.IsNaN(cols[1].Numeric[1]) {
		t.Errorf("blank should be NaN, got %v", cols[1].Numeric[1])
	}
}

func TestRenderDashboard_HappyPath(t *testing.T) {
	dir := t.TempDir()
	in := []byte("date,users,revenue\n2026-04-01,100,500.50\n2026-04-02,120,600\n2026-04-03,110,550\n")
	g := Generator{Version: "v1", Now: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)}
	rowCount, err := RenderDashboard(in, dir, g, DashboardOptions{Title: "Daily Metrics"})
	if err != nil {
		t.Fatal(err)
	}
	if rowCount != 3 {
		t.Errorf("rowCount: %d", rowCount)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if !strings.Contains(s, "<title>Daily Metrics</title>") {
		t.Errorf("title missing: %s", s[:200])
	}
	if !strings.Contains(s, `<svg`) {
		t.Errorf("expected SVG chart: %s", s[:500])
	}
	// Stat card formatting (latest revenue = 550 → "550")
	if !strings.Contains(s, ">550<") {
		t.Errorf("expected revenue stat 550: %s", s)
	}
	// Data CSV preserved.
	csv, err := os.ReadFile(filepath.Join(dir, "data.csv"))
	if err != nil {
		t.Fatalf("data.csv: %v", err)
	}
	if string(csv) != string(in) {
		t.Errorf("data.csv differs from input")
	}
}

func TestRenderDashboard_EscapesCSVCells(t *testing.T) {
	// HTML escaping: a CSV cell containing "<script>" should NOT land as
	// live HTML in the rendered table.
	dir := t.TempDir()
	in := []byte("name,value\n<script>alert(1)</script>,42\n")
	if _, err := RenderDashboard(in, dir, Generator{}, DashboardOptions{}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if strings.Contains(s, "<script>alert(1)</script>") {
		t.Errorf("CSV cell not escaped: %s", s)
	}
	if !strings.Contains(s, "&lt;script&gt;") {
		t.Errorf("expected escaped: %s", s)
	}
}

func TestRenderDashboard_TruncatesLongTable(t *testing.T) {
	dir := t.TempDir()
	rows := []string{"a,b"}
	for i := 0; i < 50; i++ {
		rows = append(rows, "x,1")
	}
	in := []byte(strings.Join(rows, "\n") + "\n")
	rc, err := RenderDashboard(in, dir, Generator{}, DashboardOptions{MaxTableRows: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rc != 50 {
		t.Errorf("rowCount: %d", rc)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	// "Showing first" is the truncation marker.
	if !strings.Contains(s, "Showing first 10") {
		t.Errorf("missing truncation note: %s", s)
	}
}

func TestRenderDashboard_NoNumericColumns(t *testing.T) {
	// All-string CSV: no charts but still renders the table.
	dir := t.TempDir()
	in := []byte("name,note\nfoo,bar\nbaz,qux\n")
	if _, err := RenderDashboard(in, dir, Generator{}, DashboardOptions{}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if strings.Contains(s, `<svg`) {
		t.Errorf("should have no chart for all-string data: %s", s)
	}
	if !strings.Contains(s, "<td>foo</td>") {
		t.Errorf("table missing: %s", s)
	}
}

func TestRenderDashboard_DeterministicOutput(t *testing.T) {
	in := []byte("date,n\n2026-04-01,1\n2026-04-02,2\n")
	g := Generator{Version: "v1", Now: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)}
	d1 := t.TempDir()
	if _, err := RenderDashboard(in, d1, g, DashboardOptions{}); err != nil {
		t.Fatal(err)
	}
	d2 := t.TempDir()
	if _, err := RenderDashboard(in, d2, g, DashboardOptions{}); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(d1, "index.html"))
	b, _ := os.ReadFile(filepath.Join(d2, "index.html"))
	if string(a) != string(b) {
		t.Errorf("non-deterministic render")
	}
}

func TestRenderDashboard_BundleSizeBudget(t *testing.T) {
	// Fitness function from the epic: bundle <2MB for typical input. We
	// don't actually push 100KB CSVs in tests (slow + noisy) but a 100-row
	// CSV with 5 numeric columns should produce well under 200KB.
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("date,a,b,c,d,e\n")
	for i := 0; i < 100; i++ {
		b.WriteString("2026-04-01,1,2,3,4,5\n")
	}
	if _, err := RenderDashboard([]byte(b.String()), dir, Generator{}, DashboardOptions{MaxTableRows: 25}); err != nil {
		t.Fatal(err)
	}
	var totalSize int64
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		totalSize += info.Size()
		return nil
	})
	if totalSize > 256*1024 {
		t.Errorf("bundle larger than 256KB: %d", totalSize)
	}
}
