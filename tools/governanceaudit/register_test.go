// Tests of the launch-governance register (P20-T01).
//
// Two properties have to hold, and a test with only the first would be
// worthless:
//
//   - the register this repository commits passes every rule (the baseline,
//     which is also audited as the file on disk);
//   - each rule refuses a document that breaks exactly it. The mutation cases
//     below change one thing in a clean fixture and require the finding, so a
//     rule that stopped working fails here instead of certifying the release;
//     and each mutation is applied through a replacement that fails the test
//     when the text it targets is gone, so a case cannot pass by mutating
//     nothing.
//
// The second half is what makes the release gate mean something: a gate whose
// rules nobody has seen refuse anything is a green light with a comment.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// registerPath is where the fixture document lives, relative to its root: the
// register is a document of the repository, not of the tool.
const registerPath = "docs/GOVERNANCE.md"

// fixtureRegister is a complete, minimal register: the phase's seven decisions,
// five of them decided and two blocked, prose and machine-readable block in
// agreement. It is a model of the register, not a copy of the one delivered —
// the delivered one is audited separately, as it is.
const fixtureRegister = `# Registro de governança (fixture)

O registro destas sete decisões.

## Decisões

### age-minimum

**Estado:** decidida

**Decisão:** dezoito anos, em todos os mercados.

### repository-license

**Estado:** decidida

**Decisão:** AGPL-3.0-only.

**Pendências:** entregar o arquivo LICENSE.

### data-subject-channel

**Estado:** decidida

**Decisão:** alias dedicado mantido pelo proprietário.

### retention-policy

**Estado:** decidida

**Decisão:** ratificadas as janelas em vigor no código.

### launch-markets

**Estado:** decidida

**Decisão:** Brasil e internacional desde o beta.

### terms-of-use

**Estado:** bloqueio

**Bloqueio:** decidir se o beta público exige termos publicados.

### security-channel

**Estado:** bloqueio

**Bloqueio:** confirmar o canal oficial de reporte.

## Registro executável

` + "```json" + `
{
  "decided_by": "titular do repositório",
  "recorded_at": "2026-09-22",
  "items": [
    {
      "id": "age-minimum",
      "status": "decided",
      "decision": "dezoito anos, em todos os mercados",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/PRIVACY.md §9",
      "pending": []
    },
    {
      "id": "repository-license",
      "status": "decided",
      "decision": "AGPL-3.0-only",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/DEPENDENCIES.md §6",
      "pending": [
        "entregar o arquivo LICENSE com o texto canônico"
      ]
    },
    {
      "id": "data-subject-channel",
      "status": "decided",
      "decision": "alias dedicado mantido pelo proprietário",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/PRIVACY.md §10",
      "pending": []
    },
    {
      "id": "retention-policy",
      "status": "decided",
      "decision": "ratificadas as janelas em vigor",
      "decided_at": "2026-09-22",
      "enforced_by": "internal/profiles/domain/retention.go",
      "pending": []
    },
    {
      "id": "launch-markets",
      "status": "decided",
      "decision": "Brasil e internacional desde o beta",
      "decided_at": "2026-09-22",
      "enforced_by": "README.md",
      "pending": []
    },
    {
      "id": "terms-of-use",
      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "decidir se o beta público exige termos publicados",
        "owner": "titular do repositório",
        "blocks": "beta público"
      }
    },
    {
      "id": "security-channel",
      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "confirmar o canal oficial de reporte",
        "owner": "titular do repositório",
        "blocks": "release"
      }
    }
  ]
}
` + "```" + `
`

// writeRoot lays the document out in a temporary root, the way the repository
// lays it out.
func writeRoot(t *testing.T, document string) string {
	t.Helper()
	root := t.TempDir()
	full := filepath.Join(root, registerPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("prepare fixture: %v", err)
	}
	if err := os.WriteFile(full, []byte(document), 0o644); err != nil {
		t.Fatalf("prepare fixture: %v", err)
	}
	return root
}

// auditDocument judges one document.
func auditDocument(t *testing.T, document string) Report {
	t.Helper()
	report, err := Audit(filepath.Join(writeRoot(t, document), registerPath))
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	return report
}

// replaceInRegister applies a mutation, and fails when the text it targets is
// gone: a mutation that did not apply would let the case pass without testing
// anything.
func replaceInRegister(t *testing.T, document, old, new string) string {
	t.Helper()
	if !strings.Contains(document, old) {
		t.Fatalf("the fixture no longer contains %q, so this mutation no longer applies", old)
	}
	return strings.Replace(document, old, new, 1)
}

// closedRegister is the fixture with its two open decisions taken: the gate has
// to be green for a register that answers everything, and the blocked case
// below would otherwise prove nothing about the difference.
func closedRegister(t *testing.T) string {
	t.Helper()
	document := replaceInRegister(t, fixtureRegister, `      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "decidir se o beta público exige termos publicados",
        "owner": "titular do repositório",
        "blocks": "beta público"
      }`, `      "status": "decided",
      "decision": "termos e aviso de privacidade publicados nos dois idiomas",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/TERMS.md",
      "pending": []`)
	document = replaceInRegister(t, document, `      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "confirmar o canal oficial de reporte",
        "owner": "titular do repositório",
        "blocks": "release"
      }`, `      "status": "decided",
      "decision": "o canal privado do GitHub é o canal oficial do lançamento",
      "decided_at": "2026-09-22",
      "enforced_by": "SECURITY.md",
      "pending": []`)
	document = replaceInRegister(t, document, "### terms-of-use\n\n**Estado:** bloqueio", "### terms-of-use\n\n**Estado:** decidida")
	document = replaceInRegister(t, document, "### security-channel\n\n**Estado:** bloqueio", "### security-channel\n\n**Estado:** decidida")
	return document
}

// rulesOf lists the rules a finding set broke, without repetition and in a
// stable order, so a case can require exactly one.
func rulesOf(findings []Finding) []string {
	seen := map[string]bool{}
	var rules []string
	for _, finding := range findings {
		if !seen[finding.Rule] {
			seen[finding.Rule] = true
			rules = append(rules, finding.Rule)
		}
	}
	sort.Strings(rules)
	return rules
}

// TestBaselinePassesEveryRule is the fixture's own verdict. Every mutation case
// below depends on it: without it, a case could pass because the baseline was
// already broken.
func TestBaselinePassesEveryRule(t *testing.T) {
	report := auditDocument(t, fixtureRegister)
	for _, finding := range report.Findings {
		t.Errorf("baseline: %s", finding)
	}
	if len(report.Findings) > 0 {
		t.Fatalf("the baseline is not clean, so no mutation case means anything")
	}
	if len(report.Items) != len(requiredItems) {
		t.Fatalf("the baseline carries %d items and the phase requires %d", len(report.Items), len(requiredItems))
	}
	if len(report.Blocked) != 2 {
		t.Fatalf("the baseline blocks %d item(s); the fixture models two open decisions", len(report.Blocked))
	}
}

// TestEachRuleRefusesItsMutation is the falsification of every rule: one
// mutation, one expected rule, and no other rule broken by it.
func TestEachRuleRefusesItsMutation(t *testing.T) {
	cases := []struct {
		name string
		// rules is the exact set the mutation must break: naming it in full
		// keeps a mutation that reaches further than its intent from passing
		// as if it reached exactly its intent.
		rules  []string
		mutate func(*testing.T, string) string
	}{
		{
			name:  "a document with no machine-readable block",
			rules: []string{ruleFenceMissing},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, "```json", "```text")
			},
		},
		{
			name:  "a block that is not valid JSON",
			rules: []string{ruleFenceUnparsable},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `"recorded_at": "2026-09-22",`, `"recorded_at" "2026-09-22",`)
			},
		},
		{
			name: "a block with a field the schema does not know",
			// A typo in a field name must fail loudly: silently ignoring it
			// would drop the requirement the field was meant to state.
			rules: []string{ruleFenceUnparsable},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `  "recorded_at": "2026-09-22",`, "  \"recorded_at\": \"2026-09-22\",\n  \"reviewed_by\": \"alguém\",")
			},
		},
		{
			name:  "a register that names nobody as the owner of its decisions",
			rules: []string{ruleRegisterIncomplete},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `"decided_by": "titular do repositório",`, `"decided_by": "  ",`)
			},
		},
		{
			name:  "a decision the phase requires that the register does not carry",
			rules: []string{ruleRequiredItem},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `    {
      "id": "terms-of-use",
      "status": "blocked",
      "decision": "",
      "decided_at": "",
      "enforced_by": "",
      "pending": [],
      "blocker": {
        "needs": "decidir se o beta público exige termos publicados",
        "owner": "titular do repositório",
        "blocks": "beta público"
      }
    },
`, "")
			},
		},
		{
			name: "a decision the register states twice",
			// The prose of the duplicated item is shared, so the only defect
			// is the duplicate itself: one decision, one entry.
			rules: []string{ruleRequiredItem},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `    {
      "id": "age-minimum",
      "status": "decided",
      "decision": "dezoito anos, em todos os mercados",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/PRIVACY.md §9",
      "pending": []
    },
`, `    {
      "id": "age-minimum",
      "status": "decided",
      "decision": "dezoito anos, em todos os mercados",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/PRIVACY.md §9",
      "pending": []
    },
    {
      "id": "age-minimum",
      "status": "decided",
      "decision": "dezoito anos, em todos os mercados",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/PRIVACY.md §9",
      "pending": []
    },
`)
			},
		},
		{
			name: "a decision nobody asked the register for",
			// The prose section comes with it, so the only defect is the item
			// the phase does not require.
			rules: []string{ruleRequiredItem},
			mutate: func(t *testing.T, document string) string {
				document = replaceInRegister(t, document, "### terms-of-use\n", "### extra-decision\n\n**Estado:** decidida\n\n**Decisão:** algo que a fase não pede.\n\n### terms-of-use\n")
				return replaceInRegister(t, document, `    {
      "id": "terms-of-use",`, `    {
      "id": "extra-decision",
      "status": "decided",
      "decision": "algo que a fase não pede",
      "decided_at": "2026-09-22",
      "enforced_by": "docs/GOVERNANCE.md",
      "pending": []
    },
    {
      "id": "terms-of-use",`)
			},
		},
		{
			name: "a status the gate cannot judge",
			// The prose states the fallback word, so the only defect is the
			// status itself: a third status is not a decision, it is a word.
			rules: []string{ruleStatusUnknown},
			mutate: func(t *testing.T, document string) string {
				document = replaceInRegister(t, document, "### age-minimum\n\n**Estado:** decidida", "### age-minimum\n\n**Estado:** bloqueio")
				return replaceInRegister(t, document, `      "id": "age-minimum",
      "status": "decided",`, `      "id": "age-minimum",
      "status": "pendente",`)
			},
		},
		{
			name:  "a decision decided without stating what was decided",
			rules: []string{ruleDecidedIncomplete},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `      "decision": "dezoito anos, em todos os mercados",
      "decided_at": "2026-09-22",`, `      "decision": "",
      "decided_at": "2026-09-22",`)
			},
		},
		{
			name: "a decision decided without naming where it is enforced",
			// A decision with no place of application is an annotation, and an
			// annotation cannot be verified by anybody.
			rules: []string{ruleDecidedIncomplete},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `      "enforced_by": "docs/PRIVACY.md §9",
      "pending": []`, `      "enforced_by": "",
      "pending": []`)
			},
		},
		{
			name:  "an open decision that states what is needed and not who owes it",
			rules: []string{ruleBlockerIncomplete},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `        "owner": "titular do repositório",
        "blocks": "beta público"`, `        "owner": "",
        "blocks": "beta público"`)
			},
		},
		{
			name:  "an open decision that states nothing about what it prevents",
			rules: []string{ruleBlockerIncomplete},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `        "blocks": "release"`, `        "blocks": " "`)
			},
		},
		{
			name:  "an open decision with no statement of what is needed",
			rules: []string{ruleBlockerIncomplete},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `        "needs": "confirmar o canal oficial de reporte",
`, "")
			},
		},
		{
			name:  "a pending step that is empty",
			rules: []string{rulePendingEmpty},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `        "entregar o arquivo LICENSE com o texto canônico"`, `        "   "`)
			},
		},
		{
			name:  "one item answered yes and no at once, in the decision",
			rules: []string{ruleStateInconsistent},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `      "id": "terms-of-use",
      "status": "blocked",
      "decision": "",`, `      "id": "terms-of-use",
      "status": "blocked",
      "decision": "decidido sim",`)
			},
		},
		{
			name:  "one item answered yes and no at once, in the blocker",
			rules: []string{ruleStateInconsistent},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `      "enforced_by": "docs/PRIVACY.md §9",
      "pending": []`, `      "enforced_by": "docs/PRIVACY.md §9",
      "pending": [],
      "blocker": {
        "needs": "decidir o que já foi decidido",
        "owner": "titular do repositório",
        "blocks": "release"
      }`)
			},
		},
		{
			name:  "a date a reader cannot parse the way the tool does",
			rules: []string{ruleDateMalformed},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `      "decision": "ratificadas as janelas em vigor",
      "decided_at": "2026-09-22",`, `      "decision": "ratificadas as janelas em vigor",
      "decided_at": "22/09/2026",`)
			},
		},
		{
			name:  "a register with no date of its own",
			rules: []string{ruleDateMalformed},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, `"recorded_at": "2026-09-22",`, `"recorded_at": "",`)
			},
		},
		{
			name:  "a decision the machine carries and the reader cannot read",
			rules: []string{ruleProseMissing},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, "### security-channel\n\n**Estado:** bloqueio\n\n**Bloqueio:** confirmar o canal oficial de reporte.\n\n", "")
			},
		},
		{
			name:  "a decision whose two halves disagree about its state",
			rules: []string{ruleProseStateMismatch},
			mutate: func(t *testing.T, document string) string {
				return replaceInRegister(t, document, "### launch-markets\n\n**Estado:** decidida", "### launch-markets\n\n**Estado:** bloqueio")
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			findings := auditDocument(t, testCase.mutate(t, fixtureRegister)).Findings
			if len(findings) == 0 {
				t.Fatalf("nothing was refused, and the rule that should have refused it is %s", strings.Join(testCase.rules, ", "))
			}
			rules := rulesOf(findings)
			if strings.Join(rules, ",") != strings.Join(testCase.rules, ",") {
				for _, finding := range findings {
					t.Errorf("finding: %s", finding)
				}
				t.Fatalf("the mutation broke %v; it should have broken %v", rules, testCase.rules)
			}
		})
	}
}

// TestDeliveredRegisterCarriesEveryDecision audits the document this repository
// commits. The fixture above is a model; this is the one the gate reads.
func TestDeliveredRegisterCarriesEveryDecision(t *testing.T) {
	path := filepath.Join("..", "..", registerPath)
	report, err := Audit(path)
	if err != nil {
		t.Fatalf("Audit(%s): %v", path, err)
	}
	for _, finding := range report.Findings {
		t.Errorf("delivered register: %s", finding)
	}
	if len(report.Findings) > 0 {
		t.Fatalf("the delivered register is not well formed")
	}

	stated := map[string]bool{}
	for _, item := range report.Items {
		stated[item.ID] = true
	}
	for _, id := range requiredItems {
		if !stated[id] {
			t.Errorf("the delivered register carries no verdict on %s", id)
		}
	}
	if len(report.Items) != len(requiredItems) {
		t.Errorf("the delivered register carries %d item(s) and the phase requires %d", len(report.Items), len(requiredItems))
	}

	// Every open decision names what is missing and who owes it, and every
	// decided one names where it is applied: that is the difference between a
	// document that records decisions and one that collects wishes.
	for _, item := range report.Blocked {
		if item.Blocker == nil || item.Blocker.Owner == "" {
			t.Errorf("the delivered register blocks %s without naming who decides", item.ID)
		}
	}
	for _, item := range report.Items {
		if item.Status == statusDecided && strings.TrimSpace(item.EnforcedBy) == "" {
			t.Errorf("the delivered register decides %s without naming where it is applied", item.ID)
		}
	}
}

// TestGateFailsWhileAnItemIsBlocked is the contract of the release gate: a
// well-formed register with an open decision is a red gate, and -check is the
// mode that judges the document without pretending the release is authorized.
func TestGateFailsWhileAnItemIsBlocked(t *testing.T) {
	cases := []struct {
		name     string
		document string
		check    bool
		want     int
	}{
		{
			name:     "an open decision, without -check",
			document: fixtureRegister,
			check:    false,
			want:     exitBlocked,
		},
		{
			name:     "an open decision, with -check",
			document: fixtureRegister,
			check:    true,
			want:     exitOK,
		},
		{
			name:     "no open decision, without -check",
			document: closedRegister(t),
			check:    false,
			want:     exitOK,
		},
		{
			name:     "a malformed register, with -check",
			document: replaceInRegister(t, fixtureRegister, "```json", "```text"),
			check:    true,
			want:     exitBlocked,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := writeRoot(t, testCase.document)
			args := []string{"-root", root, "-file", registerPath}
			if testCase.check {
				args = append(args, "-check")
			}
			var stdout, stderr bytes.Buffer
			if got := run(args, &stdout, &stderr); got != testCase.want {
				t.Fatalf("run exited %d; it should exit %d\nstdout: %s\nstderr: %s", got, testCase.want, stdout.String(), stderr.String())
			}

			// The failure has to be actionable: the exit code stops a release,
			// and the message says what is missing, who owes it and what it
			// prevents. An unexplained red gate is an ignored red gate.
			if testCase.want == exitBlocked && testCase.document == fixtureRegister {
				for _, want := range []string{"BLOQUEIO terms-of-use", "BLOQUEIO security-channel", "titular do repositório", "beta público"} {
					if !strings.Contains(stderr.String(), want) {
						t.Errorf("the gate's refusal does not state %q\nstderr: %s", want, stderr.String())
					}
				}
			}
		})
	}
}

// TestGateReportsUnreadableRegisterWithoutCrashing keeps the failure mode
// honest: a missing file is a red gate with a message, not a panic.
func TestGateReportsUnreadableRegisterWithoutCrashing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"-root", t.TempDir(), "-file", registerPath}, &stdout, &stderr); got != exitBlocked {
		t.Fatalf("run exited %d on a register it cannot read", got)
	}
	if !strings.Contains(stderr.String(), "no such file") {
		t.Fatalf("the refusal does not explain the missing document: %s", stderr.String())
	}
}
