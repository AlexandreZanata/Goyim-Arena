// The vocabulary and the machine-readable half of the privacy and moderation
// audit (P20-T06).
//
// The phase asks for a review of the data lifecycle — export, delete,
// retention, analytics payload, logs, low counts, moderation appeals and
// reversals — exercised with two synthetic accounts looking for data mixing,
// and it asks for three things to be true when the review ends: the suite
// passes, the report carries no real PII, and the public exports are validated
// against an allowlist.
//
// A review nobody can re-run is an opinion, so the review lives in one
// versioned document (docs/PRIVACY_AUDIT.md: prose for the reader, a fenced
// JSON block for this tool) and this file is the shape of that block. The
// rules in audit.go are what turn each claim into an answer.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// documentPath is the register this gate judges, relative to the root.
const documentPath = "docs/PRIVACY_AUDIT.md"

// registerFence opens the machine-readable block of the document.
const registerFence = "```json"

// dateFormat is the only date shape the register may use.
const dateFormat = "2006-01-02"

// The surfaces an allowlist may describe. A public export may not carry a
// single field about a person; the subject's own export may carry the subject's
// data and may not carry anybody else's, nor a provider, modality, device or
// secret identifier. The two are different contracts, so they are different
// words here.
const (
	// surfacePublic is what anyone may read.
	surfacePublic = "public"
	// surfaceSubject is what only the authenticated owner may read.
	surfaceSubject = "subject"
)

// surfaces is the complete vocabulary.
var surfaces = []string{surfacePublic, surfaceSubject}

// The severities the audit uses, most severe first.
const (
	severityCritical = "Crítica"
	severityHigh     = "Alta"
	severityMedium   = "Média"
	severityLow      = "Baixa"
)

// severities is the complete vocabulary. A severity outside it is a typo that
// would quietly skip every rule keyed on severity.
var severities = []string{severityCritical, severityHigh, severityMedium, severityLow}

// The verdicts an area may carry.
const (
	// verdictMitigated: the control exists and an execution covers it.
	verdictMitigated = "mitigated"
	// verdictMonitoring: the control is procedure or observation, not
	// mechanism.
	verdictMonitoring = "monitoring"
	// verdictGap: the control declared for the area is absent or weaker, and
	// the residual is a named finding.
	verdictGap = "gap"
)

// verdicts is the complete vocabulary.
var verdicts = []string{verdictMitigated, verdictMonitoring, verdictGap}

// The statuses a finding may hold.
const (
	// statusAccepted: the residual is known, owned and dated.
	statusAccepted = "accepted"
	// statusOpen: nothing was decided about the residual.
	statusOpen = "open"
	// statusFixed: the defect was corrected in this phase.
	statusFixed = "fixed"
)

// statuses is the complete vocabulary.
var statuses = []string{statusAccepted, statusOpen, statusFixed}

// requiredAreas are the seven areas the phase names, in the order the document
// presents them. It is hardcoded on purpose: dropping an area, or adding one,
// has to be a deliberate change here and not something that slides in with a
// document edit.
var requiredAreas = []string{
	"public-export",
	"personal-export",
	"deletion",
	"retention",
	"analytics-payload",
	"logs",
	"moderation",
}

// Register is the machine-readable half of docs/PRIVACY_AUDIT.md.
type Register struct {
	// Version is the schema version of the block.
	Version int `json:"version"`
	// AuditedOn is the date of the review, YYYY-MM-DD.
	AuditedOn string `json:"audited_on"`
	// Accounts are the synthetic subjects the review was run with.
	Accounts []Account `json:"accounts"`
	// Mixing names the execution that looks for data mixing between them.
	Mixing Mixing `json:"mixing_evidence"`
	// Allowlists are the closed key sets of the exports.
	Allowlists []Allowlist `json:"allowlists"`
	// Retention is the retention table the policy declares.
	Retention []Retention `json:"retention"`
	// LowCount is the reidentification threshold of the aggregates.
	LowCount LowCount `json:"low_count"`
	// Analytics is the event vocabulary the product may send.
	Analytics []Analytics `json:"analytics"`
	// Areas is one entry per area the phase names.
	Areas []Area `json:"areas"`
	// Findings are the defects the audit found, each with its residual.
	Findings []Finding `json:"findings"`
}

// Account is one synthetic subject of the review. The email is what the
// executions use, and the rule refuses anything that is not in a domain
// reserved for exactly this purpose: a report that carries a real address is a
// report that leaked one.
type Account struct {
	// ID is the role the account plays in the review.
	ID string `json:"id"`
	// Email is the synthetic address, in a reserved domain.
	Email string `json:"email"`
	// Note states what the account is used for.
	Note string `json:"note"`
}

// Mixing names the execution that hunts for data of one account inside the
// other's surfaces. It is a path, not a sentence: the rule opens the file and
// requires it to name both accounts, because "two accounts were used" is a
// claim and a test that compares them is evidence.
type Mixing struct {
	// Path is the file that compares the two accounts.
	Path string `json:"path"`
	// Note states what the comparison asserts.
	Note string `json:"note"`
}

// Allowlist is the closed key set of one document the code can emit.
type Allowlist struct {
	// ID names the surface, for the reader of a failure.
	ID string `json:"id"`
	// Surface is public or subject, which decides what may never appear.
	Surface string `json:"surface"`
	// Path is the Go file whose JSON keys are the surface.
	Path string `json:"path"`
	// Keys is the complete set of JSON keys the file may emit, sorted.
	Keys []string `json:"keys"`
	// Note states why the surface has exactly these keys.
	Note string `json:"note"`
}

// Retention is one row of the executable retention policy as the document
// publishes it. The rule compares the table with the policy the job enforces.
type Retention struct {
	// Class is the governed data class.
	Class string `json:"class"`
	// Action is purge, anonymize or retain.
	Action string `json:"action"`
	// WindowHours is the period after a record became terminal, in hours.
	// Zero with Indefinite for a class kept under an obligation.
	WindowHours int `json:"window_hours"`
	// Indefinite marks a class with no purge horizon.
	Indefinite bool `json:"indefinite"`
	// ReasonCode is the stable justification published with the policy.
	ReasonCode string `json:"reason_code"`
}

// LowCount is the reidentification threshold the aggregates apply.
type LowCount struct {
	// Threshold is the smallest population an aggregate may publish.
	Threshold int `json:"threshold"`
	// Note states where it is enforced.
	Note string `json:"note"`
}

// Analytics is one event of the analytics allowlist and the properties it
// admits.
type Analytics struct {
	// Event is the event name as the code spells it.
	Event string `json:"event"`
	// Properties is the complete set of admitted property names, sorted. An
	// event with no property is declared with an empty list, which is a
	// decision and not an omission.
	Properties []string `json:"properties"`
}

// Area is one of the seven areas, with how it is judged and how it is
// executed.
type Area struct {
	// Key is the area's identifier, from requiredAreas.
	Key string `json:"key"`
	// Verdict is one of the verdict vocabulary.
	Verdict string `json:"verdict"`
	// Finding names the finding that carries the residual. Required when the
	// verdict is gap, forbidden otherwise.
	Finding string `json:"finding"`
	// Evidence are the paths, relative to the repository root, that show the
	// control. Every one of them has to exist.
	Evidence []string `json:"evidence"`
	// Execution is the command the gate runs for this area.
	Execution string `json:"execution"`
	// Note states what was confirmed, and for a gap what is missing.
	Note string `json:"note"`
}

// Finding is a defect the audit found, with its residual.
type Finding struct {
	// ID is the stable identifier, PRV-<NN>.
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
	// Area is the area the finding belongs to.
	Area string `json:"area"`
	// Evidence are the paths that show the defect.
	Evidence []string `json:"evidence"`
	// Plan is the work that closes the finding.
	Plan string `json:"plan"`
}

// Violation is one rule violation, addressed to what caused it.
type Violation struct {
	// Path is the file the violation is about. Empty means the register this
	// gate judges.
	Path string
	// Rule is the name of the rule that refused, so a test can assert which
	// one fired instead of asserting that something did.
	Rule string
	// Subject is the area, allowlist or finding the violation is about.
	// Empty for a violation about the document as a whole.
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
	// Text is the whole document, which is what the PII scan reads.
	Text string
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

	var register Register
	decoder := json.NewDecoder(bytes.NewReader([]byte(body[:end])))
	// A field the tool does not know is a claim the tool cannot judge, and an
	// unjudged claim in an audit register is exactly what this gate refuses.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&register); err != nil {
		return Document{}, fmt.Errorf("%s: the register block is not readable: %w", path, err)
	}
	return Document{Path: path, Prose: text[:fence], Text: text, Register: register}, nil
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

// sorted reports whether values is in ascending order and holds no duplicate.
// The register is compared by a reader as much as by this tool, so a list whose
// order changes between two runs of an unrelated edit is a diff nobody can
// review.
func sorted(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] >= values[i] {
			return false
		}
	}
	return true
}

// sameSet reports whether two string lists hold the same values.
func sameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, value := range left {
		if !contains(right, value) {
			return false
		}
	}
	return true
}
