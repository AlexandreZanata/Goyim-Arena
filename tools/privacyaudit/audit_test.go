package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repositoryRoot is the tree the register describes. The rules that resolve
// what the document cites — every evidence path, every allowlist file — read
// from here, and a mutation that removes a citation has to be judged against the
// tree the document actually talks about.
const repositoryRoot = "../.."

// document returns the delivered register, parsed. The whole suite is built on
// the document that ships: a mutation the reader of the real document could not
// have made is a mutation that tests something else.
func document(t *testing.T) Document {
	t.Helper()
	parsed, err := readDocument(filepath.Join(repositoryRoot, documentPath))
	if err != nil {
		t.Fatalf("reading the delivered register: %v", err)
	}
	return parsed
}

// auditIn writes one register to a temporary document and judges it against the
// real tree. The seam is the document path and not the root on purpose: the
// rules that resolve evidence have to see the tree they describe, and a mock
// tree would answer for the tool instead of the repository.
func auditIn(t *testing.T, register Register, prose string) []Violation {
	t.Helper()
	raw, err := json.MarshalIndent(register, "", "  ")
	if err != nil {
		t.Fatalf("rendering the mutated register: %v", err)
	}
	path := filepath.Join(t.TempDir(), "PRIVACY_AUDIT.md")
	body := prose + registerFence + "\n" + string(raw) + "\n```\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the mutated register: %v", err)
	}
	report, err := Audit(Options{Root: repositoryRoot, Document: path})
	if err != nil {
		t.Fatalf("auditing the mutated register: %v", err)
	}
	return report.Violations
}

// auditText judges a register whose *text* is the subject, which is how a
// document that carries a real address or loses its prose is falsified: those
// rules read the document, not the parsed block.
func auditText(t *testing.T, body string) []Violation {
	t.Helper()
	path := filepath.Join(t.TempDir(), "PRIVACY_AUDIT.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the mutated document: %v", err)
	}
	report, err := Audit(Options{Root: repositoryRoot, Document: path})
	if err != nil {
		t.Fatalf("auditing the mutated document: %v", err)
	}
	return report.Violations
}

// mutated returns the delivered register with one change applied. The change
// reports whether it found what it was looking for: a mutation that silently
// did nothing would make its rule look enforced when nothing was exercised.
func mutated(t *testing.T, apply func(*Register) bool) Register {
	t.Helper()
	register := document(t).Register
	if !apply(&register) {
		t.Fatal("the mutation found nothing to change: the register no longer has the shape the test assumes")
	}
	return register
}

// rulesOf lists the rules that fired, so a failure names them.
func rulesOf(violations []Violation) []string {
	names := make([]string, 0, len(violations))
	for _, violation := range violations {
		names = append(names, violation.Rule)
	}
	return names
}

func hasRule(violations []Violation, rule string) bool {
	return contains(rulesOf(violations), rule)
}

// TestTheDeliveredRegisterIsClean is the control. Without it, every mutation
// below could be passing because the rules refuse everything.
func TestTheDeliveredRegisterIsClean(t *testing.T) {
	violations := auditIn(t, document(t).Register, document(t).Prose)
	if len(violations) > 0 {
		t.Fatalf("the delivered register is refused: %s", violations)
	}
}

// findAllowlist returns the allowlist with an identifier, so a mutation can
// address one surface instead of the first.
func findAllowlist(register *Register, id string) *Allowlist {
	for index := range register.Allowlists {
		if register.Allowlists[index].ID == id {
			return &register.Allowlists[index]
		}
	}
	return nil
}

// findArea returns the area with a key.
func findArea(register *Register, key string) *Area {
	for index := range register.Areas {
		if register.Areas[index].Key == key {
			return &register.Areas[index]
		}
	}
	return nil
}

// findFinding returns the finding with an identifier.
func findFinding(register *Register, id string) *Finding {
	for index := range register.Findings {
		if register.Findings[index].ID == id {
			return &register.Findings[index]
		}
	}
	return nil
}

// findRetention returns the retention row of a class.
func findRetention(register *Register, class string) *Retention {
	for index := range register.Retention {
		if register.Retention[index].Class == class {
			return &register.Retention[index]
		}
	}
	return nil
}

// TestOneMutationPerRule is the falsification: one change per rule, each one
// the change a reviewer could plausibly make, and each one refused by the rule
// it belongs to. A rule without a mutation is a rule that might never fire.
func TestOneMutationPerRule(t *testing.T) {
	prose := document(t).Prose
	cases := []struct {
		name   string
		mutate func(*Register) bool
		rule   string
	}{
		{
			name: "schema version from another schema",
			mutate: func(register *Register) bool {
				register.Version = 2
				return true
			},
			rule: "schema-version",
		},
		{
			name: "a date no reader can place",
			mutate: func(register *Register) bool {
				register.AuditedOn = "22/09/2026"
				return true
			},
			rule: "audited-on",
		},
		{
			name: "one account instead of two",
			mutate: func(register *Register) bool {
				register.Accounts = register.Accounts[:1]
				return true
			},
			rule: "accounts-count",
		},
		{
			name: "two accounts with the same role",
			mutate: func(register *Register) bool {
				register.Accounts[1].ID = register.Accounts[0].ID
				return true
			},
			rule: "accounts-id",
		},
		{
			name: "an account in a real domain",
			mutate: func(register *Register) bool {
				register.Accounts[1].Email = "someone@example.invalid-domain.net"
				return true
			},
			rule: "accounts-synthetic",
		},
		{
			name: "the two accounts are the same subject",
			mutate: func(register *Register) bool {
				register.Accounts[1].Email = register.Accounts[0].Email
				return true
			},
			rule: "accounts-distinct",
		},
		{
			name: "no execution looks for mixing",
			mutate: func(register *Register) bool {
				register.Mixing.Path = ""
				return true
			},
			rule: "mixing-path",
		},
		{
			name: "the mixing evidence does not exist",
			mutate: func(register *Register) bool {
				register.Mixing.Path = "internal/profiles/adapters/http/nowhere_test.go"
				return true
			},
			rule: "mixing-path",
		},
		{
			name: "the mixing evidence names one side only",
			mutate: func(register *Register) bool {
				register.Accounts[1].Email = "export-nobody@arena.example.com"
				return true
			},
			rule: "mixing-evidence",
		},
		{
			name: "two allowlists with the same name",
			mutate: func(register *Register) bool {
				register.Allowlists[1].ID = register.Allowlists[0].ID
				return true
			},
			rule: "allowlist-id",
		},
		{
			name: "a surface that is neither public nor the subject's",
			mutate: func(register *Register) bool {
				register.Allowlists[1].Surface = "internal"
				return true
			},
			rule: "allowlist-surface",
		},
		{
			name: "an allowlist of a file that is not there",
			mutate: func(register *Register) bool {
				findAllowlist(register, "public-export").Path = "internal/transparency/export.go"
				return true
			},
			rule: "allowlist-path",
		},
		{
			name: "an allowlist that declares nothing",
			mutate: func(register *Register) bool {
				findAllowlist(register, "public-export").Keys = nil
				return true
			},
			rule: "allowlist-empty",
		},
		{
			name: "keys in an order two runs cannot compare",
			mutate: func(register *Register) bool {
				entry := findAllowlist(register, "public-export")
				entry.Keys[0], entry.Keys[1] = entry.Keys[1], entry.Keys[0]
				return true
			},
			rule: "allowlist-order",
		},
		{
			name: "the code gained a key nobody declared",
			mutate: func(register *Register) bool {
				entry := findAllowlist(register, "public-export")
				entry.Keys = entry.Keys[1:]
				return true
			},
			rule: "allowlist-undeclared",
		},
		{
			name: "the register declares a key the code cannot emit",
			mutate: func(register *Register) bool {
				entry := findAllowlist(register, "personal-export")
				entry.Keys = append(entry.Keys, "z_provider")
				return true
			},
			rule: "allowlist-unused",
		},
		{
			name: "the public surface is declared as the subject's own document",
			mutate: func(register *Register) bool {
				entry := findAllowlist(register, "public-export")
				personal := findAllowlist(register, "personal-export")
				entry.Path = personal.Path
				entry.Keys = append([]string(nil), personal.Keys...)
				return true
			},
			rule: "allowlist-forbidden",
		},
		{
			name: "the threshold drifts from the aggregates",
			mutate: func(register *Register) bool {
				register.LowCount.Threshold = 4
				return true
			},
			rule: "low-count-threshold",
		},
		{
			name: "a threshold that suppresses nothing",
			mutate: func(register *Register) bool {
				register.LowCount.Threshold = 1
				return true
			},
			rule: "low-count-floor",
		},
		{
			name: "a class published twice",
			mutate: func(register *Register) bool {
				register.Retention = append(register.Retention, register.Retention[0])
				return true
			},
			rule: "retention-duplicate",
		},
		{
			name: "the policy governs a class the register omits",
			mutate: func(register *Register) bool {
				register.Retention = register.Retention[1:]
				return true
			},
			rule: "retention-missing",
		},
		{
			name: "the table publishes the wrong action",
			mutate: func(register *Register) bool {
				findRetention(register, "abuse_signals").Action = "purge"
				return true
			},
			rule: "retention-action",
		},
		{
			name: "a class kept under an obligation looks temporary",
			mutate: func(register *Register) bool {
				findRetention(register, "billing").Indefinite = false
				return true
			},
			rule: "retention-indefinite",
		},
		{
			name: "a window that drifted from the policy",
			mutate: func(register *Register) bool {
				findRetention(register, "sessions").WindowHours = 24
				return true
			},
			rule: "retention-window",
		},
		{
			name: "a reason code that drifted from the policy",
			mutate: func(register *Register) bool {
				findRetention(register, "tokens").ReasonCode = "cleanup"
				return true
			},
			rule: "retention-reason",
		},
		{
			name: "a class no policy enforces",
			mutate: func(register *Register) bool {
				register.Retention[0].Class = "drafts"
				return true
			},
			rule: "retention-unknown",
		},
		{
			name: "no analytics event published at all",
			mutate: func(register *Register) bool {
				register.Analytics = nil
				return true
			},
			rule: "analytics-empty",
		},
		{
			name: "an event published twice",
			mutate: func(register *Register) bool {
				register.Analytics = append(register.Analytics, register.Analytics[0])
				return true
			},
			rule: "analytics-duplicate",
		},
		{
			name: "properties in an order two runs cannot compare",
			mutate: func(register *Register) bool {
				for index := range register.Analytics {
					if len(register.Analytics[index].Properties) > 1 {
						properties := register.Analytics[index].Properties
						properties[0], properties[1] = properties[1], properties[0]
						return true
					}
				}
				return false
			},
			rule: "analytics-order",
		},
		{
			name: "a property shaped like a person",
			mutate: func(register *Register) bool {
				event := &register.Analytics[0]
				event.Properties = []string{"locale", "user_agent"}
				return true
			},
			rule: "analytics-properties",
		},
		{
			name: "an event the dispatcher may send and the register omits",
			mutate: func(register *Register) bool {
				register.Analytics = register.Analytics[1:]
				return true
			},
			rule: "analytics-missing",
		},
		{
			name: "an event the dispatcher refuses",
			mutate: func(register *Register) bool {
				register.Analytics[0].Event = "account.deleted"
				return true
			},
			rule: "analytics-unknown",
		},
		{
			name: "an area of the phase left out",
			mutate: func(register *Register) bool {
				register.Areas = register.Areas[1:]
				return true
			},
			rule: "area-set",
		},
		{
			name: "the areas out of the order the phase names them",
			mutate: func(register *Register) bool {
				register.Areas[0], register.Areas[1] = register.Areas[1], register.Areas[0]
				return true
			},
			rule: "area-order",
		},
		{
			name: "a verdict outside the vocabulary",
			mutate: func(register *Register) bool {
				findArea(register, "logs").Verdict = "safe"
				return true
			},
			rule: "area-verdict",
		},
		{
			name: "an area that states nothing",
			mutate: func(register *Register) bool {
				findArea(register, "logs").Note = "  "
				return true
			},
			rule: "area-note",
		},
		{
			name: "an area nothing was run for",
			mutate: func(register *Register) bool {
				findArea(register, "deletion").Execution = ""
				return true
			},
			rule: "area-execution",
		},
		{
			name: "an area that cites no path",
			mutate: func(register *Register) bool {
				findArea(register, "logs").Evidence = nil
				return true
			},
			rule: "area-evidence",
		},
		{
			name: "an area that cites a path that is not there",
			mutate: func(register *Register) bool {
				findArea(register, "logs").Evidence = []string{"internal/platform/logging/nowhere_test.go"}
				return true
			},
			rule: "area-evidence",
		},
		{
			name: "a gap with no finding to carry the residual",
			mutate: func(register *Register) bool {
				findArea(register, "public-export").Finding = ""
				return true
			},
			rule: "area-gap",
		},
		{
			name: "a gap that points at a finding nobody declared",
			mutate: func(register *Register) bool {
				findArea(register, "public-export").Finding = "PRV-09"
				return true
			},
			rule: "area-gap",
		},
		{
			name: "a mitigated area that still points at a finding",
			mutate: func(register *Register) bool {
				findArea(register, "logs").Finding = "PRV-01"
				return true
			},
			rule: "area-finding",
		},
		{
			name: "two findings with the same identifier",
			mutate: func(register *Register) bool {
				register.Findings[1].ID = register.Findings[0].ID
				return true
			},
			rule: "finding-id",
		},
		{
			name: "a severity outside the vocabulary",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-02").Severity = "Grave"
				return true
			},
			rule: "finding-severity",
		},
		{
			name: "a status outside the vocabulary",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-02").Status = "todo"
				return true
			},
			rule: "finding-status",
		},
		{
			name: "a finding that is not stated",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-01").Title = ""
				return true
			},
			rule: "finding-title",
		},
		{
			name: "a finding with no work that closes it",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-01").Plan = ""
				return true
			},
			rule: "finding-plan",
		},
		{
			name: "a finding outside the areas of the phase",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-01").Area = "exports"
				return true
			},
			rule: "finding-area",
		},
		{
			name: "a finding that cites nothing",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-01").Evidence = nil
				return true
			},
			rule: "finding-evidence",
		},
		{
			name: "a finding that cites a path that is not there",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-02").Evidence = []string{"internal/platform/clientip/nowhere.go"}
				return true
			},
			rule: "finding-evidence",
		},
		{
			name: "an Alta finding left open",
			mutate: func(register *Register) bool {
				finding := findFinding(register, "PRV-02")
				finding.Severity = severityHigh
				finding.Status = statusOpen
				finding.Owner = ""
				finding.AcceptedOn = ""
				return true
			},
			rule: "finding-open",
		},
		{
			name: "an accepted residual with no owner",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-02").Owner = ""
				return true
			},
			rule: "finding-owner",
		},
		{
			name: "an acceptance with no date",
			mutate: func(register *Register) bool {
				findFinding(register, "PRV-02").AcceptedOn = ""
				return true
			},
			rule: "finding-accepted-on",
		},
		{
			name: "an open finding carrying an owner, which is an acceptance in disguise",
			mutate: func(register *Register) bool {
				finding := findFinding(register, "PRV-01")
				finding.Status = statusOpen
				return true
			},
			rule: "finding-acceptance",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			before := document(t).Register
			after := mutated(t, testCase.mutate)
			beforeRaw, _ := json.Marshal(before)
			afterRaw, _ := json.Marshal(after)
			if bytes.Equal(beforeRaw, afterRaw) {
				t.Fatal("the mutation did not change the register")
			}
			violations := auditIn(t, after, prose)
			if !hasRule(violations, testCase.rule) {
				t.Fatalf("the mutation was not refused by %s; the rules that fired were %v",
					testCase.rule, rulesOf(violations))
			}
		})
	}
}

// TestTheDocumentWithoutProseIsRefused covers the half of the document the
// block cannot carry: a register with no review is a table, not an audit.
func TestTheDocumentWithoutProseIsRefused(t *testing.T) {
	delivered := document(t)
	raw, err := json.MarshalIndent(delivered.Register, "", "  ")
	if err != nil {
		t.Fatalf("rendering the register: %v", err)
	}
	violations := auditText(t, registerFence+"\n"+string(raw)+"\n```\n")
	if !hasRule(violations, "prose") {
		t.Fatalf("a document with no prose was accepted: %v", rulesOf(violations))
	}
}

// TestARealAddressInTheReportIsRefused is the phase's own validation —
// "o relatório não contém PII real" — with the address a reviewer actually
// writes by accident: one from their own mail, not from a reserved domain.
func TestARealAddressInTheReportIsRefused(t *testing.T) {
	delivered := document(t)
	leaked := strings.Replace(delivered.Text,
		"export-other@arena.example.com", "ana.silva@corp.example-business.com", 1)
	if leaked == delivered.Text {
		t.Fatal("the mutation found nothing to change")
	}
	violations := auditText(t, leaked)
	if !hasRule(violations, "register-pii") {
		t.Fatalf("a real address was accepted in the report: %v", rulesOf(violations))
	}
}

// TestReservedDomainsAreTheOnesDocumentationUses pins the vocabulary: the
// synthetic accounts have to live where no mail is delivered.
func TestReservedDomainsAreTheOnesDocumentationUses(t *testing.T) {
	for _, domain := range []string{"example.com", "arena.example.com", "example.org", "invalid.test", "localhost"} {
		if !reservedDomain(domain) {
			t.Errorf("%q is reserved and the rule refused it", domain)
		}
	}
	for _, domain := range []string{"gmail.com", "example-business.com", "arena.example.com.br"} {
		if reservedDomain(domain) {
			t.Errorf("%q is not reserved and the rule accepted it", domain)
		}
	}
}

// TestForbiddenShapeDoesNotCryWolf is the guard against a rule that gets
// waived: "participants_total" contains the letters of "ip" and must pass, while
// a key actually built from the token must not.
func TestForbiddenShapeDoesNotCryWolf(t *testing.T) {
	allowed := []string{"participants_total", "position_changes", "description", "subscriptions", "username_history"}
	for _, key := range allowed {
		if token := forbiddenShape(surfacePublic, key); token != "" {
			t.Errorf("the key %q was refused for carrying %q", key, token)
		}
	}
	refused := map[string]string{
		"ip_address":   "ip",
		"user_agent":   "user",
		"provider_ref": "provider",
		"access_key":   "key",
		"email":        "email",
	}
	for key, token := range refused {
		if got := forbiddenShape(surfacePublic, key); got != token {
			t.Errorf("the key %q was accepted, or refused for %q instead of %q", key, got, token)
		}
	}
	// The subject's own document is allowed the categories the public one is
	// not, and the vocabulary has to be the stricter one only where it is
	// meant to be.
	for _, key := range []string{"email", "username", "transactions", "subscriptions"} {
		if token := forbiddenShape(surfaceSubject, key); token != "" {
			t.Errorf("the key %q was refused for the subject's own export for carrying %q", key, token)
		}
	}
}

// TestThePublicSurfaceIsStricterThanTheSubjects pins the asymmetry the two
// vocabularies exist for: the subject's own email is admitted in the subject's
// export and never in a public one.
func TestThePublicSurfaceIsStricterThanTheSubjects(t *testing.T) {
	if token := forbiddenShape(surfaceSubject, "email"); token != "" {
		t.Errorf("the subject's own export cannot carry the subject's email, refused for %q", token)
	}
	if token := forbiddenShape(surfacePublic, "email"); token != "email" {
		t.Errorf("a public export carrying an email was accepted")
	}
}

// TestScannedSurfacesAreReal catches the failure mode of a scanner: reading a
// file that does not hold the document and reporting a clean empty surface.
func TestScannedSurfacesAreReal(t *testing.T) {
	for _, entry := range document(t).Register.Allowlists {
		keys, err := scanJSONKeys(filepath.Join(repositoryRoot, entry.Path))
		if err != nil {
			t.Fatalf("%s: %v", entry.Path, err)
		}
		if len(keys) < 10 {
			t.Errorf("%s: the scan found %d key(s), which is a surface nobody validated", entry.Path, len(keys))
		}
	}
}

// TestExecutionFailuresAndEmptyRunsAreRefused covers the two ways a command can
// answer "yes" without exercising anything.
func TestExecutionFailuresAndEmptyRunsAreRefused(t *testing.T) {
	areas := []Area{
		{Key: "red", Execution: "false"},
		{Key: "empty", Execution: "printf 'testing: warning: no tests to run\\n'"},
		{Key: "green", Execution: "true"},
	}
	_, violations := runAreas(repositoryRoot, areas, 30*time.Second)
	if !hasRule(violations, "area-run") {
		t.Errorf("a failing execution was accepted: %v", rulesOf(violations))
	}
	if !hasRule(violations, "area-empty") {
		t.Errorf("an execution that matched no test was accepted: %v", rulesOf(violations))
	}
	for _, violation := range violations {
		if violation.Subject == "green" {
			t.Errorf("a green execution was refused: %v", violation)
		}
	}
}

// TestReadDocumentRefusesTheTwoMalformedShapes holds the parser itself: a
// document with no block and a block that is never closed are the two ways the
// register arrives broken, and both have to be an error instead of an empty
// register that passes every rule about empty things.
func TestReadDocumentRefusesTheTwoMalformedShapes(t *testing.T) {
	directory := t.TempDir()
	shapes := map[string]string{
		"no-block.md":      "# Auditoria\n\nprosa sem bloco\n",
		"unclosed.md":      "# Auditoria\n\n" + registerFence + "\n{\"version\": 1}\n",
		"unknown-field.md": "# Auditoria\n\n" + registerFence + "\n{\"version\": 1, \"auditor\": \"eu\"}\n```\n",
	}
	for name, body := range shapes {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		if _, err := readDocument(path); err == nil {
			t.Errorf("%s was read as a register", name)
		}
	}
}
