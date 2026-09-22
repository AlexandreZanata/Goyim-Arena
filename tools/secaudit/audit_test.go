package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// allRules is every rule the audit can refuse on. The table below has to hold
// one mutation for each name here — a rule without a mutation is a rule that
// might never fire, and TestEveryRuleHasAMutation is what keeps the two lists
// from drifting apart. Rules that are read failures rather than violations (a
// document without a block, JSON that does not parse) are covered by
// TestReadDocumentRefusesWhatCannotBeRead instead.
var allRules = []string{
	"register-version",
	"register-date",
	"register-threats",
	"register-areas",
	"threat-id",
	"threat-duplicate",
	"threat-note",
	"threat-model-unreadable",
	"threat-model-severity",
	"threat-model-duplicate",
	"threat-model-empty",
	"threat-model-matrix",
	"threat-set",
	"threat-severity",
	"threat-verdict",
	"threat-critical-monitored",
	"threat-open-floor",
	"threat-evidence",
	"area-set",
	"area-execution",
	"area-evidence",
	"area-job",
	"area-run",
	"finding-id",
	"finding-title",
	"finding-severity",
	"finding-status",
	"finding-plan",
	"finding-evidence",
	"finding-floor",
	"finding-acceptance",
	"finding-threat",
	"prose",
	"secrets-env-tracked",
	"secrets-env-ignored",
	"secrets-key-material",
}

// fixture is a throwaway repository: the smallest tree a register can resolve
// against, so a rule can be tested without the real one.
type fixture struct {
	root     string
	document string
	tracked  []string
	// running asks the harness for the audit that executes the areas, which
	// the one rule about a red command needs.
	running bool
}

// fixtureFiles are the evidence paths the baseline register cites, so mutating
// a citation is the only thing that can break resolution.
var fixtureFiles = []string{
	"internal/platform/security/csrf.go",
	"internal/moderation/adapters/http/handler_test.go",
	"api/openapi.json",
}

// baseline is the register the fixture starts from: two threats of the model
// and the twelve areas of the phase, with three findings that exercise the
// floor and the acceptance rules.
func baseline() Register {
	areas := make([]Area, 0, len(requiredAreas))
	for _, key := range requiredAreas {
		areas = append(areas, Area{
			Key:       key,
			Execution: "true",
			Evidence:  []string{"internal/platform/security/csrf.go"},
			Note:      "the area is exercised",
		})
	}
	areas[indexOf(requiredAreas, "secrets")].DeferredTo = []string{"source-scans"}

	return Register{
		Version:   1,
		AuditedOn: "2026-09-22",
		Threats: []Threat{
			{
				ID:       "THR-AUTH-01",
				Severity: severityCritical,
				Verdict:  verdictMitigated,
				Evidence: []string{"internal/platform/security/csrf.go"},
				Note:     "the control exists and the suite covers it",
			},
			{
				ID:       "THR-MOD-01",
				Severity: severityHigh,
				Verdict:  verdictAccepted,
				Finding:  "SEC-01",
				Evidence: []string{"internal/moderation/adapters/http/handler_test.go"},
				Note:     "the control exists and a residual was accepted",
			},
		},
		Areas: areas,
		Findings: []Finding{
			{
				ID: "SEC-01", Title: "a high residual", Severity: severityHigh, Status: statusAccepted,
				Owner: "the owner", AcceptedOn: "2026-09-22", Threat: "THR-MOD-01",
				Evidence: []string{"api/openapi.json"}, Plan: "the next microtask closes it",
			},
			{
				ID: "SEC-02", Title: "a medium residual", Severity: severityMedium, Status: statusAccepted,
				Owner: "the owner", AcceptedOn: "2026-09-22",
				Evidence: []string{"api/openapi.json"}, Plan: "the next microtask closes it",
			},
			{
				ID: "SEC-03", Title: "a low residual", Severity: severityLow, Status: statusFixed,
				Evidence: []string{"api/openapi.json"}, Plan: "closed in this phase",
			},
		},
	}
}

// fixtureModel is the model of the fixture: two threats, one Crítica and one
// Alta, in the shape the real document uses.
const fixtureModel = `# Modelo (fixture)

| ID | Ameaça | STRIDE | Ator | Severidade | Mitigação |
| --- | --- | --- | --- | --- | --- |
| **THR-AUTH-01** | Uma sessão é tomada | Spoofing | A-01 | **Crítica** | token opaco |
| **THR-MOD-01** | Uma denúncia é abusada | Elevation | A-04 | **Alta** | triagem humana |

## Sumário

| Severidade | Ameaças |
| --- | --- |
| **Crítica** | 1 ameaça (` + "`THR-AUTH-01`" + `) |
`

// fixtureMatrix is the test matrix of the fixture, in the shape the real one
// uses: the identifier opens the row and is not emboldened, so the matrix rule
// cannot read a prose mention inside a cell as a row.
const fixtureMatrix = `# Matriz (fixture)

| ID | Severidade | Teste | Residual |
| --- | --- | --- | --- |
| THR-AUTH-01 | Crítica | test: internal/platform/security | Baixo |
| THR-MOD-01 | Alta | test: internal/moderation | Médio |
`

// fixtureVerify is a workflow with the job one area of the fixture defers to.
const fixtureVerify = `name: verify

on:
  push:

jobs:
  foundation:
    runs-on: ubuntu-latest
`

// fixtureSupplyChain is the second workflow, which holds the scan jobs.
const fixtureSupplyChain = `name: supply-chain

on:
  push:

jobs:
  dependency-review:
    runs-on: ubuntu-latest
  source-scans:
    runs-on: ubuntu-latest
`

// newFixture writes the tree and returns it with the document rendered from the
// baseline register.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		threatModelPath: fixtureModel,
		matrixPath:      fixtureMatrix,
		filepath.Join(".github", "workflows", "verify.yml"):       fixtureVerify,
		filepath.Join(".github", "workflows", "supply-chain.yml"): fixtureSupplyChain,
		gitignorePath: "env\n.env\n.env.*\n!.env.example\n",
	}
	for _, rel := range fixtureFiles {
		files[rel] = "// the file the register cites\n"
	}
	for rel, content := range files {
		writeFixtureFile(t, root, rel, content)
	}
	f := &fixture{root: root}
	f.tracked = append([]string{gitignorePath}, fixtureFiles...)
	f.writeRegister(t, baseline())
	return f
}

// writeRegister renders the document from a register and writes it, so the
// prose and the block are always rendered together.
func (f *fixture) writeRegister(t *testing.T, register Register) string {
	t.Helper()
	block, err := json.MarshalIndent(register, "", "  ")
	if err != nil {
		t.Fatalf("marshal the register: %v", err)
	}
	var prose strings.Builder
	prose.WriteString("# Auditoria de segurança (fixture)\n\n")
	prose.WriteString("**Data da execução:** " + register.AuditedOn + "\n\n")
	prose.WriteString("Achados: ")
	for _, finding := range register.Findings {
		prose.WriteString(finding.ID + " ")
	}
	prose.WriteString("\n\nÁreas: ")
	for _, area := range register.Areas {
		prose.WriteString(area.Key + " ")
	}
	prose.WriteString("\n\n")
	document := prose.String() + "```json\n" + string(block) + "\n```\n"
	writeFixtureFile(t, f.root, documentPath, document)
	f.document = document
	return document
}

// audit judges the fixture without running anything, with the tracked list the
// fixture controls, so the environment scan can be tested without a repository.
func (f *fixture) audit(t *testing.T) Report {
	t.Helper()
	return f.judge(t, false)
}

// auditRunning judges the fixture and executes the areas, which is the only way
// the rule about a red command can be reached.
func (f *fixture) auditRunning(t *testing.T) Report {
	t.Helper()
	return f.judge(t, true)
}

// judge is the one place the fixture builds the options, so both modes are
// judged with the same root and the same tracked list.
func (f *fixture) judge(t *testing.T, running bool) Report {
	t.Helper()
	report, err := Audit(Options{
		Root: f.root, Run: running, Timeout: time.Minute,
		Tracker: func(string) ([]string, error) { return f.tracked, nil },
	})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	return report
}

// writeFixtureFile writes one fixture file, creating its directory.
func writeFixtureFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// rulesOf is the set of rules that fired.
func rulesOf(report Report) map[string]bool {
	fired := map[string]bool{}
	for _, violation := range report.Violations {
		fired[violation.Rule] = true
	}
	return fired
}

// sortedRules renders a rule set for a failure message, in a stable order.
func sortedRules(set map[string]bool) []string {
	rules := make([]string, 0, len(set))
	for rule := range set {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	return rules
}

// TestCleanFixtureIsGreen is the control the whole table depends on: if the
// baseline already violated something, every mutation below would prove
// nothing.
func TestCleanFixtureIsGreen(t *testing.T) {
	report := newFixture(t).audit(t)
	if len(report.Violations) != 0 {
		t.Fatalf("the baseline register is refused: %v", report.Violations)
	}
	if report.Threats != 2 {
		t.Fatalf("the fixture model has %d threat(s), want 2", report.Threats)
	}
	if len(report.Executions) != 0 {
		t.Fatalf("nothing runs without Run: %d execution(s)", len(report.Executions))
	}
	if report.Tracked == 0 {
		t.Fatal("the environment scan looked at nothing")
	}
}

// TestEveryRuleHasAMutation refuses a rule nobody mutated, and a mutation of a
// rule that does not exist: the check is two-way.
func TestEveryRuleHasAMutation(t *testing.T) {
	mutated := map[string]bool{}
	for _, mutation := range mutations() {
		mutated[mutation.rule] = true
	}
	for _, rule := range allRules {
		if !mutated[rule] {
			t.Errorf("rule %s has no mutation: a rule that is never exercised might never work", rule)
		}
	}
	for rule := range mutated {
		if !contains(allRules, rule) {
			t.Errorf("mutation of an unknown rule %s: the catalogue and the table disagree", rule)
		}
	}
}

// mutation is one change to the fixture and the rule it must provoke.
type mutation struct {
	name  string
	rule  string
	apply func(t *testing.T, f *fixture)
}

// replace is a text mutation of one file. It fails when the target is not
// there: a mutation that silently did nothing would leave the rule under test
// unexercised while the test still passed.
func replace(rel, old, new string) func(*testing.T, *fixture) {
	return func(t *testing.T, f *fixture) {
		t.Helper()
		path := filepath.Join(f.root, rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.Contains(string(raw), old) {
			t.Fatalf("the mutation target is not in %s, so the mutation never applied:\n%s", rel, old)
		}
		writeFixtureFile(t, f.root, rel, strings.Replace(string(raw), old, new, 1))
	}
}

// write is a whole-file mutation, for a document that has to lose its shape.
func write(rel, content string) func(*testing.T, *fixture) {
	return func(t *testing.T, f *fixture) {
		t.Helper()
		writeFixtureFile(t, f.root, rel, content)
	}
}

// remove deletes a fixture file, which is how a cited source disappears.
func remove(rel string) func(*testing.T, *fixture) {
	return func(t *testing.T, f *fixture) {
		t.Helper()
		if err := os.Remove(filepath.Join(f.root, rel)); err != nil {
			t.Fatalf("remove %s: %v", rel, err)
		}
	}
}

// transform is a mutation of the register itself: the document is rendered
// again from the changed register, so the prose follows the block and the rule
// under test is the only thing left broken. It fails when the rendered document
// is unchanged, for the same reason replace does.
func transform(change func(register *Register)) func(*testing.T, *fixture) {
	return func(t *testing.T, f *fixture) {
		t.Helper()
		before := f.document
		register := baseline()
		change(&register)
		if after := f.writeRegister(t, register); after == before {
			t.Fatal("the mutation did not change the document, so nothing was exercised")
		}
	}
}

// mutations is the table: one change per rule, each against a fixture the
// control test just proved clean.
func mutations() []mutation {
	const doc = documentPath
	const model = threatModelPath
	const matrix = matrixPath
	return []mutation{
		{"the version is not a schema version", "register-version",
			transform(func(r *Register) { r.Version = 0 })},
		{"the date is not a date", "register-date",
			transform(func(r *Register) { r.AuditedOn = "22/09/2026" })},
		{"the register carries no threat", "register-threats",
			transform(func(r *Register) { r.Threats = nil })},
		{"the register carries no area", "register-areas",
			transform(func(r *Register) { r.Areas = nil })},
		{"the identifier is not a threat id", "threat-id",
			transform(func(r *Register) { r.Threats[1].ID = "THR-MOD" })},
		{"the same threat twice", "threat-duplicate",
			transform(func(r *Register) { r.Threats[1].ID = r.Threats[0].ID })},
		{"the threat states no note", "threat-note",
			transform(func(r *Register) { r.Threats[0].Note = "" })},
		{"the model cannot be read", "threat-model-unreadable", remove(model)},
		{"the model row carries no severity", "threat-model-severity",
			replace(model, "| A-04 | **Alta** |", "| A-04 | Alta |")},
		{"the model declares a threat twice", "threat-model-duplicate",
			replace(model, "| **THR-MOD-01** |", "| **THR-AUTH-01** |")},
		{"the model has no threat row", "threat-model-empty",
			write(model, "# Modelo (fixture)\n\n| ID | Ameaça |\n| --- | --- |\n| sem linha de ameaça | nada |\n")},
		{"the matrix never names a threat of the model", "threat-model-matrix",
			replace(matrix, "| THR-MOD-01 | Alta | test: internal/moderation | Médio |", "| (sem ameaça) |")},
		{"the register audits a threat the model lacks", "threat-set",
			transform(func(r *Register) { r.Threats[1].ID = "THR-XXX-99" })},
		{"the register re-grades a threat", "threat-severity",
			transform(func(r *Register) { r.Threats[1].Severity = severityLow })},
		{"the verdict is outside the vocabulary", "threat-verdict",
			transform(func(r *Register) { r.Threats[0].Verdict = "later" })},
		{"an accepted threat names no finding", "threat-verdict",
			transform(func(r *Register) { r.Threats[1].Finding = "" })},
		{"a verdict names a finding that does not exist", "threat-verdict",
			transform(func(r *Register) { r.Threats[1].Finding = "SEC-09" })},
		{"a mitigated threat names a finding nothing owes", "threat-verdict",
			transform(func(r *Register) { r.Threats[0].Finding = "SEC-01" })},
		{"a Crítica threat is answered with monitoring", "threat-critical-monitored",
			transform(func(r *Register) { r.Threats[0].Verdict = verdictMonitored })},
		{"an Alta threat is left open", "threat-open-floor",
			transform(func(r *Register) { r.Threats[1].Verdict = verdictOpen })},
		{"the threat cites what does not exist", "threat-evidence",
			transform(func(r *Register) { r.Threats[0].Evidence = []string{"internal/platform/security/gone.go"} })},
		{"the threat cites nothing at all", "threat-evidence",
			transform(func(r *Register) { r.Threats[0].Evidence = nil })},
		{"the area is not one the phase names", "area-set",
			transform(func(r *Register) { r.Areas[2].Key = "cachee" })},
		{"an area of the phase is missing", "area-set",
			transform(func(r *Register) { r.Areas = r.Areas[1:] })},
		{"the area neither runs nor defers", "area-execution",
			transform(func(r *Register) { r.Areas[0].Execution = "" })},
		{"the area states no note", "area-execution",
			transform(func(r *Register) { r.Areas[0].Note = "" })},
		{"the area cites nothing", "area-evidence",
			transform(func(r *Register) { r.Areas[0].Evidence = nil })},
		{"the area cites what does not exist", "area-evidence",
			transform(func(r *Register) { r.Areas[0].Evidence = []string{"gone.go"} })},
		{"the area defers to a job no workflow defines", "area-job",
			transform(func(r *Register) {
				r.Areas[indexOf(requiredAreas, "secrets")].DeferredTo = []string{"the-job-that-does-not-exist"}
			})},
		{"the finding id breaks the convention", "finding-id",
			transform(func(r *Register) { r.Findings[2].ID = "SEC-3" })},
		{"the finding has no title", "finding-title",
			transform(func(r *Register) { r.Findings[2].Title = "" })},
		{"the finding severity is outside the vocabulary", "finding-severity",
			transform(func(r *Register) { r.Findings[1].Severity = "Grave" })},
		{"the finding status is outside the vocabulary", "finding-status",
			transform(func(r *Register) { r.Findings[1].Status = "later" })},
		{"the finding names no closing work", "finding-plan",
			transform(func(r *Register) { r.Findings[1].Plan = "" })},
		{"the finding cites nothing", "finding-evidence",
			transform(func(r *Register) { r.Findings[1].Evidence = nil })},
		{"the finding cites what does not exist", "finding-evidence",
			transform(func(r *Register) { r.Findings[1].Evidence = []string{"internal/gone.go"} })},
		{"an Alta finding is still open", "finding-floor",
			transform(func(r *Register) { r.Findings[0].Status = statusOpen })},
		{"an accepted finding has no owner", "finding-acceptance",
			transform(func(r *Register) { r.Findings[1].Owner = "" })},
		{"an accepted finding has no date", "finding-acceptance",
			transform(func(r *Register) { r.Findings[1].AcceptedOn = "" })},
		{"an accepted finding has a date that is not one", "finding-acceptance",
			transform(func(r *Register) { r.Findings[1].AcceptedOn = "ontem" })},
		{"the finding names a threat nobody declares", "finding-threat",
			transform(func(r *Register) { r.Findings[1].Threat = "THR-ZZZ-01" })},
		{"the finding names a threat that does not carry it", "finding-threat",
			transform(func(r *Register) { r.Findings[0].Threat = "THR-AUTH-01" })},
		{"an area's command is red", "area-run",
			func(t *testing.T, f *fixture) {
				transform(func(r *Register) { r.Areas[0].Execution = "echo 'a suite is red' >&2; exit 1" })(t, f)
				// The rule is only reachable by running, so this case asks for
				// the audit that executes the areas.
				f.running = true
			}},
		{"the prose forgot a finding", "prose",
			replace(doc, "Achados: SEC-01 SEC-02 SEC-03", "Achados: SEC-01 SEC-03")},
		{"the prose forgot an area", "prose",
			replace(doc, "Áreas: threat-model negative-authorization", "Áreas: threat-model")},
		{"the prose forgot the date of the run", "prose",
			replace(doc, "**Data da execução:** 2026-09-22", "**Data da execução:** ontem")},
		{"a tracked environment file", "secrets-env-tracked",
			func(t *testing.T, f *fixture) {
				f.tracked = append(f.tracked, ".env")
				writeFixtureFile(t, f.root, ".env", "ARENA_SESSION_KEY=not-a-real-secret\n")
			}},
		{"the environment file is not ignored", "secrets-env-ignored",
			write(gitignorePath, "env\n.env.*\n")},
		{"a private key is committed", "secrets-key-material",
			func(t *testing.T, f *fixture) {
				f.tracked = append(f.tracked, "deploy/key.pem")
				writeFixtureFile(t, f.root, "deploy/key.pem", "-----BEGIN RSA PRIVATE KEY-----\n")
			}},
	}
}

// TestMutationsAreRefused applies each mutation to a fresh, clean fixture and
// requires the intended rule to fire. Other rules may fire too — a mutation is
// a broken register, not always a surgical one — so the assertion is that the
// rule under test is among those that refused.
func TestMutationsAreRefused(t *testing.T) {
	for _, mutation := range mutations() {
		t.Run(mutation.name, func(t *testing.T) {
			f := newFixture(t)
			mutation.apply(t, f)
			report := f.audit(t)
			if f.running {
				report = f.auditRunning(t)
			}
			if len(report.Violations) == 0 {
				t.Fatalf("the document was accepted after: %s", mutation.name)
			}
			if !rulesOf(report)[mutation.rule] {
				t.Fatalf("rule %s did not fire; the rules that did were %v\nviolations: %v",
					mutation.rule, sortedRules(rulesOf(report)), report.Violations)
			}
		})
	}
}

// TestTheProseAndTheBlockAreOneDocument covers the failure no transform can
// show, because a transform renders both halves: a block edited while the prose
// was left behind.
func TestTheProseAndTheBlockAreOneDocument(t *testing.T) {
	f := newFixture(t)
	register := baseline()
	register.Findings = append(register.Findings, Finding{
		ID: "SEC-04", Title: "a fourth residual", Severity: severityLow, Status: statusFixed,
		Evidence: []string{"api/openapi.json"}, Plan: "closed in this phase",
	})
	f.writeRegister(t, register)
	if violations := f.audit(t).Violations; len(violations) != 0 {
		t.Fatalf("a document rendered from its own register was refused: %v", violations)
	}

	replace(documentPath, "Achados: SEC-01 SEC-02 SEC-03 SEC-04", "Achados: SEC-01 SEC-02 SEC-03")(t, f)
	report := f.audit(t)
	if !rulesOf(report)["prose"] {
		t.Fatalf("a finding in the block and absent from the prose was accepted: %v", report.Violations)
	}
}

// TestTheExampleEnvironmentFileIsNotALeak covers the one case the scan must not
// refuse: a template carries names, never values.
func TestTheExampleEnvironmentFileIsNotALeak(t *testing.T) {
	f := newFixture(t)
	f.tracked = append(f.tracked, ".env.example")
	writeFixtureFile(t, f.root, ".env.example", "ARENA_SESSION_KEY=\n")
	if report := f.audit(t); len(report.Violations) != 0 {
		t.Fatalf("the example environment file was refused: %v", report.Violations)
	}
}

// TestAScannerPatternIsNotAKey covers the other case: the tools that look for
// key material have to name the header, and a rule that refused them would make
// the scanners uncommittable.
func TestAScannerPatternIsNotAKey(t *testing.T) {
	f := newFixture(t)
	f.tracked = append(f.tracked, "tools/scanner/audit.go")
	writeFixtureFile(t, f.root, "tools/scanner/audit.go", "var header = \"-----BEGIN RSA PRIVATE KEY-----\"\n")
	if report := f.audit(t); len(report.Violations) != 0 {
		t.Fatalf("a Go scanner pattern was refused as a key: %v", report.Violations)
	}
}

// TestReadDocumentRefusesWhatCannotBeRead covers the failures that are Go
// errors rather than violations: the document is not a register yet, so there is
// nothing to judge — and an unknown field is a claim the tool cannot judge.
func TestReadDocumentRefusesWhatCannotBeRead(t *testing.T) {
	cases := []struct {
		name     string
		document string
	}{
		{"no block", "# Auditoria\n\nprosa sem bloco\n"},
		{"an unclosed block", "# Auditoria\n\n```json\n{\"version\": 1}\n"},
		{"unreadable json", "# Auditoria\n\n```json\n{not json}\n```\n"},
		{"a field the tool cannot judge", "# Auditoria\n\n```json\n{\"version\": 1, \"extra\": true}\n```\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			writeFixtureFile(t, f.root, documentPath, test.document)
			if _, err := Audit(Options{
				Root: f.root, Tracker: func(string) ([]string, error) { return nil, nil },
			}); err == nil {
				t.Fatal("the document was accepted")
			}
		})
	}
}

// TestFailingAreaIsAViolation covers the rule that is only reachable by running:
// the audit must never say an area failed without saying what failed.
func TestFailingAreaIsAViolation(t *testing.T) {
	f := newFixture(t)
	executions, violations := runAreas(f.root, []Area{{
		Key: "container", Execution: "echo 'the container recipe is broken' >&2; exit 3",
	}}, time.Minute)
	if len(executions) != 1 {
		t.Fatalf("want one execution, got %d", len(executions))
	}
	if len(violations) != 1 || violations[0].Rule != "area-run" {
		t.Fatalf("want one area-run violation, got %v", violations)
	}
	if !strings.Contains(violations[0].Detail, "the container recipe is broken") {
		t.Fatalf("the violation hides the command's own output: %s", violations[0].Detail)
	}

	green, greenViolations := runAreas(f.root, []Area{{Key: "cache", Execution: "true"}}, time.Minute)
	if len(greenViolations) != 0 || len(green) != 1 {
		t.Fatalf("a green command was refused: %v", greenViolations)
	}
}

// TestTheAuditNeverWrites proves the gate judges the document instead of
// rewriting it: the file has to be byte-identical after a refused run, or the
// audit would be fixing what it is supposed to report.
func TestTheAuditNeverWrites(t *testing.T) {
	f := newFixture(t)
	transform(func(r *Register) { r.Threats[1].Severity = severityLow })(t, f)
	before := readFixture(t, filepath.Join(f.root, documentPath))
	report := f.audit(t)
	if len(report.Violations) == 0 {
		t.Fatal("the mutation was accepted, so this test proves nothing")
	}
	if after := readFixture(t, filepath.Join(f.root, documentPath)); after != before {
		t.Fatal("the audit rewrote the document it judged")
	}
}

// TestDeliveredRegisterStands audits the register this repository ships, in the
// tree it ships in: every path it cites, every threat it claims to have
// executed, every area and every finding. It is the test that makes the
// document's claims the repository's problem instead of a reader's.
func TestDeliveredRegisterStands(t *testing.T) {
	report, err := Audit(Options{
		Root: filepath.Join("..", ".."), Run: false, Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("audit the delivered register: %v", err)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("the delivered register is refused: %v", report.Violations)
	}
	if report.Threats != len(report.Document.Register.Threats) {
		t.Fatalf("the register audits %d of the %d threats of the model",
			len(report.Document.Register.Threats), report.Threats)
	}
	if len(report.Document.Register.Areas) != len(requiredAreas) {
		t.Fatalf("the register has %d area(s), the phase names %d",
			len(report.Document.Register.Areas), len(requiredAreas))
	}
	if report.Tracked == 0 {
		t.Fatal("the environment scan looked at nothing")
	}
}

// readFixture reads a fixture file as a string.
func readFixture(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
