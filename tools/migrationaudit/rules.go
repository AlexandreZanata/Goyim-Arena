// The rules, stated over what the run measured (P20-T03).
//
// Every rule here answers a question the phase asks, and each one fails with the
// objects that broke it rather than with a count. The rules are read against the
// reference database (the one built from scratch at head) and against the
// measurements of the ladder, the snapshots and the failure.
//
// Two severities exist, and only two:
//
//   - **gate**: the audit refuses. A history that removes something nobody
//     recorded, a snapshot that does not land on the reference database, a
//     migration that leaves state behind or a grant that is wider than the
//     ledger says are all reasons to stop a promotion.
//   - **advisory**: the audit reports it and does not refuse. It exists for
//     measurements where the *schema* is defensible and the finding is a
//     follow-up rather than a stop — today exactly one rule, the coverage of
//     foreign keys by an index, which the report states precisely (covered,
//     partially covered, uncovered) so the follow-up can be sized. The set is
//     closed on purpose and the tests assert it is exactly this: adding a
//     second advisory is an edit to this file and to that test, in review,
//     never a quiet demotion of a gate.
package main

import (
	"fmt"
	"sort"
	"strings"
)

// Severities.
const (
	severityGate     = "gate"
	severityAdvisory = "advisory"
)

// advisoryRules is the closed set of rules that report without refusing, each
// with the work the finding is handed to. An advisory without a follow-up would
// be a finding nobody owns, which is how a report becomes decoration; the test
// asserts the two maps have exactly the same keys, so neither can grow alone.
var advisoryRules = map[string]string{
	"foreign-keys-are-indexed": "uma migration que adicione os índices de cobertura para as chaves que este run lista, revisada por si: um índice muda o caminho de escrita da tabela e a janela da promoção, e não é algo que uma auditoria inclua no próprio commit",
}

type ruleResult struct {
	Name     string
	Subject  string
	Severity string
	Passed   bool
	Detail   string
}

// evaluateRules returns the failures that refuse a promotion and the ones that
// are recorded instead.
func evaluateRules(data *reportData) (findings, advisories []string) {
	for _, rule := range data.Rules {
		if rule.Passed {
			continue
		}
		entry := fmt.Sprintf("%s: %s", rule.Subject, rule.Detail)
		if rule.Severity == severityAdvisory {
			advisories = append(advisories, fmt.Sprintf("%s: %s", rule.Name, entry))
			continue
		}
		findings = append(findings, entry)
	}
	return findings, advisories
}

// rule adds one result and keeps the report's order.
func (data *reportData) rule(name, subject string, passed bool, detail string) {
	severity := severityGate
	if _, advisory := advisoryRules[name]; advisory {
		severity = severityAdvisory
	}
	data.Rules = append(data.Rules, ruleResult{Name: name, Subject: subject, Severity: severity, Passed: passed, Detail: detail})
}

// applyRules states every rule over the measured facts. It is called once the
// exercise is over, because half of the rules are about the exercise itself.
func applyRules(data *reportData) {
	virgin := data.Virgin
	data.Ledger = summarizeLedger()

	applyHistoryRules(data)
	applyLifecycleRules(data)
	if virgin == nil {
		return
	}
	applyOwnershipRules(data, virgin)
	applyGrantRules(data, virgin)
	applySequenceRules(data, virgin)
	applyIndexRules(data, virgin)
	applyForeignKeyRules(data, virgin)
	applyStructureRules(data, virgin)
}

// applyHistoryRules holds the forward history to its own decisions: what it
// removes, what it waits for, and whether each of those was written down.
func applyHistoryRules(data *reportData) {
	undeclared := make([]string, 0)
	for _, statement := range data.Destructive {
		if _, declared := contractionLedger[statement.Key]; !declared {
			undeclared = append(undeclared, fmt.Sprintf("%d %s %s (%s)",
				statement.Version, statement.What, statement.Object, statement.Name))
		}
	}
	data.rule("destructive-declared", "statement", len(undeclared) == 0,
		"a migration that removes something is recorded in the ledger with its reason: "+strings.Join(undeclared, "; "))

	stale := make([]string, 0)
	real := map[string]bool{}
	for _, statement := range data.Destructive {
		real[statement.Key] = true
	}
	for key := range contractionLedger {
		if !real[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	data.rule("destructive-ledger-fresh", "ledger", len(stale) == 0,
		"an entry of the ledger that no migration states is a decision that outlived its code: "+strings.Join(stale, "; "))

	blocking := make([]int64, 0)
	blockingUndeclared := make([]string, 0)
	for _, step := range data.Ladder {
		if !step.Blocked {
			continue
		}
		blocking = append(blocking, step.Version)
		if _, declared := blockingLedger[step.Version]; !declared {
			blockingUndeclared = append(blockingUndeclared,
				fmt.Sprintf("%d (%s) waits on %s", step.Version, step.Name, waitingList(step.Waiting)))
		}
	}
	data.rule("blocking-declared", "migration", len(blockingUndeclared) == 0,
		"a migration that waits for an active reader needs a window and a line in the ledger: "+strings.Join(blockingUndeclared, "; "))

	staleBlocking := make([]string, 0)
	seen := map[int64]bool{}
	for _, version := range blocking {
		seen[version] = true
	}
	for version := range blockingLedger {
		if !seen[version] {
			staleBlocking = append(staleBlocking, itoa(version))
		}
	}
	sort.Strings(staleBlocking)
	data.rule("blocking-ledger-fresh", "ledger", len(staleBlocking) == 0,
		"this ledger entry names a migration that no longer waits for a reader: "+strings.Join(staleBlocking, "; "))
}

// applyLifecycleRules is the empty database, the ladder, the snapshots and the
// simulated failure.
func applyLifecycleRules(data *reportData) {
	if len(data.Ladder) == 0 || len(data.Upgrades) == 0 {
		data.rule("ladder-reaches-head", "history", false, "the ladder measured nothing")
		data.rule("snapshot-lands-on-virgin", "snapshot", false, "no snapshot was upgraded")
		data.rule("upgrade-keeps-rows", "data", false, "no snapshot was upgraded")
		data.rule("expansion-is-monotonic", "structure", false, "no snapshot was upgraded")
		data.rule("history-complete-after-upgrade", "history", false, "no snapshot was upgraded")
		data.rule("failure-rolls-back", "failure", false, "no failure was simulated")
		data.rule("failure-recovers", "failure", false, "no failure was simulated")
		return
	}

	ladderHead := data.Ladder[len(data.Ladder)-1]
	data.rule("ladder-reaches-head", "history", ladderHead.Version == data.Ladder[0].Version+int64(len(data.Ladder)-1) && ladderHead.Relations > 0,
		fmt.Sprintf("the ladder stopped at version %d after %d step(s)", ladderHead.Version, len(data.Ladder)))

	notEqual := make([]string, 0)
	for _, step := range data.Upgrades {
		if !step.FingerprintOK {
			notEqual = append(notEqual, itoa(step.Version))
		}
	}
	data.rule("snapshot-lands-on-virgin", "snapshot", len(notEqual) == 0,
		"a database upgraded from these snapshots is not the database a fresh install produces: "+strings.Join(notEqual, ", "))

	lost := make([]string, 0)
	for _, step := range data.Upgrades {
		for _, table := range step.LostRows {
			lost = append(lost, fmt.Sprintf("from %d: %s", step.Version, table))
		}
	}
	data.rule("upgrade-keeps-rows", "data", len(lost) == 0,
		"an upgrade lost rows: "+strings.Join(lost, "; "))

	contractionsFound := make([]string, 0)
	for _, step := range data.Upgrades {
		for _, name := range step.Contractions {
			contractionsFound = append(contractionsFound, fmt.Sprintf("from %d: %s", step.Version, name))
		}
	}
	data.rule("expansion-is-monotonic", "structure", len(contractionsFound) == 0,
		"a column or table the snapshot had is gone at head, and expand/contract means it stays until a later release removes it: "+strings.Join(contractionsFound, "; "))

	incomplete := make([]string, 0)
	for _, step := range data.Upgrades {
		if !step.VersionTableOK {
			incomplete = append(incomplete, itoa(step.Version))
		}
	}
	data.rule("history-complete-after-upgrade", "history", len(incomplete) == 0,
		"the version table of an upgraded database does not list the history: "+strings.Join(incomplete, ", "))

	data.rule("failure-rolls-back", "failure",
		data.Failure.Version != 0 && !data.Failure.ProbeRecorded && len(data.Failure.Leftovers) == 0 &&
			len(data.Failure.Missing) == 0 && len(data.Failure.RowsLost) == 0,
		fmt.Sprintf("the failed migration left state behind: recorded=%t leftover=%v missing=%v lost-rows=%v",
			data.Failure.ProbeRecorded, data.Failure.Leftovers, data.Failure.Missing, data.Failure.RowsLost))
	data.rule("failure-recovers", "failure", data.Failure.Recovered,
		fmt.Sprintf("after the failure the history had %d migration(s) to run and did not reach the database a fresh install produces",
			data.Failure.RecoverApplied))
}

// applyOwnershipRules is who owns the schema and what the runtime role is.
func applyOwnershipRules(data *reportData, virgin *snapshot) {
	wrongOwner := make([]string, 0)
	for _, relation := range virgin.Relations {
		if relation.Owner != ownerRole {
			wrongOwner = append(wrongOwner, fmt.Sprintf("%s (%s)", relation.Name, relation.Owner))
		}
	}
	data.rule("owner-is-the-schema-owner", "ownership", len(wrongOwner) == 0,
		"every object of the schema belongs to "+ownerRole+": "+strings.Join(wrongOwner, ", "))

	role := virgin.Role
	data.rule("runtime-is-least-privilege", "role",
		!role.Super && !role.CreateDB && !role.CreateRole && !role.BypassRLS && !role.Replicate && role.CanLogin,
		fmt.Sprintf("%s: super=%t createdb=%t createrole=%t bypassrls=%t replication=%t login=%t",
			runtimeRole, role.Super, role.CreateDB, role.CreateRole, role.BypassRLS, role.Replicate, role.CanLogin))

	data.rule("schema-usage-without-create", "role",
		virgin.SchemaGrants["USAGE"] && !virgin.SchemaGrants["CREATE"],
		fmt.Sprintf("%s on schema %s: usage=%t create=%t", runtimeRole, schemaName,
			virgin.SchemaGrants["USAGE"], virgin.SchemaGrants["CREATE"]))

	data.rule("no-public-grants", "grants", len(virgin.PublicGrants) == 0,
		"PUBLIC holds privileges on the schema: "+strings.Join(virgin.PublicGrants, ", "))

	if table := virgin.VersionTable; table != nil {
		extra := make([]string, 0)
		for _, privilege := range tablePrivileges {
			expected := privilege == "SELECT"
			if table.Grants[privilege] != expected {
				extra = append(extra, fmt.Sprintf("%s=%t", privilege, table.Grants[privilege]))
			}
		}
		data.rule("history-is-read-only-for-the-runtime", "grants", len(extra) == 0,
			runtimeRole+" holds more than SELECT on the version table: "+strings.Join(extra, ", "))
	} else {
		data.rule("history-is-read-only-for-the-runtime", "grants", false, "the version table does not exist")
	}
}

// applyGrantRules holds the runtime's privileges to the four classes the ledger
// names. A table is either mutable (UPDATE and no DELETE), deletable (DELETE,
// declared), append-only (INSERT and no UPDATE, declared) or read-only (SELECT
// alone, declared). Nothing else is accepted: a new class of table is a review,
// not a diff that passes quietly.
func applyGrantRules(data *reportData, virgin *snapshot) {
	unreadable := make([]string, 0)
	undeclared := make([]string, 0)
	structural := make([]string, 0)
	for _, relation := range virgin.Relations {
		if relation.Kind == "S" {
			continue
		}
		if !relation.Grants["SELECT"] {
			unreadable = append(unreadable, relation.Name)
		}
		if relation.Grants["TRUNCATE"] || relation.Grants["REFERENCES"] || relation.Grants["TRIGGER"] {
			structural = append(structural, relation.Name)
		}
		_, appendOnly := appendOnlyTables[relation.Name]
		_, deletable := deletableTables[relation.Name]
		_, readOnly := readOnlyTables[relation.Name]
		switch {
		case relation.Grants["UPDATE"] && !relation.Grants["DELETE"]:
			if appendOnly || readOnly || deletable {
				undeclared = append(undeclared, relation.Name+" is mutable and the ledger classifies it as something else")
			}
		case relation.Grants["DELETE"] && !deletable:
			undeclared = append(undeclared, relation.Name+" lets the runtime delete rows and the ledger does not say so")
		case !relation.Grants["UPDATE"] && !relation.Grants["DELETE"] && relation.Grants["INSERT"] && !appendOnly:
			undeclared = append(undeclared, relation.Name+" accepts inserts and never updates and the ledger does not say so")
		case !relation.Grants["UPDATE"] && !relation.Grants["DELETE"] && !relation.Grants["INSERT"] && !readOnly:
			undeclared = append(undeclared, relation.Name+" is read-only for the runtime and the ledger does not say so")
		}
	}
	sort.Strings(unreadable)
	sort.Strings(undeclared)
	sort.Strings(structural)
	data.rule("runtime-reads-every-table", "grants", len(unreadable) == 0,
		runtimeRole+" cannot read: "+strings.Join(unreadable, ", "))
	data.rule("runtime-writes-are-declared", "grants", len(undeclared) == 0, strings.Join(undeclared, "; "))
	data.rule("runtime-holds-no-structural-privilege", "grants", len(structural) == 0,
		runtimeRole+" holds TRUNCATE, REFERENCES or TRIGGER on: "+strings.Join(structural, ", "))

	// The other direction: a ledger entry that stops matching the grants is a
	// classification that outlived the schema.
	stale := make([]string, 0)
	byName := map[string]relation{}
	for _, relation := range virgin.Relations {
		byName[relation.Name] = relation
	}
	for table, reason := range appendOnlyTables {
		current, present := byName[table]
		if !present {
			stale = append(stale, table+" (no such table) "+reason)
			continue
		}
		if !current.Grants["INSERT"] || current.Grants["UPDATE"] || current.Grants["DELETE"] {
			stale = append(stale, table+" is no longer append-only")
		}
	}
	for table, reason := range readOnlyTables {
		current, present := byName[table]
		if !present {
			stale = append(stale, table+" (no such table) "+reason)
			continue
		}
		if current.Grants["INSERT"] || current.Grants["UPDATE"] || current.Grants["DELETE"] {
			stale = append(stale, table+" is no longer read-only")
		}
	}
	for table, reason := range deletableTables {
		current, present := byName[table]
		if !present {
			stale = append(stale, table+" (no such table) "+reason)
			continue
		}
		if !current.Grants["DELETE"] {
			stale = append(stale, table+" no longer allows the runtime to delete")
		}
	}
	sort.Strings(stale)
	data.rule("grant-ledgers-are-fresh", "ledger", len(stale) == 0,
		"the ledger classifies a table the schema no longer matches: "+strings.Join(stale, ", "))
}

// applySequenceRules states the rule about sequences as what the runtime
// actually needs: USAGE and SELECT for the sequences of the tables it inserts
// into — an INSERT with a default reads nothing without them — and no privilege
// at all on the sequences of the tables it only reads, which is the stricter
// half of least privilege. UPDATE on any sequence is refused: it is the
// privilege that lets a session move the counter, and nothing in this product
// has a reason to.
func applySequenceRules(data *reportData, virgin *snapshot) {
	problems := make([]string, 0)
	for _, sequence := range virgin.Sequences {
		needsUsage := virgin.tableGrantsInsert(sequence.OwnedBy)
		describe := fmt.Sprintf("%s (owner=%s usage=%t select=%t update=%t)",
			sequence.Name, sequence.OwnedBy, sequence.Grants["USAGE"], sequence.Grants["SELECT"], sequence.Grants["UPDATE"])
		switch {
		case sequence.Grants["UPDATE"]:
			problems = append(problems, "the runtime may move the counter of "+describe)
		case needsUsage && (!sequence.Grants["USAGE"] || !sequence.Grants["SELECT"]):
			problems = append(problems, "the runtime inserts into the owning table and cannot use "+describe)
		case !needsUsage && sequence.Grants["USAGE"]:
			problems = append(problems, "the runtime never inserts into the owning table and may still use "+describe)
		}
	}
	sort.Strings(problems)
	data.rule("sequences-are-usable-and-immutable", "grants", len(problems) == 0,
		runtimeRole+" must hold USAGE and SELECT exactly on the sequences of the tables it inserts into, and never UPDATE: "+strings.Join(problems, "; "))
}

// applyIndexRules: an index the planner cannot trust, and two indexes that do
// the same work.
func applyIndexRules(data *reportData, virgin *snapshot) {
	invalid := make([]string, 0)
	duplicates := make([]string, 0)
	seen := map[string]string{}
	for _, relation := range virgin.Relations {
		for _, current := range relation.Indexes {
			if !current.Valid || !current.Ready {
				invalid = append(invalid, fmt.Sprintf("%s.%s (valid=%t ready=%t)", relation.Name, current.Name, current.Valid, current.Ready))
			}
			if len(current.Columns) == 0 {
				// An index over expressions only: PostgreSQL compares the
				// expressions, and two of them on the same table are two
				// different indexes even when both are on `to_tsvector`. The
				// audit cannot judge that from a column list, so it does not
				// pretend to.
				continue
			}
			key := relation.Name + ":" + strings.Join(current.Columns, ",")
			if previous, present := seen[key]; present {
				duplicates = append(duplicates, fmt.Sprintf("%s and %s on %s(%s)",
					previous, current.Name, relation.Name, strings.Join(current.Columns, ",")))
				continue
			}
			seen[key] = current.Name
		}
	}
	sort.Strings(invalid)
	data.rule("indexes-are-valid", "indexes", len(invalid) == 0,
		"an invalid or not-ready index is a promise the planner cannot use: "+strings.Join(invalid, ", "))
	data.rule("indexes-are-not-duplicated", "indexes", len(duplicates) == 0,
		"two indexes on the same columns of the same table: "+strings.Join(duplicates, "; "))
}

// applyForeignKeyRules: every foreign key stays inside the schema, every
// cascade is declared, and the index coverage of the referencing side is stated
// — the last one as an advisory, because an uncovered foreign key costs a scan
// on the parent's deletion path and does not by itself make a release unsafe.
func applyForeignKeyRules(data *reportData, virgin *snapshot) {
	remote := make([]string, 0)
	covered := make([]string, 0)
	partial := make([]string, 0)
	uncovered := make([]string, 0)
	cascadeFound := map[string]bool{}
	for _, relation := range virgin.byName() {
		for _, current := range relation.Constraints {
			if current.Kind != "f" {
				continue
			}
			if !strings.HasPrefix(current.Referenced, schemaName+".") {
				remote = append(remote, relation.Name+"."+current.Name+" -> "+current.Referenced)
			}
			key := relation.Name + "." + current.Name
			if current.DeleteAction == "c" {
				cascadeFound[key] = true
			}
			columns := foreignKeyColumns(current.Definition)
			switch {
			case coveredByIndex(relation, columns):
				covered = append(covered, key)
			case len(columns) > 1 && coveredByLeadingColumn(relation, columns[0]):
				// A composite foreign key whose leading column is indexed can
				// still be resolved by the planner with a filter on the rest.
				partial = append(partial, fmt.Sprintf("%s (%s: %s leads an index)",
					key, strings.Join(columns, ","), columns[0]))
			default:
				uncovered = append(uncovered, fmt.Sprintf("%s (%s)", key, strings.Join(columns, ",")))
			}
		}
	}
	sort.Strings(remote)
	sort.Strings(partial)
	sort.Strings(uncovered)
	data.rule("foreign-keys-are-local", "foreign keys", len(remote) == 0,
		"a foreign key leaves the schema: "+strings.Join(remote, ", "))

	undeclaredCascades := make([]string, 0)
	staleCascades := make([]string, 0)
	for key := range cascadeFound {
		if _, declared := cascadeLedger[key]; !declared {
			undeclaredCascades = append(undeclaredCascades, key)
		}
	}
	for key := range cascadeLedger {
		if !cascadeFound[key] {
			staleCascades = append(staleCascades, key)
		}
	}
	sort.Strings(undeclaredCascades)
	sort.Strings(staleCascades)
	data.rule("cascades-are-declared", "cascades", len(undeclaredCascades) == 0,
		"deleting the parent deletes these rows and the ledger does not say why: "+strings.Join(undeclaredCascades, ", "))
	data.rule("cascade-ledger-is-fresh", "ledger", len(staleCascades) == 0,
		"the ledger names cascades the schema does not have: "+strings.Join(staleCascades, ", "))

	detail := fmt.Sprintf("%d of %d foreign key(s) have no index leading with their columns", len(uncovered), len(covered)+len(partial)+len(uncovered))
	if len(uncovered) > 0 {
		detail += ": " + strings.Join(uncovered, "; ")
	}
	if len(partial) > 0 {
		detail += " (partially covered, leading column only: " + strings.Join(partial, "; ") + ")"
	}
	data.rule("foreign-keys-are-indexed", "foreign keys", len(uncovered) == 0, detail)
}

// applyStructureRules: the schema matches the history that declares it, and the
// data the audit wrote actually travelled through every version.
func applyStructureRules(data *reportData, virgin *snapshot) {
	missing := make([]string, 0)
	undeclared := make([]string, 0)
	for name, version := range data.Declared {
		if _, present := virgin.byName()[name]; !present {
			missing = append(missing, fmt.Sprintf("%s (declared by %d)", name, version))
		}
	}
	for name := range virgin.byName() {
		if _, declared := data.Declared[name]; !declared {
			undeclared = append(undeclared, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(undeclared)
	all := append(append([]string{}, missing...), undeclared...)
	data.rule("schema-matches-the-history", "schema", len(all) == 0,
		fmt.Sprintf("the database and the forward halves disagree about which tables exist (declared %d, catalog %d): %s",
			len(data.Declared), len(virgin.byName()), strings.Join(all, ", ")))

	data.rule("dataset-floor", "dataset", len(data.Dataset.Seeded) >= minSeedableTables,
		fmt.Sprintf("the audit could seed %d table(s) and the floor is %d", len(data.Dataset.Seeded), minSeedableTables))
	data.rule("dataset-travelled", "dataset", len(data.Dataset.Seeded) > 0,
		"the audit wrote no row at all, so data survival would be vacuous")
}

// byName indexes the relations of a snapshot by table name.
func (snapshot *snapshot) byName() map[string]relation {
	indexed := make(map[string]relation, len(snapshot.Relations))
	for _, relation := range snapshot.Relations {
		if relation.Kind == "S" {
			continue
		}
		indexed[relation.Name] = relation
	}
	return indexed
}

// tableGrantsInsert reports whether the runtime may insert into the named
// table, which is what makes the sequence behind it necessary.
func (snapshot *snapshot) tableGrantsInsert(name string) bool {
	if name == "" {
		return false
	}
	relation, present := snapshot.byName()[name]
	return present && relation.Grants["INSERT"]
}

// foreignKeyColumns reads the referencing columns out of a constraint
// definition, which is `FOREIGN KEY (a, b) REFERENCES ...`.
func foreignKeyColumns(definition string) []string {
	start := strings.Index(definition, "(")
	if start < 0 {
		return nil
	}
	end := strings.Index(definition[start:], ")")
	if end < 0 {
		return nil
	}
	columns := strings.Split(definition[start+1:start+end], ",")
	cleaned := make([]string, 0, len(columns))
	for _, column := range columns {
		cleaned = append(cleaned, strings.Trim(strings.TrimSpace(column), `"`))
	}
	return cleaned
}

// coveredByIndex reports whether some index of the table leads with exactly
// these columns: PostgreSQL can use it to find the children of a parent row
// without reading the table.
func coveredByIndex(relation relation, columns []string) bool {
	if len(columns) == 0 {
		return true
	}
	for _, current := range relation.Indexes {
		if len(current.Columns) < len(columns) {
			continue
		}
		matches := true
		for index, column := range columns {
			if current.Columns[index] != column {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

// coveredByLeadingColumn reports whether some index leads with this column,
// which a composite foreign key can use with a filter on the rest.
func coveredByLeadingColumn(relation relation, column string) bool {
	for _, current := range relation.Indexes {
		if len(current.Columns) > 0 && current.Columns[0] == column {
			return true
		}
	}
	return false
}
