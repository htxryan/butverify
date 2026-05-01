// Server-side SVG chart renderer. We deliberately avoid a JS chart library
// (uPlot/Chart.js etc.) at v1 because:
//   - JS adds bundle weight (~50KB+ for uPlot, ~80KB+ for Chart.js minified)
//   - JS chart libs require a runtime to render, which means a viewer with
//     JS disabled sees nothing
//   - The dashboards we generate are agent evidence, not interactive
//     analytics — fixed-frame line/bar charts are sufficient
//
// The generated SVG is hand-rolled — no external SVG library either —
// because the chart vocabulary at v1 is one (line chart with optional area
// fill) and any library overhead is poor return for that scope.

package templates

import (
	"fmt"
	"math"
	"strings"
)

// Chart canvas dimensions. The viewBox makes the SVG responsive; max-width
// in CSS keeps it from overflowing the section card.
const (
	chartWidth   = 720
	chartHeight  = 240
	chartPadL    = 56 // leave room for y-axis labels
	chartPadR    = 16
	chartPadT    = 16
	chartPadB    = 36 // leave room for x-axis labels
	chartMaxXTks = 8
	chartYTickCt = 4
)

// renderLineChartSVG produces a line chart for one numeric series. xLabels
// are the raw cell strings (used as x-axis tick labels — at most chartMaxXTks
// of them are rendered to avoid overlap). yValues may contain NaN — those
// indices are skipped (the line breaks rather than connecting through gaps).
func renderLineChartSVG(title string, xLabels []string, yValues []float64) string {
	n := len(yValues)
	if n == 0 || len(xLabels) == 0 {
		return emptyChartSVG(title)
	}
	yMin, yMax := minMaxFinite(yValues)
	if math.IsNaN(yMin) || math.IsNaN(yMax) {
		return emptyChartSVG(title)
	}
	// Pad the y range by 5% so the line never touches the top/bottom
	// borders. If min == max (single value or flat series) widen by ±0.5
	// so the line draws mid-frame.
	if yMin == yMax {
		yMin -= 0.5
		yMax += 0.5
	}
	span := yMax - yMin
	yMin -= span * 0.05
	yMax += span * 0.05

	plotW := float64(chartWidth - chartPadL - chartPadR)
	plotH := float64(chartHeight - chartPadT - chartPadB)

	// xAt: linearly map index 0..n-1 to the plot region.
	xAt := func(i int) float64 {
		if n == 1 {
			return float64(chartPadL) + plotW/2
		}
		return float64(chartPadL) + plotW*float64(i)/float64(n-1)
	}
	yAt := func(v float64) float64 {
		t := (v - yMin) / (yMax - yMin)
		return float64(chartPadT) + plotH*(1-t)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="%s line chart" preserveAspectRatio="xMidYMid meet">`,
		chartWidth, chartHeight, escapeAttr(title),
	))

	// Y-axis gridlines + labels.
	for i := 0; i <= chartYTickCt; i++ {
		v := yMin + (yMax-yMin)*float64(i)/float64(chartYTickCt)
		y := yAt(v)
		b.WriteString(fmt.Sprintf(
			`<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="currentColor" stroke-opacity="0.08" stroke-width="1"/>`,
			chartPadL, y, chartWidth-chartPadR, y,
		))
		b.WriteString(fmt.Sprintf(
			`<text x="%d" y="%.1f" font-size="11" fill="currentColor" fill-opacity="0.55" text-anchor="end" dominant-baseline="middle">%s</text>`,
			chartPadL-6, y, escapeText(formatTickValue(v)),
		))
	}

	// X-axis tick labels (subset). Skip empty labels.
	tickStep := n / chartMaxXTks
	if tickStep < 1 {
		tickStep = 1
	}
	for i := 0; i < n; i += tickStep {
		label := strings.TrimSpace(xLabels[i])
		if label == "" {
			continue
		}
		x := xAt(i)
		b.WriteString(fmt.Sprintf(
			`<text x="%.1f" y="%d" font-size="11" fill="currentColor" fill-opacity="0.55" text-anchor="middle">%s</text>`,
			x, chartHeight-chartPadB+18, escapeText(label),
		))
	}
	// Always include the last label as long as it isn't empty (and isn't
	// already drawn by the modulo loop).
	if last := n - 1; last > 0 && last%tickStep != 0 {
		label := strings.TrimSpace(xLabels[last])
		if label != "" {
			b.WriteString(fmt.Sprintf(
				`<text x="%.1f" y="%d" font-size="11" fill="currentColor" fill-opacity="0.55" text-anchor="middle">%s</text>`,
				xAt(last), chartHeight-chartPadB+18, escapeText(label),
			))
		}
	}

	// Build the path. NaN or Inf breaks the line into separate <path>
	// segments — without the Inf skip, yAt(+Inf) emits "-Inf" into the
	// `d` attribute and the SVG path silently fails to render.
	pathOpen := false
	var path strings.Builder
	for i, v := range yValues {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			pathOpen = false
			continue
		}
		x := xAt(i)
		y := yAt(v)
		if !pathOpen {
			path.WriteString(fmt.Sprintf("M%.1f %.1f", x, y))
			pathOpen = true
		} else {
			path.WriteString(fmt.Sprintf(" L%.1f %.1f", x, y))
		}
	}
	b.WriteString(fmt.Sprintf(
		`<path d="%s" fill="none" stroke="#2563eb" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`,
		path.String(),
	))

	// Single-point series: render a dot so the value is visible (the path
	// above produces nothing for a 1-value series).
	if countFinite(yValues) == 1 {
		for i, v := range yValues {
			if !math.IsNaN(v) {
				b.WriteString(fmt.Sprintf(
					`<circle cx="%.1f" cy="%.1f" r="3" fill="#2563eb"/>`,
					xAt(i), yAt(v),
				))
			}
		}
	}

	// Plot border.
	b.WriteString(fmt.Sprintf(
		`<rect x="%d" y="%d" width="%.1f" height="%.1f" fill="none" stroke="currentColor" stroke-opacity="0.15" stroke-width="1"/>`,
		chartPadL, chartPadT, plotW, plotH,
	))
	b.WriteString(`</svg>`)
	return b.String()
}

func emptyChartSVG(title string) string {
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" role="img" aria-label="%s (no data)"><text x="%d" y="%d" font-size="13" fill="currentColor" fill-opacity="0.5" text-anchor="middle" dominant-baseline="middle">No numeric data to chart</text></svg>`,
		chartWidth, chartHeight, escapeAttr(title), chartWidth/2, chartHeight/2,
	)
}

func minMaxFinite(vs []float64) (float64, float64) {
	mn := math.NaN()
	mx := math.NaN()
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if math.IsNaN(mn) || v < mn {
			mn = v
		}
		if math.IsNaN(mx) || v > mx {
			mx = v
		}
	}
	return mn, mx
}

func countFinite(vs []float64) int {
	n := 0
	for _, v := range vs {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			n++
		}
	}
	return n
}

// formatTickValue renders an axis tick label, picking integer vs fractional
// representation based on magnitude.
func formatTickValue(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case abs >= 1e3:
		return fmt.Sprintf("%.1fk", v/1e3)
	case v == math.Trunc(v):
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

// Replacers are package-level so we build them once at process start and
// reuse them across the (potentially thousands of) escape calls a single
// chart render emits.
var (
	textReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrReplacer = strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
)

// escapeText escapes text content destined for SVG text nodes (just like
// HTML — &, <, >).
func escapeText(s string) string {
	return textReplacer.Replace(s)
}

// escapeAttr escapes for an SVG attribute value: the markup-significant
// characters plus quotes. We escape `>` even though it's legal inside a
// double-quoted attribute, because a CSV header like `users > 100` reads
// more naturally rendered literally than as a stray `&gt;` glyph in tooltip
// readers, but on the safety side: belt-and-braces over an attribute we
// emit unparsed.
func escapeAttr(s string) string {
	return attrReplacer.Replace(s)
}
