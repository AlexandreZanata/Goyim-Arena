// The rule catalogue of the security audit (P20-T04).
//
// One function per rule, each returning the violations it found instead of
// stopping at the first: an audit that reports one problem per run makes the
// next problem as expensive as the first. Every rule is named, and the tests
// hold one mutation for each name — a rule without a mutation is a rule that
// might never fire.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The paths the audit reads, relative to the repository root.
const (
	// documentPath is the register this gate judges.
	documentPath = "docs/SECURITY_AUDIT.md"
	// threatModelPath is the model the register must agree with.
	threatModelPath = "docs/THREAT_MODEL.md"
	// matrixPath is the test matrix, which has to name the same threats.
	matrixPath = "docs/THREAT_MODEL_TEST_MATRIX.md"
	// workflowDir holds the workflows whose jobs the register may defer to.
	workflowDir = ".github/workflows"
	// gitignorePath is the file the environment scan reads.
	gitignorePath = ".gitignore"
)

// envFilesAllowed are the example files a repository is expected to track: a
// template carries names, never values.
var envFilesAllowed = []string{".env.example", ".env.sample", ".env.template"}

// jobPattern is the job name of a workflow: two spaces of indentation under
// `jobs:`. A deeper line is a step, and a shallower one is the end of the
// block.
var jobPattern = regexp.MustCompile(`^  ([A-Za-z0-9_-]+):`)

// Options is what the audit is asked to do.
type Options struct {
	// Root is the repository root the paths are resolved against.
	Root string
	// Run tells the audit to execute each area's command. Without it the
	// register is judged but nothing runs: that is what the tests use, and
	// what `-check` exposes.
	Run bool
	// Timeout bounds one area's execution.
	Timeout time.Duration
	// Tracker lists the files git tracks. It exists so the environment scan
	// can be tested without a repository, and so the scan judges what is
	// committed rather than what happens to be on disk.
	Tracker func(root string) ([]string, error)
}

// Report is what the audit read, what it ran and what it refused.
type Report struct {
	// Document is the register that was audited.
	Document Document
	// Threats is the number of threats the model declares.
	Threats int
	// Matrix is the number of threats the test matrix declares.
	Matrix int
	// Executions is one entry per area that was run, in register order.
	Executions []Execution
	// Tracked is the number of files the environment scan looked at.
	Tracked int
	// Violations is every rule violation, in the order the rules ran.
	Violations []Violation
}

// Counts returns how many threats carry each verdict, which is what the reader
// of the gate output wants before the details.
func (r Report) Counts() map[string]int {
	counts := map[string]int{}
	for _, threat := range r.Document.Register.Threats {
		counts[threat.Verdict]++
	}
	return counts
}

// Audit reads the register, applies every rule and — when asked — runs the
// twelve executions. It never writes and never fixes.
func Audit(options Options) (Report, error) {
	document, err := readDocument(filepath.Join(options.Root, documentPath))
	if err != nil {
		return Report{}, err
	}
	report := Report{Document: document}

	report.Violations = append(report.Violations, ruleTopLevel(document)...)

	modelSeverities, modelOrder, modelViolations := modelThreats(options.Root)
	report.Violations = append(report.Violations, modelViolations...)
	report.Threats = len(modelOrder)
	report.Violations = append(report.Violations, ruleMatrix(options.Root, modelOrder)...)

	report.Violations = append(report.Violations,
		ruleThreatSet(document, modelOrder)...)
	report.Violations = append(report.Violations,
		ruleThreatSeverity(document, modelSeverities)...)
	report.Violations = append(report.Violations,
		ruleThreatVerdict(document, modelSeverities)...)
	report.Violations = append(report.Violations,
		ruleThreatEvidence(options.Root, document)...)

	report.Violations = append(report.Violations, ruleAreas(options.Root, document)...)

	jobs, err := workflowJobs(filepath.Join(options.Root, workflowDir))
	if err != nil {
		return Report{}, err
	}
	report.Violations = append(report.Violations, ruleAreaJobs(document, jobs)...)

	report.Violations = append(report.Violations,
		ruleFindingEvidence(options.Root, document)...)
	report.Violations = append(report.Violations, ruleFindingFloor(document)...)
	report.Violations = append(report.Violations,
		ruleFindingAcceptance(document)...)
	report.Violations = append(report.Violations,
		ruleFindingThreat(document, modelSeverities)...)
	report.Violations = append(report.Violations, ruleProse(document)...)

	tracker := options.Tracker
	if tracker == nil {
		tracker = trackedFiles
	}
	tracked, err := tracker(options.Root)
	if err != nil {
		return Report{}, err
	}
	report.Tracked = len(tracked)
	scan, err := scanSecrets(options.Root, tracked)
	if err != nil {
		return Report{}, err
	}
	report.Violations = append(report.Violations, scan...)

	// Running commands on a register that is already refused would spend
	// minutes to reach a conclusion the structure already gave: the register
	// is not a register until it is well formed.
	if options.Run && len(report.Violations) == 0 {
		executions, runViolations := runAreas(options.Root, document.Register.Areas, options.Timeout)
		report.Executions = executions
		report.Violations = append(report.Violations, runViolations...)
	}
	return report, nil
}

// ruleTopLevel refuses a register whose shape cannot be judged: a version or a
// date that is not what it says, an identifier that does not follow the
// convention, a duplicate, a missing sentence.
func ruleTopLevel(document Document) []Violation {
	register := document.Register
	var violations []Violation
	refuse := func(rule, subject, detail string) {
		violations = append(violations, Violation{Rule: rule, Subject: subject, Detail: detail})
	}

	if register.Version < 1 {
		refuse("register-version", "", fmt.Sprintf("version %d is not a schema version", register.Version))
	}
	if !isDate(register.AuditedOn) {
		refuse("register-date", "", fmt.Sprintf("audited_on %q is not a %s date", register.AuditedOn, dateFormat))
	}
	if len(register.Threats) == 0 {
		refuse("register-threats", "", "the register carries no threat")
	}
	if len(register.Areas) == 0 {
		refuse("register-areas", "", "the register carries no area")
	}

	seenThreat := map[string]bool{}
	for _, threat := range register.Threats {
		if !threatIDPattern.MatchString(threat.ID) {
			refuse("threat-id", threat.ID, "not a THR-<AREA>-<NN> identifier")
			continue
		}
		if seenThreat[threat.ID] {
			refuse("threat-duplicate", threat.ID, "the threat appears twice")
		}
		seenThreat[threat.ID] = true
		if strings.TrimSpace(threat.Note) == "" {
			refuse("threat-note", threat.ID, "no note: an entry nobody explained is not an execution")
		}
	}

	for _, finding := range register.Findings {
		if !findingIDPattern.MatchString(finding.ID) {
			refuse("finding-id", finding.ID, "not a SEC-<NN> identifier")
		}
		if strings.TrimSpace(finding.Title) == "" {
			refuse("finding-title", finding.ID, "the finding has no title")
		}
		if !contains(severities, finding.Severity) {
			refuse("finding-severity", finding.ID, fmt.Sprintf("severity %q is outside the vocabulary", finding.Severity))
		}
		if !contains(statuses, finding.Status) {
			refuse("finding-status", finding.ID, fmt.Sprintf("status %q is outside the vocabulary", finding.Status))
		}
		if strings.TrimSpace(finding.Plan) == "" {
			refuse("finding-plan", finding.ID, "the finding names no work that closes it")
		}
	}
	return violations
}

// threatIDPattern is the identifier convention of the model.
var threatIDPattern = regexp.MustCompile(`^THR-[A-Z]+-[0-9]{2}$`)

// findingIDPattern is the identifier convention of this register.
var findingIDPattern = regexp.MustCompile(`^SEC-[0-9]{2}$`)

// isDate reports whether value is a date in the only shape the register uses.
func isDate(value string) bool {
	parsed, err := time.Parse(dateFormat, value)
	return err == nil && parsed.Format(dateFormat) == value
}

// modelThreats reads the severity the model assigns to each threat, in order.
// A row is recognised by its bold identifier cell, and the severity is the
// first bold severity word on that line: that is the Severidade column whether
// or not the row carries every column, and it is why the parse does not depend
// on column count.
func modelThreats(root string) (map[string]string, []string, []Violation) {
	path := filepath.Join(root, threatModelPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, []Violation{{
			Path: threatModelPath, Rule: "threat-model-unreadable",
			Detail: fmt.Sprintf("cannot read the model: %v", err),
		}}
	}

	severitiesByID := map[string]string{}
	var order []string
	var violations []Violation
	for _, line := range strings.Split(string(raw), "\n") {
		match := threatRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		id := match[1]
		severity := severityIn(line)
		if severity == "" {
			violations = append(violations, Violation{
				Path: threatModelPath, Subject: id, Rule: "threat-model-severity",
				Detail: "the row carries no severity from the vocabulary",
			})
			continue
		}
		if _, seen := severitiesByID[id]; seen {
			violations = append(violations, Violation{
				Path: threatModelPath, Subject: id, Rule: "threat-model-duplicate",
				Detail: "the model declares this threat twice",
			})
			continue
		}
		severitiesByID[id] = severity
		order = append(order, id)
	}
	if len(order) == 0 {
		violations = append(violations, Violation{
			Path: threatModelPath, Rule: "threat-model-empty",
			Detail: "no threat row was found in the model",
		})
	}
	return severitiesByID, order, violations
}

// severityIn returns the first severity of the vocabulary written in bold on
// the line, which is the Severidade column of a model row.
func severityIn(line string) string {
	for _, severity := range severities {
		if strings.Contains(line, "**"+severity+"**") {
			return severity
		}
	}
	return ""
}

// matrixRow matches the identifier cell of a matrix row. The matrix does not
// embolden its identifiers — it opens the row with one — so its rule reads the
// rows whose first cell is a threat, and a prose mention of a threat inside a
// cell is not a row.
var matrixRow = regexp.MustCompile(`(?m)^\|\s*(THR-[A-Z]+-[0-9]{2})\s*\|`)

// ruleMatrix refuses a matrix that does not name exactly the threats of the
// model: the matrix is the evidence that each threat has a test, and a threat
// missing from it is a threat nobody looked at.
func ruleMatrix(root string, modelOrder []string) []Violation {
	raw, err := os.ReadFile(filepath.Join(root, matrixPath))
	if err != nil {
		return []Violation{{
			Path: matrixPath, Rule: "threat-model-matrix",
			Detail: fmt.Sprintf("cannot read the matrix: %v", err),
		}}
	}
	inMatrix := map[string]bool{}
	for _, match := range matrixRow.FindAllStringSubmatch(string(raw), -1) {
		inMatrix[match[1]] = true
	}
	var violations []Violation
	for _, id := range modelOrder {
		if !inMatrix[id] {
			violations = append(violations, Violation{
				Path: matrixPath, Subject: id, Rule: "threat-model-matrix",
				Detail: "the model declares this threat and the matrix never names it",
			})
		}
	}
	return violations
}

// ruleThreatSet refuses a register that does not carry exactly the threats of
// the model: a threat left out is a threat this audit did not execute, and a
// threat invented here is a threat nobody declared.
func ruleThreatSet(document Document, modelOrder []string) []Violation {
	declared := map[string]bool{}
	for _, id := range modelOrder {
		declared[id] = true
	}
	audited := map[string]bool{}
	var violations []Violation
	for _, threat := range document.Register.Threats {
		audited[threat.ID] = true
		if !declared[threat.ID] {
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-set",
				Detail: "the register audits a threat the model does not declare",
			})
		}
	}
	for _, id := range modelOrder {
		if !audited[id] {
			violations = append(violations, Violation{
				Subject: id, Rule: "threat-set",
				Detail: "the model declares this threat and the register leaves it unaudited",
			})
		}
	}
	return violations
}

// ruleThreatSeverity refuses a register that re-grades a threat: the severity
// belongs to the model, and an audit that lowers it while calling itself an
// audit is the failure this rule exists for.
func ruleThreatSeverity(document Document, modelSeverities map[string]string) []Violation {
	var violations []Violation
	for _, threat := range document.Register.Threats {
		declared, known := modelSeverities[threat.ID]
		if !known {
			continue
		}
		if threat.Severity != declared {
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-severity",
				Detail: fmt.Sprintf("the register grades it %s and the model grades it %s", orEmpty(threat.Severity), declared),
			})
		}
	}
	return violations
}

// ruleThreatVerdict refuses a verdict outside the vocabulary, a verdict that
// names no finding where one is owed, a verdict that names one where none is,
// and the two floor rules of the phase: a Crítica threat may not be merely
// monitored (the model's §1), and no threat may be left open.
func ruleThreatVerdict(document Document, modelSeverities map[string]string) []Violation {
	findings := map[string]bool{}
	for _, finding := range document.Register.Findings {
		findings[finding.ID] = true
	}
	var violations []Violation
	for _, threat := range document.Register.Threats {
		if !contains(verdicts, threat.Verdict) {
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-verdict",
				Detail: fmt.Sprintf("verdict %q is outside the vocabulary", threat.Verdict),
			})
			continue
		}
		owesFinding := threat.Verdict == verdictAccepted || threat.Verdict == verdictOpen
		switch {
		case owesFinding && threat.Finding == "":
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-verdict",
				Detail: fmt.Sprintf("verdict %s owes a finding and names none", threat.Verdict),
			})
		case owesFinding && !findings[threat.Finding]:
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-verdict",
				Detail: fmt.Sprintf("verdict %s names the finding %s, which the register does not carry", threat.Verdict, threat.Finding),
			})
		case !owesFinding && threat.Finding != "":
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-verdict",
				Detail: fmt.Sprintf("verdict %s names the finding %s, which nothing owes", threat.Verdict, threat.Finding),
			})
		}

		declared := modelSeverities[threat.ID]
		if declared == severityCritical && threat.Verdict == verdictMonitored {
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-critical-monitored",
				Detail: "a Crítica threat may not be answered with monitoring: the model demands a preventive mitigation and a verifiable test",
			})
		}
		if threat.Verdict == verdictOpen && (declared == severityCritical || declared == severityHigh) {
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-open-floor",
				Detail: fmt.Sprintf("a %s threat is left open; the phase allows no open finding at Crítica or Alta", declared),
			})
		}
	}
	return violations
}

// ruleThreatEvidence refuses a threat that cites nothing and resolves every path
// a threat does cite. This is what makes the register a register: a threat that
// says "mitigated" without showing where the control lives is an assertion, and a
// reference to a file that does not exist is a claim about nothing — the cheapest
// kind of false statement to write.
func ruleThreatEvidence(root string, document Document) []Violation {
	var violations []Violation
	for _, threat := range document.Register.Threats {
		if len(threat.Evidence) == 0 {
			violations = append(violations, Violation{
				Subject: threat.ID, Rule: "threat-evidence",
				Detail: "the threat shows no evidence at all: its verdict is an assertion",
			})
			continue
		}
		for _, path := range threat.Evidence {
			if detail := resolveEvidence(root, path); detail != "" {
				violations = append(violations, Violation{
					Subject: threat.ID, Rule: "threat-evidence", Detail: detail,
				})
			}
		}
	}
	return violations
}

// resolveEvidence returns the reason a cited path is not evidence, or the empty
// string when it is. A cited path must be relative and must exist; a directory
// is accepted, because citing the surface that carries the rule is often more
// honest than naming one file of it.
func resolveEvidence(root, path string) string {
	if strings.TrimSpace(path) == "" {
		return "an empty path is not evidence"
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "..") {
		return fmt.Sprintf("%q leaves the repository", path)
	}
	if _, err := os.Stat(filepath.Join(root, path)); err != nil {
		return fmt.Sprintf("%q does not exist", path)
	}
	return ""
}

// ruleAreas refuses an area that is missing, unknown or duplicated, an area
// that neither runs nor defers to a job, and an area whose rules cite nothing
// that exists.
func ruleAreas(root string, document Document) []Violation {
	var violations []Violation
	seen := map[string]bool{}
	for _, area := range document.Register.Areas {
		if !contains(requiredAreas, area.Key) {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-set",
				Detail: "the register audits an area the phase does not name",
			})
			continue
		}
		if seen[area.Key] {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-set", Detail: "the area appears twice",
			})
			continue
		}
		seen[area.Key] = true

		if strings.TrimSpace(area.Execution) == "" && len(area.DeferredTo) == 0 {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-execution",
				Detail: "the area neither runs a command nor names the job that covers it",
			})
		}
		if strings.TrimSpace(area.Note) == "" {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-execution",
				Detail: "the area states no note: what it establishes is unknown",
			})
		}
		if len(area.Evidence) == 0 {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-evidence",
				Detail: "the area cites nothing",
			})
		}
		for _, path := range area.Evidence {
			if detail := resolveEvidence(root, path); detail != "" {
				violations = append(violations, Violation{
					Subject: area.Key, Rule: "area-evidence", Detail: detail,
				})
			}
		}
	}
	for _, key := range requiredAreas {
		if !seen[key] {
			violations = append(violations, Violation{
				Subject: key, Rule: "area-set",
				Detail: "the phase names this area and the register never audits it",
			})
		}
	}
	return violations
}

// ruleAreaJobs refuses a job the register defers to that no workflow defines:
// "the pipeline covers it" is a check, not a sentence.
func ruleAreaJobs(document Document, jobs map[string]string) []Violation {
	var violations []Violation
	for _, area := range document.Register.Areas {
		for _, job := range area.DeferredTo {
			if _, ok := jobs[job]; !ok {
				violations = append(violations, Violation{
					Subject: area.Key, Rule: "area-job",
					Detail: fmt.Sprintf("defers to the job %q, which no workflow defines", job),
				})
			}
		}
	}
	return violations
}

// ruleFindingEvidence refuses a finding that cites nothing and a finding that
// cites what does not exist. The second half matters as much as the first: the
// cheapest false statement in an audit is a reference to a file nobody opens,
// and a residual has to be readable where this register says it is.
func ruleFindingEvidence(root string, document Document) []Violation {
	var violations []Violation
	for _, finding := range document.Register.Findings {
		if len(finding.Evidence) == 0 {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "finding-evidence",
				Detail: "the finding cites nothing",
			})
			continue
		}
		for _, path := range finding.Evidence {
			if detail := resolveEvidence(root, path); detail != "" {
				violations = append(violations, Violation{
					Subject: finding.ID, Rule: "finding-evidence", Detail: detail,
				})
			}
		}
	}
	return violations
}

// ruleFindingFloor is the minimum validation of the phase: no finding of
// severity Crítica or Alta may be open. It is not a threshold to tune — it is
// the sentence the phase wrote.
func ruleFindingFloor(document Document) []Violation {
	var violations []Violation
	for _, finding := range document.Register.Findings {
		if finding.Status != statusOpen {
			continue
		}
		if finding.Severity == severityCritical || finding.Severity == severityHigh {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "finding-floor",
				Detail: fmt.Sprintf("a %s finding is open at the end of the phase", finding.Severity),
			})
		}
	}
	return violations
}

// ruleFindingAcceptance refuses an accepted finding of Média or above without
// an owner and a date: a residual nobody owns is an unrecorded decision, which
// is what the acceptance exists to prevent.
func ruleFindingAcceptance(document Document) []Violation {
	var violations []Violation
	for _, finding := range document.Register.Findings {
		if finding.Status != statusAccepted {
			continue
		}
		if finding.Severity != severityCritical && finding.Severity != severityHigh && finding.Severity != severityMedium {
			continue
		}
		if strings.TrimSpace(finding.Owner) == "" {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "finding-acceptance",
				Detail: "accepted without an owner",
			})
		}
		if !isDate(finding.AcceptedOn) {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "finding-acceptance",
				Detail: fmt.Sprintf("accepted_on %q is not a %s date", finding.AcceptedOn, dateFormat),
			})
		}
	}
	return violations
}

// ruleFindingThreat refuses a finding that points at a threat nobody declared,
// and a threat that points at a finding that does not point back: the two halves
// of the register answer to each other, or the residual has no home.
func ruleFindingThreat(document Document, modelSeverities map[string]string) []Violation {
	named := map[string]string{}
	for _, threat := range document.Register.Threats {
		if threat.Finding != "" {
			named[threat.Finding] = threat.ID
		}
	}
	var violations []Violation
	for _, finding := range document.Register.Findings {
		if finding.Threat == "" {
			continue
		}
		if _, known := modelSeverities[finding.Threat]; !known {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "finding-threat",
				Detail: fmt.Sprintf("names the threat %s, which the model does not declare", finding.Threat),
			})
			continue
		}
		if named[finding.ID] != finding.Threat {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "finding-threat",
				Detail: fmt.Sprintf("names the threat %s, which does not carry this finding's id", finding.Threat),
			})
		}
	}
	return violations
}

// ruleProse refuses prose that forgot what the block decided: every finding and
// every area has to be readable in the document's own words, and the prose has
// to state the date of the run. The two halves of one document that disagree
// are worse than either alone.
func ruleProse(document Document) []Violation {
	var violations []Violation
	for _, finding := range document.Register.Findings {
		if !strings.Contains(document.Prose, finding.ID) {
			violations = append(violations, Violation{
				Subject: finding.ID, Rule: "prose",
				Detail: "the finding is in the block and the prose never mentions it",
			})
		}
	}
	for _, area := range document.Register.Areas {
		if !strings.Contains(document.Prose, area.Key) {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "prose",
				Detail: "the area is in the block and the prose never mentions it",
			})
		}
	}
	if !strings.Contains(document.Prose, document.Register.AuditedOn) {
		violations = append(violations, Violation{
			Rule:   "prose",
			Detail: fmt.Sprintf("the prose does not state the date of the run (%s)", document.Register.AuditedOn),
		})
	}
	return violations
}

// scanSecrets is the structural half of the secrets area: an environment file
// that is tracked is a leaked environment file, an environment file that is not
// ignored is one commit away from it, and a private key written in a file type
// that never legitimately holds one is a key. The value scan of the pipeline
// (gitleaks) is what covers the rest, which is a limit this audit records
// instead of implying.
func scanSecrets(root string, tracked []string) ([]Violation, error) {
	var violations []Violation
	for _, path := range tracked {
		base := filepath.Base(path)
		if base != ".env" && !strings.HasPrefix(base, ".env.") {
			continue
		}
		if contains(envFilesAllowed, base) {
			continue
		}
		violations = append(violations, Violation{
			Subject: path, Rule: "secrets-env-tracked",
			Detail: "an environment file is tracked; it carries values by definition",
		})
	}

	ignoreRaw, err := os.ReadFile(filepath.Join(root, gitignorePath))
	if err != nil {
		return nil, err
	}
	ignored := false
	for _, line := range strings.Split(string(ignoreRaw), "\n") {
		if strings.TrimSpace(line) == ".env" {
			ignored = true
			break
		}
	}
	if !ignored {
		violations = append(violations, Violation{
			Path: gitignorePath, Rule: "secrets-env-ignored",
			Detail: "the environment file is not ignored: the next commit can carry it",
		})
	}

	for _, path := range tracked {
		if !contains(materialExtensions, strings.ToLower(filepath.Ext(path))) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}
		if privateKeyHeader.Match(raw) {
			violations = append(violations, Violation{
				Subject: path, Rule: "secrets-key-material",
				Detail: "a private key header is committed",
			})
		}
	}
	return violations, nil
}

// workflowJobs maps each job name to the workflow that declares it, reading the
// `jobs:` block at its own indentation.
func workflowJobs(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	jobs := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		inJobs := false
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) == "jobs:" {
				inJobs = true
				continue
			}
			if !inJobs {
				continue
			}
			if strings.TrimSpace(line) == "" {
				continue
			}
			if !strings.HasPrefix(line, " ") {
				break
			}
			if match := jobPattern.FindStringSubmatch(line); match != nil {
				jobs[match[1]] = entry.Name()
			}
		}
	}
	return jobs, nil
}

// orEmpty keeps a nil-looking value readable in a message.
func orEmpty(value string) string {
	if value == "" {
		return "(nothing)"
	}
	return value
}
