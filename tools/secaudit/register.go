// Rules of the pre-release security audit (P20-T04).
//
// The phase asks that the threat model be *exercised* — negative
// authorization, cache, CSRF, sessions, MFA, Stripe, double spend, IDOR,
// secrets, dependencies, container — and that the trust boundaries be reviewed
// by hand. A review nobody can re-run is an opinion, so the review lives in one
// versioned document (docs/SECURITY_AUDIT.md: prose for the reader, a fenced
// JSON block for this tool) and this tool is what turns the claims into
// answers:
//
//   - every evidence path cited is resolved against the working tree;
//   - every threat of the model is present here exactly once, with the severity
//     the model gives it (a downgrade the register makes for itself is
//     refused);
//   - no threat the model calls Crítica reads "monitored", which is the model's
//     own §1 gate;
//   - every one of the twelve areas of the phase either runs a command or names
//     the CI job that runs it — and the job is checked to exist in the
//     workflow, so "the pipeline covers it" is not a claim;
//   - no finding of severity Crítica or Alta is open (the minimum validation of
//     the phase), and every finding at Média or above carries an owner and a
//     date of acceptance.
//
// It reads and judges; it never writes. The fix belongs to the owner, in a
// commit that changes the document.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The severities the threat model uses, most severe first.
const (
	severityCritical = "Crítica"
	severityHigh     = "Alta"
	severityMedium   = "Média"
	severityLow      = "Baixa"
)

// severities is the complete vocabulary. A severity outside it is a typo that
// would quietly skip every rule keyed on severity.
var severities = []string{severityCritical, severityHigh, severityMedium, severityLow}

// The verdicts a threat may carry in the register.
const (
	// verdictMitigated: every control the model declares exists and is
	// covered by an execution.
	verdictMitigated = "mitigated"
	// verdictAccepted: a control the model declares is absent or weaker, and
	// the residual is accepted in a named finding.
	verdictAccepted = "accepted"
	// verdictMonitored: the control is procedure or observation, not
	// mechanism. Forbidden for a Crítica threat by the model's §1.
	verdictMonitored = "monitored"
	// verdictOpen: an open finding at the level of the threat.
	verdictOpen = "open"
)

// verdicts is the complete vocabulary of verdicts.
var verdicts = []string{verdictMitigated, verdictAccepted, verdictMonitored, verdictOpen}

// The statuses a finding may hold.
const (
	// statusAccepted: the residual is known, owned and dated.
	statusAccepted = "accepted"
	// statusOpen: nothing was decided about the residual. Refused for Crítica
	// and Alta by the minimum validation of the phase.
	statusOpen = "open"
	// statusFixed: the defect was corrected in this phase.
	statusFixed = "fixed"
)

// statuses is the complete vocabulary of finding statuses.
var statuses = []string{statusAccepted, statusOpen, statusFixed}

// requiredAreas is the twelve areas the phase names, in the order the document
// presents them. It is hardcoded on purpose: dropping an area, or adding one,
// has to be a deliberate change here and not something that slides in with a
// document edit.
var requiredAreas = []string{
	"threat-model",
	"negative-authorization",
	"cache",
	"csrf",
	"sessions",
	"mfa",
	"stripe",
	"double-spend",
	"idor",
	"secrets",
	"dependencies",
	"container",
}

// materialExtensions are the file types a private key never legitimately lives
// in as text. Go source, Markdown and shell legitimately *name* the header — a
// scanner pattern, a runbook, a key generator — so they are out of this scan by
// construction, and the value scan of the pipeline (gitleaks) is what covers
// them. Recorded as a limit of the audit.
var materialExtensions = []string{
	".pem", ".key", ".crt", ".cer", ".p12", ".pfx", ".pkcs12",
	".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".txt",
}

// privateKeyHeader matches the PEM header of a private key.
var privateKeyHeader = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)

// threatRow matches the identifier cell of a threat row in the model's tables.
var threatRow = regexp.MustCompile(`\*\*(THR-[A-Z]+-[0-9]{2})\*\*`)

// dateFormat is the only date shape the register may use.
const dateFormat = "2006-01-02"

// registerFence opens the machine-readable block of the document.
const registerFence = "```json"

// Register is the machine-readable half of docs/SECURITY_AUDIT.md.
type Register struct {
	// Version is the schema version of the block.
	Version int `json:"version"`
	// AuditedOn is the date of the run, YYYY-MM-DD.
	AuditedOn string `json:"audited_on"`
	// Threats is one entry per threat of the model.
	Threats []Threat `json:"threats"`
	// Areas is one entry per area the phase names.
	Areas []Area `json:"areas"`
	// Findings are the defects the audit found, each with its residual.
	Findings []Finding `json:"findings"`
}

// Threat is the verdict on one threat of the model.
type Threat struct {
	// ID is the model's identifier, THR-<AREA>-<NN>.
	ID string `json:"id"`
	// Severity is the severity the model assigns. A disagreement is refused.
	Severity string `json:"severity"`
	// Verdict is one of the verdict vocabulary.
	Verdict string `json:"verdict"`
	// Finding names the finding that carries the residual. Required when the
	// verdict is accepted or open, forbidden otherwise.
	Finding string `json:"finding"`
	// Evidence are the paths, relative to the repository root, that show the
	// control. Every one of them has to exist.
	Evidence []string `json:"evidence"`
	// Note states, for the execution, what was confirmed — and for an accepted
	// threat, what is missing.
	Note string `json:"note"`
}

// Area is one of the twelve areas, with how it is executed.
type Area struct {
	// Key is the area's identifier, from requiredAreas.
	Key string `json:"key"`
	// Execution is the command the gate runs for this area. Either this or
	// DeferredTo is present.
	Execution string `json:"execution"`
	// DeferredTo names the CI jobs that cover what cannot run locally. Each
	// name is checked against the workflow.
	DeferredTo []string `json:"deferred_to"`
	// Evidence are the paths that carry the area's rules.
	Evidence []string `json:"evidence"`
	// Note states what the execution actually establishes.
	Note string `json:"note"`
}

// Finding is a defect the audit found, with its residual.
type Finding struct {
	// ID is the stable identifier, SEC-<NN>.
	ID string `json:"id"`
	// Title is the defect in one sentence.
	Title string `json:"title"`
	// Severity of the defect.
	Severity string `json:"severity"`
	// Status is one of the status vocabulary.
	Status string `json:"status"`
	// Owner is who decides about the residual. Required when accepted.
	Owner string `json:"owner"`
	// AcceptedOn is the date of the acceptance, YYYY-MM-DD. Required when
	// accepted.
	AcceptedOn string `json:"accepted_on"`
	// Threat is the threat the finding qualifies, if any.
	Threat string `json:"threat"`
	// Evidence are the paths that show the defect.
	Evidence []string `json:"evidence"`
	// Plan is the work that closes the finding.
	Plan string `json:"plan"`
}

// Violation is one rule violation, addressed to what caused it.
type Violation struct {
	// Path is the file the violation is about. Empty means the register this
	// gate judges; the model, the matrix and .gitignore have their own rules
	// and name themselves.
	Path string
	// Rule is the name of the rule that refused, so a test can assert which
	// one fired instead of asserting that something did.
	Rule string
	// Subject is the threat, area or finding the violation is about. Empty
	// for a violation about a document as a whole.
	Subject string
	// Detail states what was found and what was expected.
	Detail string
}

// String renders a violation the way the other auditors do, so a reader can
// find the subject and act.
func (v Violation) String() string {
	path := v.Path
	if path == "" {
		path = documentPath
	}
	if v.Subject == "" {
		return fmt.Sprintf("%s: %s: %s", path, v.Rule, v.Detail)
	}
	return fmt.Sprintf("%s: %s: %s: %s", path, v.Subject, v.Rule, v.Detail)
}

// Document is the register split into its two halves, so the rules can check
// that they agree: the prose the reader gets and the block the tool reads.
type Document struct {
	// Path is where the document was read from.
	Path string
	// Prose is everything before the machine-readable block.
	Prose string
	// Register is the parsed block.
	Register Register
}

// readDocument reads the document and splits it into prose and block.
func readDocument(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := string(raw)
	fence := strings.Index(text, registerFence)
	if fence < 0 {
		return Document{}, fmt.Errorf("%s: no %s block", path, registerFence)
	}
	body := text[fence+len(registerFence):]
	end := strings.Index(body, "```")
	if end < 0 {
		return Document{}, fmt.Errorf("%s: the %s block is never closed", path, registerFence)
	}
	block := body[:end]

	var register Register
	decoder := json.NewDecoder(bytes.NewReader([]byte(block)))
	// A field the tool does not know is a claim the tool cannot judge, and an
	// unjudged claim in an audit register is exactly what this gate refuses.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&register); err != nil {
		return Document{}, fmt.Errorf("%s: the register block is not readable: %w", path, err)
	}
	return Document{Path: path, Prose: text[:fence], Register: register}, nil
}

// contains reports whether values holds want.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// indexOf returns the position of want in values, or -1.
func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}
