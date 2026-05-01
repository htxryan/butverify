package templates

import (
	"regexp"
	"testing"
)

// blanketReducedMotionRe pins the cross-cutting reduced-motion gate.
// Every butverify surface (marketing-site, dashboard,
// templated evidence sites) carries the same rule, so users with system
// reduce-motion never see motion no matter where they land.
var blanketReducedMotionRe = regexp.MustCompile(
	`@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{` +
		`\s*\*\s*,\s*\*::before\s*,\s*\*::after\s*\{` +
		`([^}]+)\}\s*\}`,
)

func TestStylesCSS_BlanketReducedMotionGate(t *testing.T) {
	css, err := assetsFS.ReadFile("assets/styles.css")
	if err != nil {
		t.Fatalf("read embedded styles.css: %v", err)
	}
	m := blanketReducedMotionRe.FindStringSubmatch(string(css))
	if m == nil {
		t.Fatal("templates assets/styles.css missing the blanket @media (prefers-reduced-motion: reduce) rule")
	}
	body := m[1]

	for _, want := range []string{
		`animation:\s*none\s*!important`,
		`transition:\s*none\s*!important`,
		`scroll-behavior:\s*auto\s*!important`,
	} {
		if !regexp.MustCompile(want).MatchString(body) {
			t.Errorf("blanket reduced-motion block missing %q; got body:\n%s", want, body)
		}
	}
}
