// Rules of the requirements traceability audit (P20-T02).
//
// The phase demands that every requirement of the MVP be traceable, and that no
// critical requirement lack an automated test. A matrix that *says* so is a
// wish: the whole failure mode of a traceability document is that it keeps
// looking right after the code moves. This audit makes each claim mechanical —
// it resolves every reference of the matrix against the files that exist, the
// contract that is served and the test functions that are really there — so a
// link that rots fails `make verify` instead of misleading the next reader.
//
// It reads and judges; it never writes. The fix belongs to whoever changes the
// code or the document.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The files the matrix is checked against.
const (
	// requirementsPath is the document under audit.
	requirementsPath = "docs/REQUIREMENTS.md"
	// mvpPath is the scope the coverage table has to exhaust.
	mvpPath = "docs/MVP.md"
	// contractPath is the served contract: a route cited by the matrix is a
	// route an operator can call.
	contractPath = "api/openapi.json"
	// migrationsDir holds the forward-only migrations of the platform.
	migrationsDir = "internal/platform/dbmigrate/migrations"
)

// requiredInvariants is the closed set of business invariants of
// docs/BUSINESS_RULES.md §11. Hardcoded on purpose: dropping one from the
// matrix, or renaming it, has to be a deliberate change here.
var requiredInvariants = []string{
	"REQ-INV-01", "REQ-INV-02", "REQ-INV-03", "REQ-INV-04", "REQ-INV-05",
	"REQ-INV-06", "REQ-INV-07", "REQ-INV-08", "REQ-INV-09", "REQ-INV-10",
}

// coreColumns and invariantColumns are the widths of the two matrix tables.
const (
	coreColumns      = 9
	invariantColumns = 7
	coverageColumns  = 3
	nonReqColumns    = 2
)

// The rules this audit enforces. Each one exists because its absence would let
// an unverifiable claim pass as a traced requirement.
const (
	// ruleRowShape: a row has the declared number of columns, so a parser and a
	// reader agree on what they are looking at.
	ruleRowShape = "row_shape"
	// ruleIdentifier: identifiers follow REQ-<CAT>-<NN>, are stable and unique.
	ruleIdentifier = "identifier"
	// ruleFieldEmpty: description, origin, module and phase say something.
	ruleFieldEmpty = "field_empty"
	// ruleEndpointUnknown: a cited route exists in the served contract with
	// that method, or the cell declares the absence with a reason.
	ruleEndpointUnknown = "endpoint_unknown"
	// ruleUseCaseMissing: a cited use case is a path that exists.
	ruleUseCaseMissing = "use_case_missing"
	// ruleMigrationMissing: a cited migration exists in the migrations
	// directory.
	ruleMigrationMissing = "migration_missing"
	// ruleTestMissing: a cited test exists — the file and the function — and
	// every requirement cites at least one. This is the rule that answers
	// "nenhum requisito crítico sem teste automatizado".
	ruleTestMissing = "test_missing"
	// ruleDeferredLanguage: no cell promises future work. A "teste futuro" is
	// exactly the placeholder this revision removed.
	ruleDeferredLanguage = "deferred_language"
	// ruleInvariantSet: the ten invariants of BUSINESS_RULES §11 are all
	// present, each with the mechanism, the migration and the test that
	// sustains it.
	ruleInvariantSet = "invariant_set"
	// ruleCoverageMissing: every item of the MVP scope has a coverage row, so
	// the matrix cannot quietly shrink the product.
	ruleCoverageMissing = "coverage_missing"
	// ruleCoverageOrphan: a referenced requirement exists, and every
	// requirement of the matrix is referenced somewhere — bidirectionality.
	ruleCoverageOrphan = "coverage_orphan"
	// ruleNonRequirement: every item the MVP excludes has a NON-REQ row, so an
	// exclusion cannot become an undecided.
	ruleNonRequirement = "non_requirement"
	// ruleSourceUnreadable: the documents this audit depends on must yield
	// what it reads. A parser that silently finds nothing would pass every
	// rule over an empty set.
	ruleSourceUnreadable = "source_unreadable"
)

// allRules is every rule this audit applies. It exists so the tests can prove
// that each one has been seen to refuse something: a rule nobody has watched
// fail is a rule nobody has verified, and this tool is the gate of `make
// verify`.
var allRules = []string{
	ruleRowShape, ruleIdentifier, ruleFieldEmpty, ruleEndpointUnknown,
	ruleUseCaseMissing, ruleMigrationMissing, ruleTestMissing,
	ruleDeferredLanguage, ruleInvariantSet, ruleCoverageMissing,
	ruleCoverageOrphan, ruleNonRequirement, ruleSourceUnreadable,
}

// Requirement is one functional requirement of the matrix.
type Requirement struct {
	ID          string
	Description string
	Origin      string
	Module      string
	Phase       string
	Endpoints   []string
	UseCases    []string
	Migrations  []string
	Tests       []string
	Line        int
	// Shape is the number of columns the row carries when that differs from
	// the declared width, and zero when the row is well formed.
	Shape int
}

// Invariant is one business invariant of the matrix.
type Invariant struct {
	ID         string
	Statement  string
	Module     string
	Phase      string
	Mechanism  string
	Migrations []string
	Tests      []string
	Line       int
	// Shape mirrors Requirement.Shape for the invariant table.
	Shape int
}

// CoverageRow is one item of the source documents mapped to requirements.
type CoverageRow struct {
	Item         string
	Origin       string
	Requirements string
	IDs          []string
	Line         int
}

// NonRequirement is one item the MVP excludes.
type NonRequirement struct {
	Item string
	// ID is the NON-REQ identifier the row states, empty when it states none.
	ID string
	// IDCell is the whole second column, so a row that states no identifier
	// can be reported with what it does say.
	IDCell string
	Line   int
}

// Finding is one rule violation, addressed to the line that caused it.
type Finding struct {
	Line   int
	Topic  string
	Rule   string
	Detail string
}

// String renders a finding the way the other auditors do.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s: %s", requirementsPath, f.Line, f.Topic, f.Rule, f.Detail)
}

// Report is what the audit read and what it concluded.
type Report struct {
	Requirements    []Requirement
	Invariants      []Invariant
	Coverage        []CoverageRow
	NonRequirements []NonRequirement
	// MissedMVPItems are the items of docs/MVP.md §3 no coverage row mentions.
	MissedMVPItems []string
	// Operations is how many routes the contract declares.
	Operations int
	Findings   []Finding
}

// Audit reads the repository and applies every rule.
func Audit(root string) (Report, error) {
	report := Report{}

	document, err := os.ReadFile(filepath.Join(root, requirementsPath))
	if err != nil {
		return report, fmt.Errorf("read %s: %w", requirementsPath, err)
	}
	report.Requirements, report.Invariants, report.Coverage, report.NonRequirements = parseMatrix(string(document))

	contract, operations, err := readContract(filepath.Join(root, contractPath))
	if err != nil {
		report.Findings = append(report.Findings, Finding{Rule: ruleSourceUnreadable, Detail: err.Error()})
		return report, nil
	}
	report.Operations = operations

	mvpScope, mvpExcluded, err := readMVP(filepath.Join(root, mvpPath))
	if err != nil {
		report.Findings = append(report.Findings, Finding{Rule: ruleSourceUnreadable, Detail: err.Error()})
		return report, nil
	}
	if len(mvpScope) == 0 || len(mvpExcluded) == 0 {
		report.Findings = append(report.Findings, Finding{
			Rule:   ruleSourceUnreadable,
			Detail: fmt.Sprintf("%s yielded %d included and %d excluded items; a scope this audit cannot read is a scope it cannot check", mvpPath, len(mvpScope), len(mvpExcluded)),
		})
	}

	report.Findings = append(report.Findings, auditRequirements(root, report.Requirements, contract, mvpScope)...)
	report.Findings = append(report.Findings, auditInvariants(root, report.Invariants)...)
	report.Findings = append(report.Findings, auditCoverage(root, report.Requirements, report.Invariants, report.Coverage, mvpScope)...)
	report.Findings = append(report.Findings, auditNonRequirements(root, report.NonRequirements, mvpExcluded)...)
	report.MissedMVPItems = missedMVPItems(mvpScope, report.Coverage)

	sort.SliceStable(report.Findings, func(i, j int) bool {
		if report.Findings[i].Line != report.Findings[j].Line {
			return report.Findings[i].Line < report.Findings[j].Line
		}
		return report.Findings[i].Rule < report.Findings[j].Rule
	})
	return report, nil
}

// ---------------------------------------------------------------- parsing

var (
	idPattern      = regexp.MustCompile(`^REQ-[A-Z0-9]+-\d\d$`)
	anyIDPattern   = regexp.MustCompile(`REQ-[A-Z0-9]+-\d\d`)
	nonReqPattern  = regexp.MustCompile(`NON-REQ-\d\d`)
	bulletPattern  = regexp.MustCompile(`^-\s+(.*)$`)
	sectionPattern = regexp.MustCompile(`^##\s+(\d+)\.`)
)

// parseMatrix reads the four tables of the document. It is deliberately
// positional: the sections are numbered, the tables are the only tables of
// their section, and a row that does not have the declared width is reported
// instead of guessed at.
func parseMatrix(document string) ([]Requirement, []Invariant, []CoverageRow, []NonRequirement) {
	var (
		requirements []Requirement
		invariants   []Invariant
		coverage     []CoverageRow
		nonReqs      []NonRequirement
		section      = 0
		line         = 0
		// headers remembers the sections whose table header was already
		// consumed. §2 carries one table per module, and those headers are
		// skipped by the REQ- prefix; §4 and §5 carry one table each, and
		// their header is consumed here.
		headers = map[int]bool{}
	)

	for _, raw := range strings.Split(document, "\n") {
		line++
		text := strings.TrimRight(raw, "\r")

		if match := sectionPattern.FindStringSubmatch(text); match != nil {
			fmt.Sscanf(match[1], "%d", &section)
			continue
		}
		if !strings.HasPrefix(text, "|") {
			continue
		}
		cells := splitRow(text)
		if cells == nil {
			continue
		}

		switch section {
		case 2:
			if !strings.HasPrefix(cells[0], "REQ-") {
				continue
			}
			if strings.HasPrefix(cells[0], "REQ-INV-") {
				// The invariants are the table of §3; the reference to them in
				// prose is not a row of the functional matrix.
				continue
			}
			if len(cells) != coreColumns {
				requirements = append(requirements, Requirement{ID: cells[0], Line: line, Shape: len(cells)})
				continue
			}
			requirements = append(requirements, Requirement{
				ID:          cells[0],
				Description: cells[1],
				Origin:      cells[2],
				Module:      cells[3],
				Phase:       cells[4],
				Endpoints:   splitCell(cells[5]),
				UseCases:    splitCell(cells[6]),
				Migrations:  splitCell(cells[7]),
				Tests:       splitCell(cells[8]),
				Line:        line,
			})
		case 3:
			if !strings.HasPrefix(cells[0], "REQ-INV-") {
				continue
			}
			if len(cells) != invariantColumns {
				invariants = append(invariants, Invariant{ID: cells[0], Line: line, Shape: len(cells)})
				continue
			}
			invariants = append(invariants, Invariant{
				ID:         cells[0],
				Statement:  cells[1],
				Module:     cells[2],
				Phase:      cells[3],
				Mechanism:  cells[4],
				Migrations: splitCell(cells[5]),
				Tests:      splitCell(cells[6]),
				Line:       line,
			})
		case 4, 5:
			if !headers[section] {
				headers[section] = true
				continue
			}
			if len(cells) != coverageColumns && len(cells) != nonReqColumns {
				continue
			}
			if section == 4 {
				coverage = append(coverage, CoverageRow{
					Item:         cells[0],
					Origin:       cells[1],
					Requirements: cells[2],
					IDs:          anyIDPattern.FindAllString(cells[2], -1),
					Line:         line,
				})
				continue
			}
			if len(cells) != nonReqColumns {
				continue
			}
			id := nonReqPattern.FindString(cells[1])
			nonReqs = append(nonReqs, NonRequirement{Item: cells[0], ID: id, IDCell: cells[1], Line: line})
		}
	}
	return requirements, invariants, coverage, nonReqs
}

// splitRow breaks a Markdown table row into its cells, dropping the empty
// edges. A separator row (|---|) is not a row.
func splitRow(text string) []string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil
	}
	parts := strings.Split(trimmed[1:len(trimmed)-1], "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, unstyle(strings.TrimSpace(part)))
	}
	if isSeparator(cells) {
		return nil
	}
	return cells
}

// isSeparator reports whether every non-empty cell is a run of dashes and
// colons — the row that draws the table.
func isSeparator(cells []string) bool {
	for _, cell := range cells {
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}

// unstyle strips the emphasis and the code fences a cell uses for the reader,
// so the value compared is the value meant. Backticks are markup and never
// content — a path or an identifier never contains one — so they all go.
func unstyle(cell string) string {
	return strings.Trim(strings.ReplaceAll(cell, "`", ""), "* ")
}

// splitCell breaks a multi-valued cell into its parts, each one stripped of
// its own emphasis and code fence.
func splitCell(cell string) []string {
	parts := strings.Split(cell, "<br>")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := unstyle(strings.TrimSpace(part)); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// readContract reads the served contract into a path → method set, and counts
// its operations.
func readContract(path string) (map[string]map[string]bool, int, error) {
	document, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", contractPath, err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(document, &spec); err != nil {
		return nil, 0, fmt.Errorf("parse %s: %w", contractPath, err)
	}
	contract := make(map[string]map[string]bool, len(spec.Paths))
	operations := 0
	for route, methods := range spec.Paths {
		contract[route] = map[string]bool{}
		for method := range methods {
			lower := strings.ToLower(method)
			switch lower {
			case "get", "post", "put", "patch", "delete", "head", "options":
				contract[route][lower] = true
				operations++
			}
		}
	}
	if len(contract) == 0 {
		return nil, 0, fmt.Errorf("%s declares no paths", contractPath)
	}
	return contract, operations, nil
}

// readMVP reads the two lists of docs/MVP.md: what the MVP includes (§3) and
// what it excludes (§4).
func readMVP(path string) (included, excluded []string, err error) {
	document, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", mvpPath, err)
	}
	section := 0
	for _, raw := range strings.Split(string(document), "\n") {
		text := strings.TrimRight(raw, "\r")
		if match := sectionPattern.FindStringSubmatch(text); match != nil {
			fmt.Sscanf(match[1], "%d", &section)
			continue
		}
		if match := bulletPattern.FindStringSubmatch(strings.TrimSpace(text)); match != nil {
			switch section {
			case 3:
				// A bullet of §3 that introduces the pilot protocol ("O piloto
				// deve testar:") belongs to the narrative, not to the scope:
				// §3's scope bullets live under its ### subsections.
				included = append(included, match[1])
			case 4:
				excluded = append(excluded, match[1])
			}
		}
	}
	return included, excluded, nil
}

// ------------------------------------------------------------------- rules

func auditRequirements(root string, requirements []Requirement, contract map[string]map[string]bool, mvpScope []string) []Finding {
	findings := make([]Finding, 0)
	if len(requirements) == 0 {
		return []Finding{{Rule: ruleSourceUnreadable, Detail: "the matrix carries no requirement row; a matrix this audit cannot read is one it cannot check"}}
	}

	seen := map[string]int{}
	for _, requirement := range requirements {
		topic := requirement.ID
		// A malformed row is reported for what it is and judged no further.
		if requirement.Shape != 0 {
			findings = append(findings, Finding{Line: requirement.Line, Topic: topic, Rule: ruleRowShape, Detail: fmt.Sprintf("a linha tem %d coluna(s) e a matriz declara %d", requirement.Shape, coreColumns)})
			continue
		}

		if !idPattern.MatchString(requirement.ID) {
			findings = append(findings, Finding{Line: requirement.Line, Topic: topic, Rule: ruleIdentifier, Detail: fmt.Sprintf("%q is not REQ-<CATEGORIA>-<NN>", requirement.ID)})
		}
		seen[requirement.ID]++
		if seen[requirement.ID] > 1 {
			findings = append(findings, Finding{Line: requirement.Line, Topic: topic, Rule: ruleIdentifier, Detail: "the matrix states this requirement more than once"})
		}
		for name, value := range map[string]string{
			"descrição": requirement.Description,
			"origem":    requirement.Origin,
			"módulo":    requirement.Module,
			"fase":      requirement.Phase,
		} {
			if value == "" {
				findings = append(findings, Finding{Line: requirement.Line, Topic: topic, Rule: ruleFieldEmpty, Detail: "a coluna " + name + " está vazia"})
			}
		}

		findings = append(findings, auditEndpoints(requirement, contract)...)
		findings = append(findings, auditPaths(root, requirement, ruleUseCaseMissing, "caso de uso", requirement.UseCases)...)
		findings = append(findings, auditMigrations(root, requirement.Line, topic, requirement.Migrations)...)
		findings = append(findings, auditTests(root, requirement.Line, topic, requirement.Tests)...)
		findings = append(findings, auditLanguage(requirement)...)
	}
	return findings
}

// auditEndpoints resolves each cited route against the served contract.
func auditEndpoints(requirement Requirement, contract map[string]map[string]bool) []Finding {
	findings := make([]Finding, 0)
	if len(requirement.Endpoints) == 0 {
		return []Finding{{Line: requirement.Line, Topic: requirement.ID, Rule: ruleEndpointUnknown, Detail: "a célula de endpoint está vazia: cite a rota ou declare a ausência com uma razão"}}
	}
	for _, endpoint := range requirement.Endpoints {
		if reason, absent := absence(endpoint); absent {
			if reason == "" {
				findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: ruleEndpointUnknown, Detail: "a ausência de rota foi declarada sem razão"})
			}
			continue
		}
		fields := strings.Fields(endpoint)
		if len(fields) != 2 {
			findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: ruleEndpointUnknown, Detail: fmt.Sprintf("%q não é \"MÉTODO /rota\" nem \"— (razão)\"", endpoint)})
			continue
		}
		method, route := strings.ToLower(fields[0]), fields[1]
		methods, known := contract[route]
		if !known {
			findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: ruleEndpointUnknown, Detail: fmt.Sprintf("a rota %s não existe em %s", route, contractPath)})
			continue
		}
		if !methods[method] {
			findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: ruleEndpointUnknown, Detail: fmt.Sprintf("%s %s não existe: a rota admite %s", strings.ToUpper(method), route, methodList(methods))})
		}
	}
	return findings
}

// auditPaths checks that cited files exist.
func auditPaths(root string, requirement Requirement, rule, column string, values []string) []Finding {
	findings := make([]Finding, 0)
	if len(values) == 0 {
		return []Finding{{Line: requirement.Line, Topic: requirement.ID, Rule: rule, Detail: "a célula de " + column + " está vazia: cite o arquivo ou declare a ausência com uma razão"}}
	}
	for _, value := range values {
		if reason, absent := absence(value); absent {
			if reason == "" {
				findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: rule, Detail: "a ausência de " + column + " foi declarada sem razão"})
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(root, value)); err != nil {
			findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: rule, Detail: fmt.Sprintf("o %s %s não existe", column, value)})
		}
	}
	return findings
}

// auditMigrations checks that cited migrations are files of the migrations
// directory. The cell carries the file name, so a migration that is renamed
// breaks the claim instead of dangling.
func auditMigrations(root string, line int, topic string, migrations []string) []Finding {
	findings := make([]Finding, 0)
	if len(migrations) == 0 {
		return []Finding{{Line: line, Topic: topic, Rule: ruleMigrationMissing, Detail: "a célula de migration está vazia: cite o arquivo ou declare a ausência com uma razão"}}
	}
	for _, migration := range migrations {
		if reason, absent := absence(migration); absent {
			if reason == "" {
				findings = append(findings, Finding{Line: line, Topic: topic, Rule: ruleMigrationMissing, Detail: "a ausência de migration foi declarada sem razão"})
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(root, migrationsDir, migration)); err != nil {
			findings = append(findings, Finding{Line: line, Topic: topic, Rule: ruleMigrationMissing, Detail: fmt.Sprintf("%s/%s não existe", migrationsDir, migration)})
		}
	}
	return findings
}

// auditTests resolves every `path::symbol` citation. This is the rule that
// answers the phase: a requirement with no existing automated test is refused.
func auditTests(root string, line int, topic string, tests []string) []Finding {
	findings := make([]Finding, 0)
	if len(tests) == 0 {
		return []Finding{{Line: line, Topic: topic, Rule: ruleTestMissing, Detail: "nenhum teste citado: um requisito sem teste automatizado existente não está rastreado"}}
	}
	for _, test := range tests {
		parts := strings.SplitN(test, "::", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			findings = append(findings, Finding{Line: line, Topic: topic, Rule: ruleTestMissing, Detail: fmt.Sprintf("%q não é \"caminho::FunçãoDeTeste\"", test)})
			continue
		}
		path, symbol := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		body, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			findings = append(findings, Finding{Line: line, Topic: topic, Rule: ruleTestMissing, Detail: fmt.Sprintf("o arquivo de teste %s não existe", path)})
			continue
		}
		if !declaresTest(string(body), path, symbol) {
			findings = append(findings, Finding{Line: line, Topic: topic, Rule: ruleTestMissing, Detail: fmt.Sprintf("%s não declara o teste %q", path, symbol)})
		}
	}
	return findings
}

// declaresTest reports whether a file declares the named test. Go tests are
// functions; the browser journeys of tools/e2e are test(...) calls whose title
// is the name cited.
func declaresTest(body, path, symbol string) bool {
	if strings.HasSuffix(path, ".go") {
		return strings.Contains(body, "func "+symbol+"(")
	}
	for _, quote := range []string{`"`, `'`, "`"} {
		if strings.Contains(body, "test("+quote+symbol+quote) {
			return true
		}
	}
	return false
}

// deferredWords is the vocabulary of postponement. Two details keep it from
// refusing a document that is telling the truth about what exists:
//
//   - it matches whole words, so "metodológico" (which carries "todo") and
//     "total" pass;
//   - the code markers are matched in upper case only, because "TODO" and
//     "TBD" are conventions of source code, while a Portuguese matrix writes
//     the ordinary word "todo" — "todo requisito" — without postponing
//     anything.
//
// The case-insensitive flag is scoped to its own alternation: written bare it
// would reach the second one, which is exactly the mistake this comment is
// here to keep from coming back.
var deferredWords = regexp.MustCompile(`(?i:\b(futuro|a definir|a implementar)\b)|\b(TODO|TBD)\b`)

// auditLanguage refuses the vocabulary of postponement in a cell. Its presence
// is how a matrix drifts back to listing intentions.
func auditLanguage(requirement Requirement) []Finding {
	findings := make([]Finding, 0)
	cells := append([]string{requirement.Description, requirement.Origin, requirement.Module, requirement.Phase}, requirement.Endpoints...)
	cells = append(cells, requirement.UseCases...)
	cells = append(cells, requirement.Migrations...)
	cells = append(cells, requirement.Tests...)
	for _, cell := range cells {
		if deferredWords.MatchString(cell) {
			findings = append(findings, Finding{Line: requirement.Line, Topic: requirement.ID, Rule: ruleDeferredLanguage, Detail: fmt.Sprintf("a célula %q adia trabalho em vez de apontar o que existe", cell)})
		}
	}
	return findings
}

// auditInvariants requires the closed set of invariants, each with the
// mechanism that sustains it, the migration it lives in and the test that
// verifies it.
func auditInvariants(root string, invariants []Invariant) []Finding {
	findings := make([]Finding, 0)
	stated := map[string]bool{}
	for _, invariant := range invariants {
		stated[invariant.ID] = true
		if invariant.Shape != 0 {
			findings = append(findings, Finding{Line: invariant.Line, Topic: invariant.ID, Rule: ruleRowShape, Detail: fmt.Sprintf("a linha tem %d coluna(s) e a tabela de invariantes declara %d", invariant.Shape, invariantColumns)})
			continue
		}
		if invariant.Statement == "" || invariant.Mechanism == "" {
			findings = append(findings, Finding{Line: invariant.Line, Topic: invariant.ID, Rule: ruleInvariantSet, Detail: "a invariante não declara o enunciado e o mecanismo que a sustenta"})
		}
		findings = append(findings, auditMigrations(root, invariant.Line, invariant.ID, invariant.Migrations)...)
		findings = append(findings, auditTests(root, invariant.Line, invariant.ID, invariant.Tests)...)
	}
	for _, id := range requiredInvariants {
		if !stated[id] {
			findings = append(findings, Finding{Topic: id, Rule: ruleInvariantSet, Detail: "a invariante de BR §11 não está na matriz"})
		}
	}
	return findings
}

// auditCoverage requires bidirectionality: every requirement is referenced by
// a coverage row, and every reference resolves.
func auditCoverage(root string, requirements []Requirement, invariants []Invariant, coverage []CoverageRow, mvpScope []string) []Finding {
	findings := make([]Finding, 0)
	if len(coverage) == 0 {
		return []Finding{{Rule: ruleSourceUnreadable, Detail: "a tabela de cobertura está vazia"}}
	}

	known := map[string]bool{}
	for _, requirement := range requirements {
		known[requirement.ID] = true
	}
	for _, invariant := range invariants {
		known[invariant.ID] = true
	}

	referenced := map[string]bool{}
	for _, row := range coverage {
		if row.Item == "" || row.Origin == "" || row.Requirements == "" {
			findings = append(findings, Finding{Line: row.Line, Rule: ruleCoverageOrphan, Detail: "a linha de cobertura tem coluna vazia"})
			continue
		}
		if len(row.IDs) == 0 {
			findings = append(findings, Finding{Line: row.Line, Rule: ruleCoverageOrphan, Detail: fmt.Sprintf("o item %q não mapeia requisito nenhum", row.Item)})
			continue
		}
		for _, id := range row.IDs {
			if !known[id] {
				findings = append(findings, Finding{Line: row.Line, Rule: ruleCoverageOrphan, Detail: fmt.Sprintf("o item %q cita %s, que não existe na matriz", row.Item, id)})
				continue
			}
			if !referenced[id] {
				referenced[id] = true
			}
		}
	}

	for _, id := range sortedKeys(known) {
		if !referenced[id] {
			findings = append(findings, Finding{Topic: id, Rule: ruleCoverageOrphan, Detail: "o requisito existe na matriz e nenhum item de origem o cita"})
		}
	}
	for _, item := range mvpScope {
		if !coverageMentions(coverage, item) {
			findings = append(findings, Finding{Rule: ruleCoverageMissing, Detail: fmt.Sprintf("o item de %s não tem linha de cobertura: %q", mvpPath, item)})
		}
	}
	return findings
}

// auditNonRequirements requires that every excluded item is recorded as such.
func auditNonRequirements(root string, nonReqs []NonRequirement, mvpExcluded []string) []Finding {
	findings := make([]Finding, 0)
	if len(nonReqs) == 0 {
		return []Finding{{Rule: ruleSourceUnreadable, Detail: "a tabela de não-requisitos está vazia"}}
	}
	seen := map[string]int{}
	for _, nonReq := range nonReqs {
		if nonReq.ID == "" {
			findings = append(findings, Finding{Line: nonReq.Line, Rule: ruleNonRequirement, Detail: fmt.Sprintf("%q não nomeia um NON-REQ-<NN>", nonReq.IDCell)})
			continue
		}
		seen[nonReq.ID]++
		if seen[nonReq.ID] > 1 {
			findings = append(findings, Finding{Line: nonReq.Line, Rule: ruleNonRequirement, Detail: "o não-requisito aparece mais de uma vez"})
		}
	}
	for _, item := range mvpExcluded {
		found := false
		for _, nonReq := range nonReqs {
			if equalItem(nonReq.Item, item) {
				found = true
				break
			}
		}
		if !found {
			findings = append(findings, Finding{Rule: ruleNonRequirement, Detail: fmt.Sprintf("o item excluído em %s §4 não tem não-requisito: %q", mvpPath, item)})
		}
	}
	return findings
}

func missedMVPItems(mvpScope []string, coverage []CoverageRow) []string {
	missed := make([]string, 0)
	for _, item := range mvpScope {
		if !coverageMentions(coverage, item) {
			missed = append(missed, item)
		}
	}
	return missed
}

// coverageMentions reports whether a coverage row states the item verbatim.
// The comparison ignores the punctuation that separates bullets and the case,
// because the table quotes the document rather than copying its list syntax.
func coverageMentions(rows []CoverageRow, item string) bool {
	for _, row := range rows {
		if equalItem(row.Item, item) {
			return true
		}
	}
	return false
}

func equalItem(a, b string) bool {
	return normalize(a) == normalize(b)
}

func normalize(value string) string {
	lower := strings.ToLower(value)
	lower = strings.TrimRight(lower, ".;: ")
	return strings.Join(strings.Fields(lower), " ")
}

// absence reports whether a cell declares that the thing does not exist, and
// returns the reason it gives.
func absence(value string) (string, bool) {
	trimmed := strings.Trim(value, "* ")
	if !strings.HasPrefix(trimmed, "—") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "—"))
	if !strings.HasPrefix(rest, "(") || !strings.HasSuffix(rest, ")") {
		return "", true
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rest, "("), ")")), true
}

func methodList(methods map[string]bool) string {
	names := make([]string, 0, len(methods))
	for method := range methods {
		names = append(names, strings.ToUpper(method))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
