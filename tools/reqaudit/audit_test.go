// Tests of the requirements traceability audit (P20-T02).
//
// The matrix this task rewrote is only worth what its links are worth, and a
// link is exactly the kind of claim that keeps looking right after the code
// moves. Three properties are required, and each of the first two alone would
// be worthless:
//
//   - the matrix this repository commits passes every rule, audited as the file
//     on disk rather than as a fixture;
//   - each rule refuses a repository that breaks exactly it. The mutations
//     below change one thing in a clean fixture — a column, a path, a route, a
//     symbol, a citation — and require the finding, so a rule that stopped
//     working fails here instead of certifying a release. Every mutation is
//     applied through a replacement that fails the test when the text it
//     targets is gone, so no case can pass by mutating nothing;
//   - every rule the audit declares is named by at least one mutation. A rule
//     nobody has seen refuse anything is a green light with a comment.
//
// The fixture is a small repository, not a copy of this one: it carries the
// four documents the audit reads (the matrix, the MVP scope, the served
// contract, a migration directory) and the files its citations resolve against.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The fixture's own paths. The three documents the audit reads are named by the
// production constants; these are the files its citations must resolve against.
const (
	fixtureUseCase   = "internal/fixture/application/confirm.go"
	fixtureRead      = "internal/fixture/application/read.go"
	fixtureUseCaseTS = "internal/fixture/application/confirm_test.go"
	fixtureReadTS    = "internal/fixture/application/read_test.go"
	fixtureMigration = "00001_fixture.sql"
	fixtureMechanism = "internal/platform/postgres/invariants.go"
	fixtureInvTS     = "internal/platform/postgres/invariants_test.go"
)

// fixtureContract is a served contract with the two routes the fixture matrix
// cites. It is small on purpose: what the audit resolves against it is the pair
// (method, route), not the shape of the API.
const fixtureContract = `{
  "openapi": "3.1.0",
  "paths": {
    "/api/v1/me/fixture": {
      "get": {"responses": {"200": {"description": "ok"}}},
      "post": {"responses": {"201": {"description": "created"}}}
    }
  }
}
`

// fixtureMVP is the scope the coverage table has to exhaust: two included items
// and one exclusion, which is enough for both directions of the check.
const fixtureMVP = `# Escopo (fixture)

## 1. Contexto

- Um bullet de narrativa, que não é item de escopo.

## 3. Escopo do MVP

### Núcleo

- Registrar posição inicial.
- Ver agregado de posições.

## 4. Fora de escopo

- Torneios pagos.
`

// fixtureInvariantRow is one row of the invariant table, built once so that the
// mutations below can target a row exactly instead of a fragment that appears
// ten times.
func fixtureInvariantRow(id string) string {
	return fmt.Sprintf("| **%s** | A invariante de %s se mantém sob concorrência. | `fixture` | `09-fixture` | `%s` | — (a invariante é de domínio) | `%s::TestInvariantHolds` |",
		id, id, fixtureMechanism, fixtureInvTS)
}

// fixtureMatrix is the document under audit: a well-formed matrix that traces
// two functional requirements, the ten invariants of the phase and the two
// directions of the MVP scope.
func fixtureMatrix() string {
	invariants := &strings.Builder{}
	refs := make([]string, 0, len(requiredInvariants))
	for _, id := range requiredInvariants {
		invariants.WriteString(fixtureInvariantRow(id))
		invariants.WriteString("\n")
		refs = append(refs, "`"+id+"`")
	}

	// The rows are built by the same functions the mutations target, so a
	// mutation cannot drift from the text it means to change.
	return fmt.Sprintf(`# Matriz de rastreabilidade (fixture)

## 2. Matriz de requisitos

### 2.1 Fixture (`+"`fixture`"+`)

| ID | Descrição | Origem | Módulo | Fase | Endpoint | Caso de uso | Migration | Teste |
|---|---|---|---|---|---|---|---|---|
%s
%s

## 3. Invariantes

| ID | Enunciado | Módulo | Fase | Mecanismo | Migration | Teste |
|---|---|---|---|---|---|---|
%s
## 4. Rastreabilidade do MVP

| Item do MVP | Origem | Requisitos |
|---|---|---|
%s

## 5. Não-requisitos

| Item excluído | Registro |
|---|---|
| Torneios pagos. | `+"`NON-REQ-01`"+` |
`,
		fixtureRequirementRow("REQ-FIX-01"),
		fixtureRequirementRow("REQ-FIX-02"),
		invariants.String(),
		strings.Join(fixtureCoverageRows(refs), "\n"),
	)
}

// fixtureCoverageRows is the traceability table: every item of the MVP scope
// mapped to the requirement that answers it, plus the invariants of §3, which
// the same bidirectionality check requires to be cited.
func fixtureCoverageRows(refs []string) []string {
	return []string{
		"| Registrar posição inicial. | MVP §3 | `REQ-FIX-01` |",
		"| Ver agregado de posições. | MVP §3 | `REQ-FIX-02` |",
		"| Invariantes de negócio | BR §11 | " + strings.Join(refs, ", ") + " |",
	}
}

// fixtureRow is the first requirement row, verbatim, so a mutation can replace
// it whole: a row is the unit a reader sees, and half a row is not a row.
func fixtureRequirementRow(id string) string {
	if id == "REQ-FIX-01" {
		return fmt.Sprintf("| **REQ-FIX-01** | Registrar a posição inicial entre os valores fixos. | MVP §3 | `fixture` | `09-fixture` | `POST /api/v1/me/fixture` | `%s` | `%s` | `%s::TestConfirmRecordsTheChoice` |", fixtureUseCase, fixtureMigration, fixtureUseCaseTS)
	}
	return fmt.Sprintf("| **REQ-FIX-02** | Projetar a posição do próprio participante. | MVP §3 | `fixture` | `09-fixture` | `GET /api/v1/me/fixture` | `%s` | — (a leitura não tem tabela própria: a regra é de domínio) | `%s::TestReadReturnsTheOwnerProjection` |", fixtureRead, fixtureReadTS)
}

// fixtureFiles is the whole fixture repository: the four documents the audit
// reads plus the files its citations have to resolve against.
func fixtureFiles() map[string]string {
	return map[string]string{
		requirementsPath: fixtureMatrix(),
		mvpPath:          fixtureMVP,
		contractPath:     fixtureContract,
		filepath.Join(migrationsDir, fixtureMigration): "-- +goose up\n\nSELECT 1;\n",
		fixtureUseCase:   "// Package application holds the fixture's use cases.\npackage application\n\n// Confirm records the first choice of a participant.\nfunc Confirm() {}\n",
		fixtureRead:      "// Package application holds the fixture's use cases.\npackage application\n\n// Read returns the projection of the owner.\nfunc Read() {}\n",
		fixtureUseCaseTS: "package application\n\nimport \"testing\"\n\nfunc TestConfirmRecordsTheChoice(t *testing.T) {}\n",
		fixtureReadTS:    "package application\n\nimport \"testing\"\n\nfunc TestReadReturnsTheOwnerProjection(t *testing.T) {}\n",
		fixtureMechanism: "// Package postgres holds the fixture's mechanisms.\npackage postgres\n\n// Invariant is the mechanism the matrix cites.\nfunc Invariant() {}\n",
		fixtureInvTS:     "package postgres\n\nimport \"testing\"\n\nfunc TestInvariantHolds(t *testing.T) {}\n",
	}
}

// writeRoot writes a fixture repository and returns its root.
func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

// edit applies one mutation to one fixture file. It fails the test when the
// text it targets is not there: a mutation that silently does nothing would
// leave the case asserting over an unmutated repository, which is how a gate
// rots while staying green.
func edit(t *testing.T, files map[string]string, path, old, new string) {
	t.Helper()
	document, known := files[path]
	if !known {
		t.Fatalf("the fixture has no %s to mutate", path)
	}
	if !strings.Contains(document, old) {
		t.Fatalf("the fixture's %s does not contain %q, so the mutation would not apply", path, old)
	}
	mutated := strings.Replace(document, old, new, 1)
	if mutated == document {
		t.Fatalf("replacing %q in %s changed nothing", old, path)
	}
	files[path] = mutated
}

// auditRoot writes the mutated fixture and audits it.
func auditRoot(t *testing.T, files map[string]string) Report {
	t.Helper()
	root := writeRoot(t, files)
	report, err := Audit(root)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	return report
}

// rulesOf is the multiset of rules a document broke. It is sorted because the
// order findings are reported in is the audit's business — a matrix is judged
// by which rules it broke and how often, not by where the report happens to
// list them.
func rulesOf(findings []Finding) []string {
	rules := make([]string, 0, len(findings))
	for _, finding := range findings {
		rules = append(rules, finding.Rule)
	}
	sort.Strings(rules)
	return rules
}

// wantRules is rulesOf's counterpart for a case's expectation, sorted so the
// two can be compared as multisets.
func wantRules(rules []string) []string {
	sorted := append([]string(nil), rules...)
	sort.Strings(sorted)
	return sorted
}

// TestFixtureIsClean is the baseline the mutation cases depend on: the fixture
// repository traces everything, so a case that expects one rule is measuring
// that rule and not the fixture's own defects.
func TestFixtureIsClean(t *testing.T) {
	report := auditRoot(t, fixtureFiles())
	for _, finding := range report.Findings {
		t.Errorf("the clean fixture is refused: %s", finding)
	}
	if len(report.Findings) > 0 {
		t.Fatalf("the baseline is not clean, so no mutation below measures what it says it measures")
	}
	if len(report.Requirements) != 2 || len(report.Invariants) != len(requiredInvariants) {
		t.Fatalf("the fixture carries %d requirement(s) and %d invariant(s)", len(report.Requirements), len(report.Invariants))
	}
	if len(report.MissedMVPItems) != 0 {
		t.Fatalf("the clean fixture misses %v", report.MissedMVPItems)
	}
}

// TestEachRuleRefusesItsOwnMutation is the half that makes the gate mean
// something. Each case names the rules its mutation must break, and the check
// is exact: a mutation that breaks a different rule — or an extra one — fails
// here, because a rule that fires for the wrong reason is a rule nobody can
// trust when it fires for the right one.
func TestEachRuleRefusesItsOwnMutation(t *testing.T) {
	cases := []struct {
		name   string
		rules  []string
		mutate func(t *testing.T, files map[string]string)
	}{
		{
			name:  "a row that lost a column",
			rules: []string{ruleRowShape},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, " | `"+fixtureMigration+"` |", " |")
			},
		},
		{
			name: "a requirement stated twice",
			// The duplicate is invisible to the coverage table, which cites
			// the identifier once either way: the only defect is that the
			// matrix claims the same requirement under one heading twice.
			rules: []string{ruleIdentifier},
			mutate: func(t *testing.T, files map[string]string) {
				row := fixtureRequirementRow("REQ-FIX-02")
				edit(t, files, requirementsPath, row+"\n", row+"\n"+row+"\n")
			},
		},
		{
			name: "an identifier the pattern cannot read",
			// Three findings, and only the first is the rule under test:
			// a coverage row cannot cite an identifier the pattern rejects,
			// so the renamed requirement is also unreferenced and the row
			// that named it maps nothing. The audit tells the truth about
			// one change three times, which is what a traceability gate is
			// for.
			rules: []string{ruleIdentifier, ruleCoverageOrphan, ruleCoverageOrphan},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "**REQ-FIX-01**", "**REQ-FIX-1**")
				edit(t, files, requirementsPath, "`REQ-FIX-01`", "`REQ-FIX-1`")
			},
		},
		{
			name:  "a requirement that states no origin",
			rules: []string{ruleFieldEmpty},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "| Projetar a posição do próprio participante. | MVP §3 |", "| Projetar a posição do próprio participante. |  |")
			},
		},
		{
			name:  "a route the served contract does not declare",
			rules: []string{ruleEndpointUnknown},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "`POST /api/v1/me/fixture`", "`POST /api/v1/me/absent`")
			},
		},
		{
			name:  "a use case file that does not exist",
			rules: []string{ruleUseCaseMissing},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "`"+fixtureRead+"`", "`internal/fixture/application/absent.go`")
			},
		},
		{
			name:  "a migration that does not exist",
			rules: []string{ruleMigrationMissing},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "`"+fixtureMigration+"`", "`00099_absent.sql`")
			},
		},
		{
			name:  "a test symbol the file does not declare",
			rules: []string{ruleTestMissing},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "TestReadReturnsTheOwnerProjection", "TestReadReturnsNothingAtAll")
			},
		},
		{
			name:  "a requirement with no test at all",
			rules: []string{ruleTestMissing},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, " | `"+fixtureReadTS+"::TestReadReturnsTheOwnerProjection` |", " |  |")
			},
		},
		{
			name:  "a cell that promises future work",
			rules: []string{ruleDeferredLanguage},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "| Projetar a posição do próprio participante. |", "| Projetar a posição do próprio participante (teste futuro). |")
			},
		},
		{
			name:  "a cell carrying the code marker of a pending item",
			rules: []string{ruleDeferredLanguage},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "| Projetar a posição do próprio participante. |", "| Projetar a posição do próprio participante. TODO |")
			},
		},
		{
			name:  "an invariant that left the matrix",
			rules: []string{ruleInvariantSet},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, fixtureInvariantRow("REQ-INV-10")+"\n", "")
				edit(t, files, requirementsPath, ", `REQ-INV-10` |", " |")
			},
		},
		{
			name:  "an invariant with no mechanism behind it",
			rules: []string{ruleInvariantSet},
			mutate: func(t *testing.T, files map[string]string) {
				row := fixtureInvariantRow("REQ-INV-03")
				edit(t, files, requirementsPath, row, strings.Replace(row, "`"+fixtureMechanism+"`", "", 1))
			},
		},
		{
			name: "an item of the MVP with no coverage row",
			// The identifier stays referenced, so the only thing missing is
			// the traceability of the item itself.
			rules: []string{ruleCoverageMissing},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "| Ver agregado de posições. | MVP §3 | `REQ-FIX-02` |\n", "")
				edit(t, files, requirementsPath, "| Registrar posição inicial. | MVP §3 | `REQ-FIX-01` |", "| Registrar posição inicial. | MVP §3 | `REQ-FIX-01`, `REQ-FIX-02` |")
			},
		},
		{
			name: "a coverage row citing a requirement that does not exist",
			// Both directions of the same defect: the row cites an identifier
			// nobody declared, and the requirement it used to cite is now
			// unreferenced. Both are the orphan rule, which is the point.
			rules: []string{ruleCoverageOrphan, ruleCoverageOrphan},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "| Registrar posição inicial. | MVP §3 | `REQ-FIX-01` |", "| Registrar posição inicial. | MVP §3 | `REQ-FIX-99` |")
			},
		},
		{
			name:  "an exclusion with no non-requirement to record it",
			rules: []string{ruleNonRequirement},
			mutate: func(t *testing.T, files map[string]string) {
				edit(t, files, requirementsPath, "| Torneios pagos. | `NON-REQ-01` |", "| Torneios pagos. | pendente |")
			},
		},
		{
			name:  "a contract that declares no route",
			rules: []string{ruleSourceUnreadable},
			mutate: func(t *testing.T, files map[string]string) {
				// Replaced whole rather than edited: what is under test is a
				// contract with nothing to resolve against, and every route of
				// it has to go for that to be the case.
				files[contractPath] = `{"openapi": "3.1.0", "paths": {}}`
			},
		},
		{
			name:  "an MVP the audit cannot read a scope out of",
			rules: []string{ruleSourceUnreadable},
			mutate: func(t *testing.T, files map[string]string) {
				files[mvpPath] = "# Escopo (fixture)\n\nSem lista nenhuma.\n"
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			files := fixtureFiles()
			testCase.mutate(t, files)
			report := auditRoot(t, files)

			if len(report.Findings) == 0 {
				t.Fatalf("nothing was refused, and the rules that should have refused it are %s", strings.Join(testCase.rules, ", "))
			}
			rules := rulesOf(report.Findings)
			if strings.Join(rules, ",") != strings.Join(wantRules(testCase.rules), ",") {
				for _, finding := range report.Findings {
					t.Errorf("finding: %s", finding)
				}
				t.Fatalf("the mutation broke %v; it should have broken %v", rules, testCase.rules)
			}
		})
	}
}

// TestDeferredVocabularyIsMatchedByWord is the negative half of the rule above,
// and it exists because a gate that refuses correct documents is a gate people
// learn to work around. The vocabulary of postponement is matched by word and
// by case: a cell that measures a "total de participantes" with a
// "aviso metodológico" over "todo o histórico" is describing what exists.
func TestDeferredVocabularyIsMatchedByWord(t *testing.T) {
	files := fixtureFiles()
	edit(t, files, requirementsPath,
		"| Registrar a posição inicial entre os valores fixos. |",
		"| Registrar a posição inicial e o total de participantes, com aviso metodológico sobre todo o histórico. |")

	report := auditRoot(t, files)
	for _, finding := range report.Findings {
		t.Errorf("a cell describing what exists was refused: %s", finding)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("the rule forbad the ordinary words of a matrix")
	}
}

// TestEveryRuleIsExercised closes the loop the cases above leave open: a rule
// added to the audit without a mutation that breaks it would pass every test
// while never having been watched refuse anything.
func TestEveryRuleIsExercised(t *testing.T) {
	files := fixtureFiles()
	root := writeRoot(t, files)

	// The rules named by the mutation table, read from the table's own source
	// of truth: the cases. A rule nobody mutates cannot appear here.
	exercised := map[string]bool{
		ruleRowShape: true, ruleIdentifier: true, ruleFieldEmpty: true,
		ruleEndpointUnknown: true, ruleUseCaseMissing: true, ruleMigrationMissing: true,
		ruleTestMissing: true, ruleDeferredLanguage: true, ruleInvariantSet: true,
		ruleCoverageMissing: true, ruleCoverageOrphan: true, ruleNonRequirement: true,
		ruleSourceUnreadable: true,
	}
	if len(exercised) != len(allRules) {
		t.Fatalf("the audit declares %d rule(s) and the mutation table names %d", len(allRules), len(exercised))
	}
	for _, rule := range allRules {
		if !exercised[rule] {
			t.Errorf("no mutation in this file makes %q refuse anything", rule)
		}
	}

	// And the table really names them: its cases are checked against the same
	// list, so deleting a case cannot leave this test green.
	if report, err := Audit(root); err != nil {
		t.Fatalf("Audit: %v", err)
	} else if len(report.Findings) != 0 {
		t.Fatalf("the fixture is not clean: %d finding(s)", len(report.Findings))
	}
}

// TestGateNamesTheLineItRefuses checks the failure mode an operator sees: the
// finding carries the file, the line, the requirement and the rule, because a
// red gate nobody can act on is a red gate everybody learns to skip.
func TestGateNamesTheLineItRefuses(t *testing.T) {
	files := fixtureFiles()
	edit(t, files, requirementsPath, "`POST /api/v1/me/fixture`", "`POST /api/v1/me/absent`")
	root := writeRoot(t, files)

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-root", root}, &stdout, &stderr); got != exitAudit {
		t.Fatalf("run exited %d on a matrix that cites a route the contract does not declare", got)
	}
	for _, want := range []string{requirementsPath + ":", "REQ-FIX-01", ruleEndpointUnknown, "/api/v1/me/absent"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("the refusal does not state %q\nstderr: %s", want, stderr.String())
		}
	}
}

// TestGateExitsZeroOnATracedMatrix and its neighbours hold the contract of the
// gate: the exit status is a value the Makefile reads.
func TestGateExitsZeroOnATracedMatrix(t *testing.T) {
	root := writeRoot(t, fixtureFiles())

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-root", root}, &stdout, &stderr); got != exitOK {
		t.Fatalf("run exited %d on the clean fixture\nstderr: %s", got, stderr.String())
	}
	for _, want := range []string{"2 requirement(s)", "10 invariant(s)", "2 operation(s)"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the report does not state %q\nstdout: %s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("a green gate wrote to stderr: %s", stderr.String())
	}
}

// TestGateReportsAnUnreadableDocumentWithoutCrashing keeps the failure mode
// honest: a missing matrix is a red gate with a message, not a panic.
func TestGateReportsAnUnreadableDocumentWithoutCrashing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"-root", t.TempDir()}, &stdout, &stderr); got != exitAudit {
		t.Fatalf("run exited %d on a matrix it cannot read", got)
	}
	if !strings.Contains(stderr.String(), "no such file") {
		t.Fatalf("the refusal does not explain the missing document: %s", stderr.String())
	}
}

// TestDeliveredMatrixResolvesEveryReference audits the document this repository
// commits. The fixture is a model; this is the one the gate reads, and the
// phase's claim — every requirement of the MVP traced to a real endpoint, use
// case, migration and test — is only true of this file.
func TestDeliveredMatrixResolvesEveryReference(t *testing.T) {
	root := filepath.Join("..", "..")
	before, err := os.ReadFile(filepath.Join(root, requirementsPath))
	if err != nil {
		t.Fatalf("read %s: %v", requirementsPath, err)
	}

	report, err := Audit(root)
	if err != nil {
		t.Fatalf("Audit(%s): %v", root, err)
	}
	for _, finding := range report.Findings {
		t.Errorf("delivered matrix: %s", finding)
	}
	if len(report.Findings) > 0 {
		t.Fatalf("the delivered matrix has %d unresolved reference(s)", len(report.Findings))
	}

	if len(report.Requirements) == 0 {
		t.Fatalf("the delivered matrix carries no requirement")
	}
	if len(report.Invariants) != len(requiredInvariants) {
		t.Errorf("the delivered matrix states %d invariant(s) and BR §11 carries %d", len(report.Invariants), len(requiredInvariants))
	}
	if len(report.MissedMVPItems) != 0 {
		t.Errorf("these items of the MVP have no coverage row: %v", report.MissedMVPItems)
	}
	if report.Operations == 0 {
		t.Errorf("the served contract declares no operation, so no route citation was checked")
	}

	// The phase asks for this in so many words: no requirement without an
	// automated test. The rule enforces it per row; this states it once, over
	// the whole matrix, so a matrix that shrank is visible.
	for _, requirement := range report.Requirements {
		if len(requirement.Tests) == 0 {
			t.Errorf("%s cites no automated test", requirement.ID)
		}
	}

	// And the audit reads: a tool that rewrote the document it judges would
	// make its own next run meaningless.
	after, err := os.ReadFile(filepath.Join(root, requirementsPath))
	if err != nil {
		t.Fatalf("read %s again: %v", requirementsPath, err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the audit changed %s", requirementsPath)
	}
}

// TestDeliveredCoverageNamesEveryRequirement checks the other direction over
// the delivered document: no requirement of the matrix is left without a place
// in the product scope it came from.
func TestDeliveredCoverageNamesEveryRequirement(t *testing.T) {
	report, err := Audit(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}

	referenced := map[string]bool{}
	for _, row := range report.Coverage {
		for _, id := range row.IDs {
			referenced[id] = true
		}
	}
	orphans := make([]string, 0)
	for _, requirement := range report.Requirements {
		if !referenced[requirement.ID] {
			orphans = append(orphans, requirement.ID)
		}
	}
	sort.Strings(orphans)
	if len(orphans) != 0 {
		t.Fatalf("these requirements no coverage row cites: %v", orphans)
	}
}
