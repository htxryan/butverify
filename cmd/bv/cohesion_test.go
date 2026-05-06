// Build-time CI assertions for `bv install-skill` (E13-T3).
//
// These tests fail CI when the embedded skill or the runtime command
// surface drift in dangerous ways. They are the SECOND-LEVEL assertion
// over T2's installer self-tests: T2 verifies the install round-trip
// is internally consistent; this file verifies the embedded markdown
// and the runtime CLI surface stay coherent with one another.
//
// Authoritative spec: docs/specs/butverify-skill.md rev 3, EARS:
//
//   - BVS-U-8 (3-part binary cohesion):
//     (a) both `evidence` and `install-skill` subcommands compiled in,
//     (b) both registered such that `bv help` lists them,
//     (c) every `bv evidence …` invocation in the embedded skill
//         markdown round-trips through the runtime evidence flag
//         parser without error.
//   - BVS-N-2 (guardrail substrings): the embedded markdown contains
//     literally `NEVER skirt` AND
//     `NEVER publish proof of unfinished work`.
//   - BVS-U-9 (hash determinism): the build-stamped hash, recomputed
//     against the canonical hash domain, equals what the installer
//     writes onto the installed file.
//
// Mutation cases documented per-test below — each test pins a
// specific drift class so a CI failure names exactly what changed.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// ---------------- BVS-U-8(a): both subcommands compiled in ----------------

// TestBVSU8a_BothSubcommandsCompiledIn confirms that the runtime
// dispatch in main.go references both `runEvidence` and
// `runInstallSkill` by their compile-time identifiers.
//
// The actual cohesion guarantee is provided by the Go compiler: if
// either function were deleted, `go build ./cmd/bv` would fail because
// main.go's switch statement names them. This test is the canary that
// pins the expected wiring so a future refactor that renames one
// handler doesn't silently bypass `bv help`'s list.
//
// Mutation contract: deleting `func runEvidence` in cmd_evidence.go
// or `func runInstallSkill` in cmd_install_skill.go MUST cause
// `go build ./cmd/bv` to fail. This test is compile-time-equivalent
// because the package containing this _test file imports those same
// symbols transitively through main.go.
func TestBVSU8a_BothSubcommandsCompiledIn(t *testing.T) {
	mainPath := mainGoPath(t)
	raw, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(raw)

	cases := []struct {
		name        string
		caseLiteral string // the `case "<cmd>":` that must appear
		handlerName string // the handler the case body must invoke
	}{
		{name: "evidence", caseLiteral: `case "evidence":`, handlerName: "runEvidence"},
		{name: "agent-init", caseLiteral: `case "agent-init":`, handlerName: "runAgentInit"},
		{name: "install-skill", caseLiteral: `case "install-skill":`, handlerName: "runInstallSkill"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(src, tc.caseLiteral) {
				t.Errorf("main.go missing dispatch case %q — `bv %s` would fall through to the unknown-command branch", tc.caseLiteral, tc.name)
			}
			if !strings.Contains(src, tc.handlerName) {
				t.Errorf("main.go does not reference handler %q — the symbol either moved or was deleted", tc.handlerName)
			}
		})
	}
}

// mainGoPath resolves the absolute path of cmd/bv/main.go relative to
// this test file. runtime.Caller is the standard way to discover a
// test's own source location without baking in the working directory.
func mainGoPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "main.go")
}

// ---------------- BVS-U-8(b): help lists both subcommands ----------------

// TestBVSU8b_HelpListsBothSubcommands probes the `usageText` const
// (the exact string `bv` prints for `--help` / `-h` / no-args) for the
// literal subcommand names. Table-driven per the spec.
//
// Mutation contract: removing the `evidence` line or `install-skill`
// line from `usageText` in main.go MUST fail this test. A user who
// runs `bv --help` should never see one of the two listed without the
// other (BVS-U-8(b) requires BOTH).
func TestBVSU8b_HelpListsBothSubcommands(t *testing.T) {
	wantSubcommands := []string{"agent-init", "install-skill", "evidence"}
	for _, name := range wantSubcommands {
		name := name
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(usageText, name) {
				t.Errorf("usageText does not list subcommand %q — `bv --help` will not surface it", name)
			}
		})
	}
}

// ---------------- BVS-U-8(c): embedded skill ↔ evidence flag set ----------------

// extractBvEvidenceInvocations scans markdown for backtick-delimited
// code spans that contain `bv evidence …` and returns the normalized
// (whitespace-collapsed) span text minus the leading `bv evidence`.
// Multi-line backtick spans (e.g. the rule 3 prose where the closing
// backtick lives on a different line than the opening one) are
// supported because `[^\x60]*` in Go's RE2 spans newlines.
//
// The returned slice contains the args portion only — e.g. for the
// span “ `bv evidence --from evidence.json --push --mode remote` “ the result is
// `--from evidence.json --push --mode remote`.
//
// Whitespace inside a span is normalized to single spaces so a span
// that wraps a line break ("bv evidence\n--push") tokenizes the same
// as the inline form.
func extractBvEvidenceInvocations(markdown []byte) []string {
	re := regexp.MustCompile("`([^`]*bv evidence[^`]*)`")
	matches := re.FindAllStringSubmatch(string(markdown), -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		span := strings.Join(strings.Fields(m[1]), " ")
		// Trim the literal `bv evidence` prefix; what remains are the
		// args we feed to the flag set. A span that's exactly
		// "bv evidence" with no args is still a valid invocation
		// (would print usage at runtime), so we keep the empty case.
		args := strings.TrimPrefix(span, "bv evidence")
		args = strings.TrimSpace(args)
		out = append(out, args)
	}
	return out
}

// TestBVSU8c_EmbeddedSkillEvidenceInvocationsRoundTrip extracts every
// `bv evidence …` invocation embedded in the skill markdown and re-
// parses each through the SAME flag set the runtime uses
// (newEvidenceFlagSet). If any invocation references a flag that no
// longer exists, the test fails naming the offending invocation.
//
// Mutation contract:
//
//   - Adding `bv evidence --bogus-flag` to the embedded markdown MUST
//     fail this test (the negative-control subtest below pins this).
//   - Renaming `--push` to `--upload` in cmd_evidence.go without
//     simultaneously editing the markdown MUST fail this test.
func TestBVSU8c_EmbeddedSkillEvidenceInvocationsRoundTrip(t *testing.T) {
	// Both the alias and the prove-it skill teach the evidence workflow;
	// each one must round-trip through the runtime flag parser.
	skills := []struct {
		name string
		body []byte
	}{
		{"butverify (alias)", embeddedSkillBytes},
		{"prove-it", embeddedProveItBytes},
	}
	for _, s := range skills {
		s := s
		t.Run(s.name, func(t *testing.T) {
			invocations := extractBvEvidenceInvocations(s.body)
			if len(invocations) == 0 {
				t.Fatalf("no `bv evidence …` invocations found in embedded %s markdown — the skill no longer teaches the evidence workflow", s.name)
			}
			for i, inv := range invocations {
				i, inv := i, inv
				t.Run("invocation_"+itoa(i), func(t *testing.T) {
					fs, _ := newEvidenceFlagSet()
					args := strings.Fields(inv)
					if err := fs.Parse(args); err != nil {
						t.Errorf("invocation %d (%q) failed to parse against runtime flag set: %v", i, "bv evidence "+inv, err)
					}
				})
			}
		})
	}

	// Mutation probe / negative control: a synthetic invocation with
	// a flag the runtime does not register MUST be rejected by the
	// flag set. This pins the contract that the round-trip above is
	// meaningful (i.e. the parser actually rejects unknown flags
	// rather than silently accepting them).
	t.Run("MutationProbe_RejectsBogusFlag", func(t *testing.T) {
		fs, _ := newEvidenceFlagSet()
		err := fs.Parse([]string{"--bogus-flag"})
		if err == nil {
			t.Fatal("evidence flag set accepted --bogus-flag — the BVS-U-8(c) round-trip would silently pass any drift in the embedded markdown")
		}
	})
}

// itoa is a stdlib-free tiny int-to-string for subtest names. We
// avoid strconv just to keep the per-test deps minimal — the t.Run
// name is purely cosmetic, so the conversion can be naive.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ---------------- BVS-N-2: guardrail substrings ----------------

// TestBVSN2_GuardrailSubstringsPresent pins the two MUST-have
// guardrail phrases in the embedded skill markdown. These phrases are
// the load-bearing safety language the spec requires the agent to
// surface to the user — the entire skill exists to enforce them.
//
// Each phrase is a separate t.Run subtest so a failure names exactly
// which substring is missing.
//
// Mutation contract: removing either substring from
// `bv-skills/claude/butverify.md` (which the build-time mirror
// at `cmd/bv/embedded_skills/claude_butverify.md` byte-equality test
// keeps in sync) MUST fail this test.
func TestBVSN2_GuardrailSubstringsPresent(t *testing.T) {
	required := []struct {
		name      string
		substring string
	}{
		{name: "NEVER_skirt", substring: "NEVER skirt"},
		{name: "NEVER_publish_proof_of_unfinished_work", substring: "NEVER publish proof of unfinished work"},
	}
	// Both the deprecated alias AND the new prove-it skill must carry
	// the load-bearing guardrails. The review skill is a different
	// workflow (acknowledging human feedback) and is exempt.
	skills := []struct {
		name string
		body string
	}{
		{"butverify (alias)", string(embeddedSkillBytes)},
		{"prove-it", string(embeddedProveItBytes)},
	}
	for _, s := range skills {
		s := s
		t.Run(s.name, func(t *testing.T) {
			for _, r := range required {
				r := r
				t.Run(r.name, func(t *testing.T) {
					if !strings.Contains(s.body, r.substring) {
						t.Errorf("embedded %s skill missing required guardrail substring %q (BVS-N-2)", s.name, r.substring)
					}
				})
			}
		})
	}
}

// ---------------- BVS-U-2/U-4/U-5: embedded skill content ACs ----------------

// TestEmbeddedSkillContent_AC2_MultipleCaptureTools pins BVS-U-2: the
// embedded markdown lists multiple capture tools as a non-exhaustive
// menu, and does NOT prescribe a single tool.
//
// Spec §4.2 BVS-U-2; Acceptance Criteria AC-2.
//
// Mutation contract: dropping any of the four exemplar tools, or
// adding prescriptive "must use X" language, MUST fail this test. The
// per-substring subtests give the markdown editor a precise pointer to
// the missing piece.
func TestEmbeddedSkillContent_AC2_MultipleCaptureTools(t *testing.T) {
	body := string(embeddedSkillBytes)

	required := []string{
		"browser MCP",
		"Playwright",
		"OS screenshot",
		"terminal recording",
	}
	for _, sub := range required {
		sub := sub
		t.Run("HasTool_"+sub, func(t *testing.T) {
			if !strings.Contains(body, sub) {
				t.Errorf("embedded skill missing capture-tool exemplar %q (BVS-U-2 / AC-2 / spec §4.2)", sub)
			}
		})
	}

	// Non-exhaustive menu: the spec forbids prescribing a single tool.
	// We pin the absence of common prescriptive phrases. A future
	// markdown edit that introduces "must use Playwright" (or similar)
	// MUST fail this test.
	prescriptive := []string{
		"must use Playwright",
		"must use the browser MCP",
		"required tool",
		"only use",
	}
	lower := strings.ToLower(body)
	for _, phrase := range prescriptive {
		phrase := phrase
		t.Run("NotPrescriptive_"+strings.ReplaceAll(phrase, " ", "_"), func(t *testing.T) {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				t.Errorf("embedded skill contains prescriptive phrase %q which violates BVS-U-2 (non-exhaustive menu, AC-2 / spec §4.2)", phrase)
			}
		})
	}
}

// TestEmbeddedSkillContent_AC4_WorkflowStepsCovered pins BVS-U-4: the
// skill's workflow covers (a) end-to-end exercise, (b) capture, (c)
// evidence.json, (d) `bv evidence --from evidence.json --push --mode remote`, (e)
// surface URL to the human.
//
// Spec §4.2 BVS-U-4; Acceptance Criteria AC-4.
//
// Mutation contract: deleting any of the five steps from the markdown
// MUST fail this test, naming the missing step letter and spec ID.
func TestEmbeddedSkillContent_AC4_WorkflowStepsCovered(t *testing.T) {
	body := string(embeddedSkillBytes)

	cases := []struct {
		name    string
		step    string
		pattern *regexp.Regexp // nil means use substring
		subs    []string       // any-of substring fallback (for nil pattern)
	}{
		{
			name:    "step_a_end_to_end",
			step:    "(a) end-to-end exercise",
			pattern: regexp.MustCompile(`(?i)(run the app end-to-end|exercise.*end-to-end)`),
		},
		{
			name: "step_b_capture",
			step: "(b) capture proof",
			subs: []string{"Capture proof"},
		},
		{
			name: "step_c_evidence_json",
			step: "(c) evidence.json",
			subs: []string{"evidence.json"},
		},
		{
			name: "step_d_bv_evidence_push",
			step: "(d) bv evidence --from evidence.json --push --mode remote",
			subs: []string{"bv evidence --from evidence.json --push --mode remote"},
		},
		{
			name:    "step_e_surface_url",
			step:    "(e) surface URL",
			pattern: regexp.MustCompile(`(?i)(surface the url|surface that url|surface .* url)`),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.pattern != nil {
				if !tc.pattern.MatchString(body) {
					t.Errorf("embedded skill missing workflow step %s (regex %q did not match) (BVS-U-4 / AC-4 / spec §4.2)",
						tc.step, tc.pattern.String())
				}
				return
			}
			for _, sub := range tc.subs {
				if strings.Contains(body, sub) {
					return
				}
			}
			t.Errorf("embedded skill missing workflow step %s (none of %v present) (BVS-U-4 / AC-4 / spec §4.2)",
				tc.step, tc.subs)
		})
	}
}

// TestEmbeddedSkillContent_AC5_NonPrescriptiveLanguage pins BVS-U-5:
// the skill text uses non-prescriptive language for capture tooling
// (e.g. "whatever capture tooling you have").
//
// Spec §4.2 BVS-U-5; Acceptance Criteria AC-5.
//
// Mutation contract: rewriting the capture step to mandate a specific
// tool MUST drop the "whatever" phrasing and fail this test.
func TestEmbeddedSkillContent_AC5_NonPrescriptiveLanguage(t *testing.T) {
	body := strings.ToLower(string(embeddedSkillBytes))
	// Accept either the exact phrasing the spec rationale uses or a
	// close paraphrase. Both forms preserve the "the choice is yours"
	// stance required by BVS-U-5.
	candidates := []string{
		"whatever capture tooling",
		"use whatever capture tooling you have",
	}
	for _, c := range candidates {
		if strings.Contains(body, c) {
			return
		}
	}
	t.Errorf("embedded skill missing non-prescriptive language (none of %v present) (BVS-U-5 / AC-5 / spec §4.2)", candidates)
}

// ---------------- BVS-U-9: hash determinism ----------------

// TestBVSU9_HashDeterminism pins the canonical-hash-domain round-trip
// AND snapshots the current embed's 12-hex hash so a future change to
// the markdown forces the dev to update this test (the right human-
// in-the-loop signal that the skill changed).
//
// Steps:
//
//  1. hash  = skillVersionHash(embeddedSkillBytes)
//  2. stamped = stampVersion(embeddedSkillBytes, hash)
//  3. hash2 = skillVersionHash(stamped)
//  4. assert hash == hash2 (canonical domain ignores the stamp)
//  5. assert hash == wantHash (snapshot — fails on any markdown change)
//
// Mutation contract: any byte-level change to the canonical source
// (bv-skills/claude/butverify.md, mirrored at
// cmd/bv/embedded_skills/claude_butverify.md) that affects the
// canonical hash domain MUST fail this test.
func TestBVSU9_HashDeterminism(t *testing.T) {
	const wantHash = "46a6be62f837"

	hash := skillVersionHash(embeddedSkillBytes)
	if hash == "" {
		t.Fatal("skillVersionHash returned empty string")
	}

	t.Run("CanonicalDomainIgnoresStamp", func(t *testing.T) {
		stamped := stampVersion(embeddedSkillBytes, hash)
		hash2 := skillVersionHash(stamped)
		if hash != hash2 {
			t.Errorf("canonical hash domain not deterministic across stamping: pre-stamp=%q post-stamp=%q", hash, hash2)
		}
	})

	t.Run("SnapshotMatchesEmbed", func(t *testing.T) {
		if hash != wantHash {
			t.Errorf("embedded skill hash drifted: got %q, want %q\n"+
				"If this is intentional (you edited bv-skills/claude/butverify.md\n"+
				"and the mirror), update the wantHash constant in cohesion_test.go.",
				hash, wantHash)
		}
	})
}
