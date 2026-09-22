// Rules of the launch-governance register (P20-T01).
//
// The phase asks that every blocking human decision of a launch be either
// *decided in a versioned document* or *blocked in a way that prevents the
// release*. A register nobody reads is neither, so the register lives in one
// versioned document — docs/GOVERNANCE.md, prose for the reader and a fenced
// JSON block for this tool — and this tool is what turns "blocked" into a
// mechanical answer: while any item is blocked, the release gate fails naming
// it, its owner and what it blocks.
//
// It reads and judges; it never writes. The fix belongs to the owner of the
// decision, in a commit that changes the document — and the prose and the block
// are checked against each other, so a decision cannot exist in one half only.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// The statuses an item may hold.
const (
	// statusDecided means the owner stated the decision, with a date and
	// the place where it is enforced.
	statusDecided = "decided"
	// statusBlocked means the owner has not decided yet, and the item names
	// what is needed, who owes it and what it prevents.
	statusBlocked = "blocked"
)

// requiredItems is the set of decisions the phase demands a verdict on
// (.local/phases/20-release-readiness.md, P20-T01): minimum age, repository
// license, legal/privacy contact, retention policy, markets, terms of use and
// security channel. It is hardcoded on purpose — adding a decision to the
// register, or dropping one, has to be a deliberate change here, not something
// that slides in with a document edit.
var requiredItems = []string{
	"age-minimum",
	"repository-license",
	"data-subject-channel",
	"retention-policy",
	"launch-markets",
	"terms-of-use",
	"security-channel",
}

// proseTerminalState is the sentence the prose section of each item carries, so
// the reader's half and the machine's half cannot disagree.
const proseTerminalState = "**Estado:** "

// registerFence opens the machine-readable block of the document.
const registerFence = "```json"

// Register is the machine-readable half of docs/GOVERNANCE.md.
type Register struct {
	// DecidedBy names who owns the decisions in this register.
	DecidedBy string `json:"decided_by"`
	// RecordedAt is the date the register was written, YYYY-MM-DD.
	RecordedAt string `json:"recorded_at"`
	// Items are the decisions, one per id of requiredItems.
	Items []Item `json:"items"`
}

// Item is one blocking decision of a launch.
type Item struct {
	// ID is the stable identifier the tool matches against requiredItems and
	// against the prose section that documents it.
	ID string `json:"id"`
	// Status is statusDecided or statusBlocked.
	Status string `json:"status"`
	// Decision states what was decided. Required when decided.
	Decision string `json:"decision"`
	// DecidedAt is the date of the decision, YYYY-MM-DD. Required when
	// decided.
	DecidedAt string `json:"decided_at"`
	// EnforcedBy names where the decision is applied: the document that
	// states it, the rule that reads it or the code that carries it.
	// Required when decided.
	EnforcedBy string `json:"enforced_by"`
	// Pending lists the steps that remain before the release even though the
	// decision is made — committing a LICENSE file, creating an address,
	// completing a legal review. A pending step never stands in for a
	// decision: it is work, not an open question, and the gate prints it
	// without treating it as one.
	Pending []string `json:"pending"`
	// Blocker states the open decision. Required when blocked, forbidden
	// otherwise.
	Blocker *Blocker `json:"blocker"`
}

// Blocker is an open decision: what is needed, who owes it and what it keeps
// from happening.
type Blocker struct {
	// Needs states the decision itself, in one sentence.
	Needs string `json:"needs"`
	// Owner names who must decide.
	Owner string `json:"owner"`
	// Blocks names what the item prevents until decided.
	Blocks string `json:"blocks"`
}

// Finding is one rule violation, addressed to the item that caused it.
type Finding struct {
	Item   string
	Rule   string
	Detail string
}

// String renders a finding the way the other auditors do, so a reader can find
// the item and act: `docs/GOVERNANCE.md: item <id>: <rule>: <detail>`.
func (f Finding) String() string {
	return fmt.Sprintf("docs/GOVERNANCE.md: item %s: %s: %s", f.Item, f.Rule, f.Detail)
}

// Report is what the audit read and what it concluded.
type Report struct {
	// Path is the document that was audited.
	Path string
	// Items are the decisions the document states, in document order.
	Items []Item
	// Findings are the rule violations. Any finding means the register is not
	// a register.
	Findings []Finding
	// Blocked are the items whose status is blocked, in document order.
	Blocked []Item
}

// The rules this audit enforces. Each one exists because its absence would let
// a missing decision pass as a decision.
const (
	// ruleFenceMissing: the document has no machine-readable block, so
	// nothing about it can be verified.
	ruleFenceMissing = "register_missing"
	// ruleFenceUnparsable: the block is not valid JSON, or carries a field
	// this schema does not know. Unknown fields are refused so a typo cannot
	// silently drop a requirement.
	ruleFenceUnparsable = "register_unparsable"
	// ruleRegisterIncomplete: the register names who owns its decisions and
	// when it was written. A register signed by nobody is a draft, and the
	// owner is the first thing an open decision needs.
	ruleRegisterIncomplete = "register_incomplete"
	// ruleRequiredItem: the seven decisions of the phase must be present
	// exactly once. A missing one is an omission, a duplicate is an
	// ambiguity, an unknown one is a decision nobody asked for.
	ruleRequiredItem = "required_item"
	// ruleStatusUnknown: only decided and blocked exist. A third word would
	// be a decision the gate cannot judge.
	ruleStatusUnknown = "status_unknown"
	// ruleDecidedIncomplete: a decided item states what was decided, when,
	// and where it is enforced. Without those three the "decision" is a note.
	ruleDecidedIncomplete = "decided_incomplete"
	// ruleBlockerIncomplete: a blocked item states what is needed, who owes
	// it and what it blocks, so the block is actionable and its consequence
	// is explicit.
	ruleBlockerIncomplete = "blocker_incomplete"
	// rulePendingEmpty: a pending step states work. An empty string is a step
	// that will never be done and never be noticed, which is worse than a
	// missing one.
	rulePendingEmpty = "pending_empty"
	// ruleStateInconsistent: a decided item carries no blocker and a blocked
	// item carries no decision; a register that holds both for the same item
	// is answering "yes and no".
	ruleStateInconsistent = "state_inconsistent"
	// ruleDateMalformed: the dates are YYYY-MM-DD, so a reader parses them
	// the same way the tool does.
	ruleDateMalformed = "date_malformed"
	// ruleProseMissing: every item has a prose section, because the register
	// is read by people too and a decision nobody can read is not recorded.
	ruleProseMissing = "prose_missing"
	// ruleProseStateMismatch: the prose section states the same terminal
	// state as the block, so the two halves of the document cannot diverge.
	ruleProseStateMismatch = "prose_state_mismatch"
)

// Audit reads the register and applies every rule.
func Audit(path string) (Report, error) {
	report := Report{Path: path}

	document, err := os.ReadFile(path)
	if err != nil {
		return report, fmt.Errorf("read %s: %w", path, err)
	}
	source := string(document)

	block, prose, ok := splitRegister(source)
	if !ok {
		report.Findings = append(report.Findings, Finding{
			Item:   "-",
			Rule:   ruleFenceMissing,
			Detail: "the document carries no " + registerFence + " block; a register without one cannot be checked and is therefore not a register",
		})
		return report, nil
	}

	register, err := decodeRegister(block)
	if err != nil {
		report.Findings = append(report.Findings, Finding{
			Item:   "-",
			Rule:   ruleFenceUnparsable,
			Detail: err.Error(),
		})
		return report, nil
	}

	report.Items = register.Items
	report.Findings = append(report.Findings, auditRegister(register, prose)...)

	for _, item := range register.Items {
		if item.Status == statusBlocked {
			report.Blocked = append(report.Blocked, item)
		}
	}
	return report, nil
}

// decodeRegister parses the block strictly: a field the schema does not know is
// a requirement nobody reads, and it is refused instead of ignored.
func decodeRegister(block string) (Register, error) {
	var register Register
	decoder := json.NewDecoder(strings.NewReader(block))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&register); err != nil {
		return Register{}, fmt.Errorf("the register is not valid JSON for this schema: %w", err)
	}
	if decoder.More() {
		return Register{}, fmt.Errorf("the register carries more than one JSON value")
	}
	return register, nil
}

// splitRegister separates the machine-readable block from the prose around it.
// An unterminated fence is a missing block: half a block is not readable.
func splitRegister(source string) (block string, prose string, ok bool) {
	start := strings.Index(source, registerFence)
	if start < 0 {
		return "", source, false
	}
	rest := source[start+len(registerFence):]
	end := strings.Index(rest, "```")
	if end < 0 {
		return "", source, false
	}
	return rest[:end], source[:start], true
}

// auditRegister applies the rules that need the whole register or the prose
// beside it.
func auditRegister(register Register, prose string) []Finding {
	findings := make([]Finding, 0)

	if strings.TrimSpace(register.DecidedBy) == "" {
		findings = append(findings, Finding{Item: "-", Rule: ruleRegisterIncomplete, Detail: "the register names nobody as the owner of its decisions"})
	}
	if !isDate(register.RecordedAt) {
		findings = append(findings, Finding{Item: "-", Rule: ruleDateMalformed, Detail: fmt.Sprintf("recorded_at %q is not a YYYY-MM-DD date", register.RecordedAt)})
	}

	seen := map[string]int{}
	order := make([]string, 0, len(register.Items))
	for _, item := range register.Items {
		seen[item.ID]++
		order = append(order, item.ID)
	}

	for _, id := range requiredItems {
		switch seen[id] {
		case 0:
			findings = append(findings, Finding{
				Item:   id,
				Rule:   ruleRequiredItem,
				Detail: "the phase requires a verdict on this decision and the register does not carry one",
			})
		case 1:
		default:
			findings = append(findings, Finding{
				Item:   id,
				Rule:   ruleRequiredItem,
				Detail: fmt.Sprintf("the register states this decision %d times; one decision, one entry", seen[id]),
			})
		}
	}
	for _, id := range order {
		if !containsString(requiredItems, id) {
			findings = append(findings, Finding{
				Item:   id,
				Rule:   ruleRequiredItem,
				Detail: "the register carries an item the phase does not ask for; a decision nobody asked for is not part of the register",
			})
		}
	}

	for _, item := range register.Items {
		findings = append(findings, auditItem(item, prose)...)
	}
	return findings
}

// auditItem applies the rules of one item.
func auditItem(item Item, prose string) []Finding {
	findings := make([]Finding, 0)

	switch item.Status {
	case statusDecided:
		if strings.TrimSpace(item.Decision) == "" {
			findings = append(findings, Finding{Item: item.ID, Rule: ruleDecidedIncomplete, Detail: "decided without stating what was decided"})
		}
		if !isDate(item.DecidedAt) {
			findings = append(findings, Finding{Item: item.ID, Rule: ruleDateMalformed, Detail: fmt.Sprintf("decided_at %q is not a YYYY-MM-DD date", item.DecidedAt)})
		}
		if strings.TrimSpace(item.EnforcedBy) == "" {
			findings = append(findings, Finding{Item: item.ID, Rule: ruleDecidedIncomplete, Detail: "decided without naming where the decision is enforced"})
		}
		if item.Blocker != nil {
			findings = append(findings, Finding{Item: item.ID, Rule: ruleStateInconsistent, Detail: "decided and blocked at once; the register has to answer one thing"})
		}
	case statusBlocked:
		switch {
		case item.Blocker == nil:
			findings = append(findings, Finding{Item: item.ID, Rule: ruleBlockerIncomplete, Detail: "blocked without stating what is needed, who owes it and what it prevents"})
		default:
			if strings.TrimSpace(item.Blocker.Needs) == "" {
				findings = append(findings, Finding{Item: item.ID, Rule: ruleBlockerIncomplete, Detail: "blocked without stating the decision that is needed"})
			}
			if strings.TrimSpace(item.Blocker.Owner) == "" {
				findings = append(findings, Finding{Item: item.ID, Rule: ruleBlockerIncomplete, Detail: "blocked without naming who must decide; a block nobody owes is not a block"})
			}
			if strings.TrimSpace(item.Blocker.Blocks) == "" {
				findings = append(findings, Finding{Item: item.ID, Rule: ruleBlockerIncomplete, Detail: "blocked without stating what it prevents"})
			}
		}
		if strings.TrimSpace(item.Decision) != "" {
			findings = append(findings, Finding{Item: item.ID, Rule: ruleStateInconsistent, Detail: "blocked and decided at once; the register has to answer one thing"})
		}
	default:
		findings = append(findings, Finding{
			Item:   item.ID,
			Rule:   ruleStatusUnknown,
			Detail: fmt.Sprintf("status %q is neither %s nor %s", item.Status, statusDecided, statusBlocked),
		})
	}

	for _, step := range item.Pending {
		if strings.TrimSpace(step) == "" {
			findings = append(findings, Finding{Item: item.ID, Rule: rulePendingEmpty, Detail: "a pending step is empty; an empty step is not work"})
		}
	}

	findings = append(findings, auditProse(item, prose)...)
	return findings
}

// auditProse checks that the reader's half of the document says what the
// machine's half says. The section of an item runs from its heading to the next
// heading or the end of the prose.
func auditProse(item Item, prose string) []Finding {
	heading := "### " + item.ID
	start := strings.Index(prose, heading)
	if start < 0 {
		return []Finding{{
			Item:   item.ID,
			Rule:   ruleProseMissing,
			Detail: "the register has no prose section for this decision; a register only a machine can read is not a record",
		}}
	}

	rest := prose[start+len(heading):]
	if next := strings.Index(rest, "\n### "); next >= 0 {
		rest = rest[:next]
	}

	if !knownStatus(item.Status) {
		// The item is already refused for carrying a status no rule can
		// judge; comparing its prose to a word that does not exist would
		// report one defect twice.
		return nil
	}

	want := proseTerminalState + proseWord(item.Status)
	if !strings.Contains(rest, want) {
		return []Finding{{
			Item:   item.ID,
			Rule:   ruleProseStateMismatch,
			Detail: fmt.Sprintf("the prose section does not state %q, so the two halves of the document disagree", want),
		}}
	}
	return nil
}

// knownStatus reports whether a status is one the register understands.
func knownStatus(status string) bool {
	return status == statusDecided || status == statusBlocked
}

// proseWord is the Portuguese word each status carries in the prose, because
// the document is read by people and the working language of the plan is
// Portuguese.
func proseWord(status string) string {
	if status == statusDecided {
		return "decidida"
	}
	return "bloqueio"
}

func isDate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
