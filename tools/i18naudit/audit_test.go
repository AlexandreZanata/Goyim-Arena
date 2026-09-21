// Tests of the localization scanner (P18-T10). Two properties have to hold,
// and a test that only had the first would be worthless:
//
//   - the rules reject a document that hardcodes its language or its prose,
//     and a stylesheet that is physical — the fixtures are the falsification,
//     one per rule;
//   - the delivered tree passes all of them.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile writes one probe into a temporary tree.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// auditFixture scans one fixture directory.
func auditFixture(t *testing.T, name string) []Finding {
	t.Helper()

	findings, err := Audit(filepath.Join("fixtures", name), Options{})
	if err != nil {
		t.Fatalf("Audit(fixtures/%s) error = %v", name, err)
	}
	return findings
}

// rulesOf collects the rules a scan reported, so a test asserts what broke and
// not only how many things did.
func rulesOf(findings []Finding) map[Rule]int {
	counts := map[Rule]int{}
	for _, finding := range findings {
		counts[finding.Rule]++
	}
	return counts
}

// TestScannerRejectsTheHardcodedFixture is the failing direction: a document
// with a literal language and its own sentences, and a sheet with physical
// properties. Every rule of the scanner has to fire, because a rule that never
// fires is a rule that never proved anything.
func TestScannerRejectsTheHardcodedFixture(t *testing.T) {
	t.Parallel()

	findings := auditFixture(t, "hardcoded")
	if len(findings) == 0 {
		t.Fatal("the hardcoded fixture was accepted")
	}

	counts := rulesOf(findings)
	if counts[RuleLanguage] == 0 {
		t.Errorf("no language violation reported for the hardcoded fixture: %v", findings)
	}
	if counts[RuleProse] < 2 {
		t.Errorf("expected the two prose text nodes to be reported, got %v", findings)
	}
	if counts[RuleDirection] < 5 {
		t.Errorf("expected the five physical declarations to be reported, got %v", findings)
	}
	for _, finding := range findings {
		if finding.Line < 2 {
			t.Errorf("finding %q is anchored on line %d, want the line inside the document", finding, finding.Line)
		}
	}
}

// TestScannerAcceptsTheLocalizedFixture is the passing direction: the same
// document with catalog actions, and the same sheet written logically.
func TestScannerAcceptsTheLocalizedFixture(t *testing.T) {
	t.Parallel()

	if findings := auditFixture(t, "localized"); len(findings) != 0 {
		t.Fatalf("the localized fixture was rejected: %v", findings)
	}
}

// TestScannerRejectsALanguageThatDisagreesWithItsDirection covers the half of
// the language rule the hardcoded fixture cannot show: the attributes are
// actions, but they print two different fields — a page that says it is in one
// locale and paints itself in the direction of another.
func TestScannerRejectsALanguageThatDisagreesWithItsDirection(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	document := "package probe\n\nconst probe = `<!DOCTYPE html>\n<html lang=\"{{.Lang}}\" dir=\"{{dir .ContentLanguage}}\">\n<body>{{.Heading}}</body>\n</html>`\n"
	writeFile(t, filepath.Join(directory, "probe.go"), document)

	findings, err := Audit(directory, Options{})
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	counts := rulesOf(findings)
	if counts[RuleLanguage] != 1 {
		t.Fatalf("expected one language violation, got %v", findings)
	}
	if detail := findings[0].Detail; !strings.Contains(detail, "ContentLanguage") {
		t.Errorf("the finding does not name the disagreeing field: %q", detail)
	}
}

// TestScannerRejectsADocumentWithoutDirection keeps the omission case visible:
// an action for the language is not enough if the direction is missing.
func TestScannerRejectsADocumentWithoutDirection(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	document := "package probe\n\nconst probe = `<!DOCTYPE html>\n<html lang=\"{{.Lang}}\">\n<body>{{.Heading}}</body>\n</html>`\n"
	writeFile(t, filepath.Join(directory, "probe.go"), document)

	findings, err := Audit(directory, Options{})
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	if counts := rulesOf(findings); counts[RuleLanguage] != 1 {
		t.Fatalf("expected one language violation, got %v", findings)
	} else if detail := findings[0].Detail; !strings.Contains(detail, "no direction") {
		t.Errorf("the finding does not report the missing direction: %q", detail)
	}
}

// TestScannerReadsTheDeliveredTree is the gate that runs with the unit suite:
// the tree this commit ships must hold no violation of any rule. It is the
// same scan `make audit-i18n` runs, so a document that slips into an adapter
// fails here too.
func TestScannerReadsTheDeliveredTree(t *testing.T) {
	t.Parallel()

	findings, err := Audit(filepath.Join("..", ".."), Options{
		Skip: []string{"tools/i18naudit/fixtures"},
	})
	if err != nil {
		t.Fatalf("Audit(repository) error = %v", err)
	}
	for _, finding := range findings {
		t.Errorf("violation: %s", finding)
	}
}
