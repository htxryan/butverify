// Diff rendering for report sections of type=diff. Two input modes:
//
//   1. Unified-diff string (the agent already has `git diff` output) — we
//      parse line-by-line and tag each line with "add", "rem", or context.
//      We DO NOT validate the diff hunks; we just colour the output.
//
//   2. Before/after pair — we run a minimal LCS-free pairing: any line that
//      appears in `after` but not `before` is "add", anything in `before`
//      but not `after` is "rem", common lines are context. This is not a
//      proper diff (it doesn't preserve line order or handle moves) but it's
//      sufficient for "render this code change" without pulling in a full
//      diff library. Agents that want a real unified diff should pre-compute
//      one and pass it via the `unified` field.
//
// The output is a flat []diffLine — the template ranges over it and emits
// one <div class="bv-diff-line {cls}">.
//
// SECURITY NOTE: diffLine.Cls is interpolated by html/template inside an
// already-open class attribute. html/template DOES context-detect that
// position and apply attribute-value escaping, so the value is not
// silently passed through. The closed-set pinning here is defense-in-depth:
// a future refactor that swaps html/template for text/template, or that
// builds the class string outside the template, would silently lose that
// protection. We construct diffLine literals with these constants only —
// there is no code path that lets caller input flow into Cls. If you add
// one, validate against the closed set first.

package templates

import (
	"strings"
)

// Closed set of values for diffLine.Cls. Defense-in-depth pin: html/template
// does escape attribute-position interpolations, but binding to constants
// here keeps the safety property local to this file even if a future
// refactor swaps the template engine or constructs the class fragment
// outside the template.
const (
	diffClassAdd = "add"
	diffClassRem = "rem"
)

func renderDiffLines(s ReportSection) []diffLine {
	if strings.TrimSpace(s.Unified) != "" {
		return parseUnifiedDiff(s.Unified)
	}
	return naiveBeforeAfterDiff(s.Before, s.After)
}

// parseUnifiedDiff scans a unified-diff string and tags lines:
//   - lines starting "+" (but not "+++") → add
//   - lines starting "-" (but not "---") → rem
//   - everything else → context (no class)
//
// Hunk headers (@@) and file headers (+++/---) are emitted as context lines
// so the reader sees the structural context.
func parseUnifiedDiff(s string) []diffLine {
	lines := strings.Split(s, "\n")
	out := make([]diffLine, 0, len(lines))
	for _, ln := range lines {
		// Drop the trailing-empty-after-final-newline that Split produces.
		if ln == "" && len(out) > 0 && out[len(out)-1].Text == "" {
			continue
		}
		switch {
		case strings.HasPrefix(ln, "+++"), strings.HasPrefix(ln, "---"):
			out = append(out, diffLine{Text: ln})
		case strings.HasPrefix(ln, "+"):
			out = append(out, diffLine{Cls: diffClassAdd, Text: ln})
		case strings.HasPrefix(ln, "-"):
			out = append(out, diffLine{Cls: diffClassRem, Text: ln})
		default:
			out = append(out, diffLine{Text: ln})
		}
	}
	return out
}

// naiveBeforeAfterDiff is a deliberately simple "everything in `before` that
// isn't in `after` is removed; everything in `after` that isn't in `before`
// is added; lines in both are context." The output isn't a real diff (no
// line-order preservation across moves) but it's enough to colour-code the
// "what changed" intent for a small snippet.
func naiveBeforeAfterDiff(before, after string) []diffLine {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	afterSet := make(map[string]int, len(afterLines))
	for _, ln := range afterLines {
		afterSet[ln]++
	}
	out := make([]diffLine, 0, len(beforeLines)+len(afterLines))
	// First pass: emit removals in original `before` order. We consume
	// afterSet matches greedily so a line appearing N times in before but
	// M < N times in after produces (N-M) "rem" lines.
	for _, ln := range beforeLines {
		if afterSet[ln] > 0 {
			afterSet[ln]--
			continue
		}
		out = append(out, diffLine{Cls: diffClassRem, Text: "-" + ln})
	}
	// Second pass: emit context/add in original `after` order. We rebuild
	// the matched-set from beforeLines (the first pass consumed afterSet,
	// not beforeLines) so context lines are preserved exactly once per
	// shared occurrence.
	matched := make(map[string]int, len(beforeLines))
	for _, ln := range beforeLines {
		matched[ln]++
	}
	for _, ln := range afterLines {
		if matched[ln] > 0 {
			matched[ln]--
			out = append(out, diffLine{Text: " " + ln})
			continue
		}
		out = append(out, diffLine{Cls: diffClassAdd, Text: "+" + ln})
	}
	return out
}
