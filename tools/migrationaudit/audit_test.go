// The tests of the migration audit (P20-T03).
//
// Two kinds, both without a database:
//
//   - the pure parts — reading a migration into its halves, telling a
//     replacement from a contraction, reading a foreign key's columns, judging
//     the version table, escaping the report — are tested directly, because
//     they are the parts that decide what the audit *measures* and a mistake
//     there would be a mistake in every conclusion;
//   - the rules are tested against synthetic measurements, one mutation per
//     rule, so a rule that stops refusing is caught without waiting for a
//     PostgreSQL: the ledger falsifications below delete an entry and assert
//     the rule that exists to justify it goes red.
//
// The database half — an empty database, the ladder with an active reader, 31
// snapshots rolled forward, a migration that fails halfway — is measured by
// `tools/migrationaudit/verify.sh` against PostgreSQL 18.4, never here: SQLite
// is prohibited by the plan and a mock would measure a different engine.
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSplitHalvesReadsBothMarkers(t *testing.T) {
	up, down, err := splitHalves("-- +goose Up\nCREATE TABLE app.a (id int);\n-- +goose Down\nDROP TABLE app.a;\n")
	if err != nil {
		t.Fatalf("splitHalves: %v", err)
	}
	if !strings.Contains(up, "CREATE TABLE") || strings.Contains(up, "DROP TABLE") {
		t.Fatalf("the forward half is wrong: %q", up)
	}
	if !strings.Contains(down, "DROP TABLE") {
		t.Fatalf("the rollback half is wrong: %q", down)
	}

	if _, _, err := splitHalves("CREATE TABLE app.a (id int);\n"); err == nil {
		t.Fatal("a file without the Up marker has to be an error: guessing which half runs would audit statements the runner may never execute")
	}
}

// TestScanDestructiveTellsReplacementFromRemoval is the rule that keeps the
// gate meaningful: this history drops 46 objects and recreates every one of
// them, and a scan that counted those as removals would demand a justification
// for each of them and hide the first real DROP.
func TestScanDestructiveTellsReplacementFromRemoval(t *testing.T) {
	cases := []struct {
		name     string
		up       string
		object   string
		replaced bool
	}{
		{
			name:     "a trigger dropped and recreated under the same name",
			up:       "DROP TRIGGER IF EXISTS arenas_guard ON app.arenas;\nCREATE TRIGGER arenas_guard BEFORE UPDATE ON app.arenas FOR EACH ROW EXECUTE FUNCTION app.arenas_protect();\n",
			object:   "arenas_guard",
			replaced: true,
		},
		{
			name:     "a constraint dropped and re-added under the same name",
			up:       "ALTER TABLE app.wallet_operations DROP CONSTRAINT IF EXISTS wallet_operations_type_check;\nALTER TABLE app.wallet_operations ADD CONSTRAINT wallet_operations_type_check CHECK (operation_type IN ('credit_free'));\n",
			object:   "wallet_operations_type_check",
			replaced: true,
		},
		{
			name:     "a column dropped for good",
			up:       "ALTER TABLE app.profiles DROP COLUMN IF EXISTS nickname;\n",
			object:   "nickname",
			replaced: false,
		},
		{
			name:     "a table dropped for good",
			up:       "DROP TABLE app.legacy_profiles;\n",
			object:   "legacy_profiles",
			replaced: false,
		},
		{
			name:     "a name mentioned in a comment is not a recreation",
			up:       "-- DROP TRIGGER IF EXISTS arenas_guard ON app.arenas; the guard is gone\nDROP TRIGGER IF EXISTS arenas_guard ON app.arenas;\n",
			object:   "arenas_guard",
			replaced: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			sources := []source{{Version: 1, Path: "1_x.sql", Name: "00001_x.sql", Up: testCase.up}}
			found := scanDestructive(sources)
			if len(found) != 1 {
				t.Fatalf("found %d statement(s), want 1: %+v", len(found), found)
			}
			if found[0].Object != testCase.object {
				t.Fatalf("object = %q, want %q", found[0].Object, testCase.object)
			}
			if found[0].Replaced != testCase.replaced {
				t.Fatalf("replaced = %t, want %t (statement %q)", found[0].Replaced, testCase.replaced, found[0].Statement)
			}
			if testCase.replaced && len(contractions(found)) != 0 {
				t.Fatal("a replacement is not a contraction")
			}
			if !testCase.replaced && len(replacements(found)) != 0 {
				t.Fatal("a removal is not a replacement")
			}
		})
	}
}

// TestDeclaredTablesReadsTheHistory: the schema check is only as good as this
// list, so it has to read `CREATE TABLE IF NOT EXISTS app.x` with and without
// the quoting, and never a table of another schema.
func TestDeclaredTablesReadsTheHistory(t *testing.T) {
	sources := []source{
		{Version: 1, Up: "CREATE TABLE IF NOT EXISTS app.accounts (id uuid);\nCREATE TABLE other.ignored (id int);\n"},
		{Version: 2, Up: "CREATE TABLE app.\"wallet_accounts\" (id int);\n"},
	}
	declared := declaredTables(sources)
	if len(declared) != 2 {
		t.Fatalf("declared %v, want exactly the two tables of the app schema", declared)
	}
	if declared["accounts"] != 1 || declared["wallet_accounts"] != 2 {
		t.Fatalf("declared %v, want accounts from 1 and wallet_accounts from 2", declared)
	}
}

// TestVersionTableIsComplete: the version table carries a row for its own
// creation (version 0), and history that repeats, loses or unapplies a version
// is not a history.
func TestVersionTableIsComplete(t *testing.T) {
	sources := []source{{Version: 1}, {Version: 2}, {Version: 3}}

	cases := []struct {
		name  string
		rows  []versionRow
		valid bool
	}{
		{"the gooseless shape", []versionRow{{Version: 0, Applied: true}, {Version: 1, Applied: true}, {Version: 2, Applied: true}, {Version: 3, Applied: true}}, true},
		{"the same without the version table's own row", []versionRow{{Version: 1, Applied: true}, {Version: 2, Applied: true}, {Version: 3, Applied: true}}, true},
		{"a lost version", []versionRow{{Version: 1, Applied: true}, {Version: 3, Applied: true}}, false},
		{"a repeated version", []versionRow{{Version: 1, Applied: true}, {Version: 2, Applied: true}, {Version: 2, Applied: true}, {Version: 3, Applied: true}}, false},
		{"a version recorded as not applied", []versionRow{{Version: 1, Applied: true}, {Version: 2, Applied: false}, {Version: 3, Applied: true}}, false},
		{"a version the history does not carry", []versionRow{{Version: 1, Applied: true}, {Version: 2, Applied: true}, {Version: 3, Applied: true}, {Version: 4, Applied: true}}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := versionTableIsComplete(&snapshot{Version: testCase.rows}, sources); got != testCase.valid {
				t.Fatalf("versionTableIsComplete = %t, want %t", got, testCase.valid)
			}
		})
	}
}

// TestForeignKeyColumnsAndCoverage pins the two halves of the index coverage
// rule: where the columns come from and what counts as covered.
func TestForeignKeyColumnsAndCoverage(t *testing.T) {
	columns := foreignKeyColumns("FOREIGN KEY (parent_id, arena_id) REFERENCES app.arguments(id, arena_id)")
	if strings.Join(columns, ",") != "parent_id,arena_id" {
		t.Fatalf("columns = %v, want parent_id,arena_id", columns)
	}

	arguments := relation{
		Name:    "arguments",
		Indexes: []index{{Name: "arguments_parent_idx", Columns: []string{"parent_id", "created_at", "id"}}},
	}
	if !coveredByIndex(arguments, []string{"parent_id"}) {
		t.Fatal("an index leading with the single column covers it")
	}
	if coveredByIndex(arguments, []string{"parent_id", "arena_id"}) {
		t.Fatal("a composite key whose second column is not in the index is not *covered* by it")
	}
	if !coveredByLeadingColumn(arguments, "parent_id") {
		t.Fatal("the leading column of the index makes the composite key partially covered, which is what the report states")
	}
	if coveredByIndex(arguments, []string{"arena_id"}) || coveredByLeadingColumn(arguments, "arena_id") {
		t.Fatal("the second column of a composite key is not covered by an index that leads with the first")
	}
	// An expression index has no columns and cannot cover anything: the audit
	// never counts it as coverage.
	expression := relation{Indexes: []index{{Name: "arenas_public_search_en_fts_idx", Columns: nil}}}
	if coveredByIndex(expression, []string{"category"}) || coveredByLeadingColumn(expression, "category") {
		t.Fatal("an expression index covers no column")
	}
}

// fixture is a measured run, built from the ledgers themselves: a schema whose
// tables hold exactly the privileges the ledgers classify, whose foreign keys
// are covered by an index, and a ladder in which exactly the versions the
// blocking ledger names wait for an active reader.
//
// It is a model, not the real schema — the real one is measured by
// `tools/migrationaudit/verify.sh` against PostgreSQL, and the unit tests here
// must not need a database. Deriving it from the ledgers is what makes the two
// kinds of test complementary rather than contradictory: the fixture fails if
// the rules stop refusing, and the database run fails if the ledgers stop
// matching the schema.
func fixture() *reportData {
	relations := make([]relation, 0, len(appendOnlyTables)+len(readOnlyTables)+len(deletableTables)+2)
	declared := map[string]int64{}

	add := func(name string, grants ...string) relation {
		table := relation{Name: name, Kind: "r", Owner: ownerRole, Grants: map[string]bool{}}
		for _, privilege := range grants {
			table.Grants[privilege] = true
		}
		table.Indexes = []index{{Name: name + "_pkey", Columns: []string{"id"}, Valid: true, Ready: true, Unique: true}}
		relations = append(relations, table)
		declared[name] = 1
		return table
	}

	// Every table of every ledger, with exactly the privileges its class
	// carries. A table outside the three ledgers that the runtime touches is
	// mutable, which is the class with no entry.
	for table := range appendOnlyTables {
		add(table, "SELECT", "INSERT")
	}
	for table := range readOnlyTables {
		add(table, "SELECT")
	}
	for table := range deletableTables {
		add(table, "SELECT", "INSERT", "UPDATE", "DELETE")
	}
	add("account_mfa", "SELECT", "INSERT", "UPDATE")

	// Every cascade the ledger names, on a table of this schema and covered by
	// an index: coverage is the advisory rule, and this fixture is a clean run.
	for key := range cascadeLedger {
		child, name := splitIndexKey(key)
		relation, present := indexOf(relations, child)
		if !present {
			relation = add(child, "SELECT", "INSERT", "UPDATE")
		}
		relation.Constraints = append(relation.Constraints, constraint{
			Name:         name,
			Kind:         "f",
			Definition:   "FOREIGN KEY (ref_id) REFERENCES app.accounts(id)",
			Referenced:   "app.accounts",
			DeleteAction: "c",
		})
		relation.Indexes = append(relation.Indexes, index{Name: child + "_ref_idx", Columns: []string{"ref_id"}, Valid: true, Ready: true})
		replace(relations, relation)
	}

	// The ladder: every version up to head, with the blocked set of the ledger.
	versions := int64(0)
	for version := range blockingLedger {
		if version > versions {
			versions = version
		}
	}
	ladder := make([]ladderStep, 0, versions)
	for version := int64(1); version <= versions; version++ {
		step := ladderStep{Version: version, Name: "00001_app_schema.sql", Relations: len(relations), Rows: 4}
		if _, blocked := blockingLedger[version]; blocked {
			step.Blocked = true
			step.Waiting = []lockObservation{{Mode: "AccessExclusiveLock", Target: "app.arenas"}}
		}
		ladder = append(ladder, step)
	}

	schemaMetadata := relation{}
	if table, present := indexOf(relations, "schema_metadata"); present {
		schemaMetadata = table
	}
	versionRows := make([]versionRow, 0, versions+1)
	versionRows = append(versionRows, versionRow{Version: 0, Applied: true})
	for version := int64(1); version <= versions; version++ {
		versionRows = append(versionRows, versionRow{Version: version, Applied: true})
	}

	data := &reportData{
		Versions: int(versions),
		Virgin: &snapshot{
			Relations: relations,
			Sequences: []relation{
				{Name: "arenas_id_seq", Kind: "S", OwnedBy: "arenas", Grants: map[string]bool{"USAGE": true, "SELECT": true}},
				{Name: "schema_metadata_id_seq", Kind: "S", OwnedBy: "schema_metadata", Grants: map[string]bool{}},
			},
			SchemaGrants: map[string]bool{"USAGE": true},
			Role:         roleAttributes{CanLogin: true},
			Version:      versionRows,
			VersionTable: &schemaMetadata,
		},
		Declared: declared,
		Ladder:   ladder,
		Upgrades: []upgradeStep{{Version: versions, Rest: 0, FingerprintOK: true, VersionTableOK: true}},
		Failure:  failureStep{Version: versions, Error: "division by zero", AppliedBeforeFailure: 0, RecoverApplied: 0, Recovered: true},
		Dataset:  dataset{Seeded: []string{"categories", "schema_metadata"}, Floor: minSeedableTables, Rows: int64(len(relations))},
	}
	return data
}

// splitIndexKey turns a ledger key ("child.fk-name") into its two halves.
func splitIndexKey(key string) (child, name string) {
	for index, character := range key {
		if character == '.' {
			return key[:index], key[index+1:]
		}
	}
	return key, key
}

func indexOf(relations []relation, name string) (relation, bool) {
	for _, relation := range relations {
		if relation.Name == name {
			return relation, true
		}
	}
	return relation{}, false
}

func replace(relations []relation, updated relation) {
	for index := range relations {
		if relations[index].Name == updated.Name {
			relations[index] = updated
			return
		}
	}
}

// rulesByName indexes the results of a run.
func rulesByName(data *reportData) map[string]ruleResult {
	results := map[string]ruleResult{}
	for _, rule := range data.Rules {
		results[rule.Name] = rule
	}
	return results
}

// TestTheFixturePasses proves the fixture is a *valid* run, not a pile of
// findings: a test built on a red fixture would pass for the wrong reason.
func TestTheFixturePasses(t *testing.T) {
	data := fixture()
	applyRules(data)
	findings, advisories := evaluateRules(data)
	if len(findings) != 0 {
		t.Fatalf("the fixture is not a clean run: %v", findings)
	}
	if len(advisories) != 0 {
		t.Fatalf("the fixture reports advice: %v", advisories)
	}
	if len(data.Rules) < 20 {
		t.Fatalf("only %d rule(s) ran: the fixture is not exercising the catalogue", len(data.Rules))
	}
	for _, ledger := range []map[string]string{appendOnlyTables, readOnlyTables, deletableTables} {
		for table := range ledger {
			if _, present := data.Virgin.byName()[table]; !present {
				t.Fatalf("the fixture has no table %s, which a ledger classifies: the two would not be comparing the same schema", table)
			}
		}
	}
}

// TestAdvisoryRulesAreClosed fixes the set of rules that report without
// refusing. Adding a second one is an edit to this test, in review: demoting a
// gate has to be visible.
func TestAdvisoryRulesAreClosed(t *testing.T) {
	failing := map[string]bool{}
	data := fixture()
	// Break the one class the advisory rules cover: a foreign key with no index
	// at all.
	uncovered := -1
	for index, relation := range data.Virgin.Relations {
		if len(relation.Constraints) > 0 {
			uncovered = index
			break
		}
	}
	if uncovered < 0 {
		t.Fatal("the fixture has no foreign key to uncover")
	}
	data.Virgin.Relations[uncovered].Indexes = nil
	applyRules(data)
	findings, advisories := evaluateRules(data)
	for _, rule := range data.Rules {
		if !rule.Passed && rule.Severity == severityAdvisory {
			failing[rule.Name] = true
		}
	}
	if len(failing) != len(advisoryRules) || !failing["foreign-keys-are-indexed"] {
		t.Fatalf("the advisory rules that failed are %v, want exactly %v", failing, advisoryRules)
	}
	// Every advisory names the work it is handed to, and every follow-up
	// belongs to an advisory that exists: the two maps cannot grow apart.
	for name, followUp := range advisoryRules {
		if strings.TrimSpace(followUp) == "" {
			t.Fatalf("the advisory %s has no follow-up: an advisory nobody owns is not a finding", name)
		}
		if len(data.Rules) == 0 {
			t.Fatalf("the advisory %s is not among the rules that ran", name)
		}
		found := false
		for _, rule := range data.Rules {
			if rule.Name == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("the advisory %s is not among the rules that ran", name)
		}
	}
	if len(findings) != 0 {
		t.Fatalf("an advisory finding refused the run: %v", findings)
	}
	if len(advisories) != 1 {
		t.Fatalf("advisories = %v, want one", advisories)
	}
	if len(data.Virgin.Relations[uncovered].Constraints) == 0 ||
		!strings.Contains(advisories[0], data.Virgin.Relations[uncovered].Constraints[0].Name) {
		t.Fatalf("the advisory does not name the foreign key: %q", advisories[0])
	}
}

// TestUnindexedForeignKeyIsAdvisoryButAnUndeclaredCascadeIsNot pins the two
// halves of the foreign key audit: coverage is recorded, a cascade nobody
// declared refuses.
func TestForeignKeysSeparateTheRecordedFromTheRefused(t *testing.T) {
	// The fixture is built first and the ledger changed afterwards: the fixture
	// is derived from the ledgers, so a mutation before it would simply produce
	// a smaller schema instead of an inconsistency.
	key := "profiles.profiles_account_id_fkey"
	reason, present := cascadeLedger[key]
	if !present {
		t.Fatalf("%s is not in the ledger, so this test cannot falsify it", key)
	}
	data := fixture()
	delete(cascadeLedger, key)
	defer func() { cascadeLedger[key] = reason }()

	applyRules(data)
	findings, _ := evaluateRules(data)
	if len(findings) != 1 {
		t.Fatalf("an undeclared cascade has to refuse once, and it produced %v", findings)
	}
	if !strings.Contains(findings[0], key) {
		t.Fatalf("the finding does not name the key: %s", findings[0])
	}

	// Declared, it stops refusing: this is what the ledger is for.
	cascadeLedger[key] = reason
	declared := fixture()
	applyRules(declared)
	if findings, _ := evaluateRules(declared); len(findings) != 0 {
		t.Fatalf("the declared cascades refused: %v", findings)
	}
}

// TestGrantLedgersAreFalsifiable removes one entry of each grant ledger and
// asserts the audit refuses the table it described. A ledger that cannot fail
// is a comment.
func TestGrantLedgersAreFalsifiable(t *testing.T) {
	cases := []struct {
		name  string
		table string
		from  map[string]string
		rule  string
	}{
		{"an append-only table", "wallet_transactions", appendOnlyTables, "runtime-writes-are-declared"},
		{"a read-only table", "categories", readOnlyTables, "runtime-writes-are-declared"},
		{"a deletable table", "sessions", deletableTables, "runtime-writes-are-declared"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reason, present := testCase.from[testCase.table]
			if !present {
				t.Fatalf("%s is not classified at all", testCase.table)
			}
			// Built before the mutation, so the schema keeps the grants the
			// ledger used to classify and the rule sees the contradiction.
			data := fixture()
			delete(testCase.from, testCase.table)
			defer func() { testCase.from[testCase.table] = reason }()

			applyRules(data)
			rule, present := rulesByName(data)[testCase.rule]
			if !present {
				t.Fatalf("the rule %s did not run", testCase.rule)
			}
			if rule.Passed {
				t.Fatalf("%s passed with %s unclassified: %q", testCase.rule, testCase.table, rule.Detail)
			}
			if !strings.Contains(rule.Detail, testCase.table) {
				t.Fatalf("the detail does not name %s: %q", testCase.table, rule.Detail)
			}
		})
	}
}

// TestGrantLedgerFreshnessIsCheckedInBothDirections: a classification that
// outlived the schema is a decision nobody is enforcing any more.
func TestGrantLedgerFreshnessIsChecked(t *testing.T) {
	appendOnlyTables["arenas"] = "test fixture: arenas declared append-only"
	defer delete(appendOnlyTables, "arenas")

	data := fixture()
	applyRules(data)
	rule := rulesByName(data)["grant-ledgers-are-fresh"]
	if rule.Passed {
		t.Fatal("a mutable table declared append-only has to fail the freshness rule")
	}
	if !strings.Contains(rule.Detail, "arenas") {
		t.Fatalf("the detail does not name the table: %q", rule.Detail)
	}
}

// TestBlockingMigrationsAreFalsifiable: a migration that waits for an active
// reader and a ledger entry that names a migration which no longer waits are
// both failures, in the two directions.
func TestBlockingLedgerIsCheckedInBothDirections(t *testing.T) {
	data := fixture()
	data.Ladder = append(data.Ladder, ladderStep{Version: 2, Name: "00002_x.sql", Blocked: true,
		Waiting: []lockObservation{{Mode: "AccessExclusiveLock", Target: "app.arenas"}}})
	applyRules(data)
	rule := rulesByName(data)["blocking-declared"]
	if rule.Passed || !strings.Contains(rule.Detail, "AccessExclusiveLock") {
		t.Fatalf("an undeclared blocking migration has to be refused with the lock it waited for: %+v", rule)
	}

	declared := fixture()
	blockingLedger[2] = "test fixture"
	defer delete(blockingLedger, 2)
	applyRules(declared)
	fresh := rulesByName(declared)["blocking-ledger-fresh"]
	if fresh.Passed {
		t.Fatal("an entry that names a migration which does not wait has to fail the freshness rule")
	}
}

// TestContractionLedgerIsChecked: a removal nobody recorded refuses, and a
// recorded removal that no migration states is stale.
func TestContractionLedgerIsChecked(t *testing.T) {
	data := fixture()
	data.Destructive = []destructive{{Version: 1, Name: "00001_x.sql", What: "DROP", Object: "nickname",
		Statement: "DROP COLUMN nickname", Key: "1 DROP COLUMN nickname"}}
	applyRules(data)
	rule := rulesByName(data)["destructive-declared"]
	if rule.Passed || !strings.Contains(rule.Detail, "nickname") {
		t.Fatalf("a removal that is not in the ledger has to be refused: %+v", rule)
	}

	declared := fixture()
	declared.Destructive = data.Destructive
	contractionLedger["1 DROP COLUMN nickname"] = "test fixture"
	defer delete(contractionLedger, "1 DROP COLUMN nickname")
	applyRules(declared)
	if findings, _ := evaluateRules(declared); len(findings) != 0 {
		t.Fatalf("a declared removal still refused: %v", findings)
	}

	// The same entry, with a measurement that does not state it: stale.
	stale := fixture()
	applyRules(stale)
	if rule := rulesByName(stale)["destructive-ledger-fresh"]; rule.Passed {
		t.Fatal("an entry that no migration states is stale and has to fail")
	}
}

// TestLifecycleRulesFailOnBrokenMeasurements walks the three lifecycle rules
// that carry the phase's minimum validation: every snapshot landing on the
// fresh database, no lost rows, no lost columns, and the failure rolling back
// and recovering.
func TestLifecycleRulesFailOnBrokenMeasurements(t *testing.T) {
	cases := []struct {
		name   string
		rule   string
		broken func(data *reportData)
	}{
		{"a snapshot that does not land on the reference database", "snapshot-lands-on-virgin", func(d *reportData) { d.Upgrades[0].FingerprintOK = false }},
		{"an upgrade that lost rows", "upgrade-keeps-rows", func(d *reportData) { d.Upgrades[0].LostRows = []string{"sessions (2 -> 1)"} }},
		{"an upgrade that lost a column", "expansion-is-monotonic", func(d *reportData) { d.Upgrades[0].Contractions = []string{"app.arenas.nickname"} }},
		{"an upgrade whose history is incomplete", "history-complete-after-upgrade", func(d *reportData) { d.Upgrades[0].VersionTableOK = false }},
		{"a failed migration that recorded itself", "failure-rolls-back", func(d *reportData) { d.Failure.ProbeRecorded = true }},
		{"a failed migration that left a table behind", "failure-rolls-back", func(d *reportData) { d.Failure.Leftovers = []string{"migration_audit_probe"} }},
		{"a failed migration that lost rows", "failure-rolls-back", func(d *reportData) { d.Failure.RowsLost = []string{"sessions (2 -> 1)"} }},
		{"a recovery that did not reach the reference database", "failure-recovers", func(d *reportData) { d.Failure.Recovered = false }},
		{"a schema the history does not declare", "schema-matches-the-history", func(d *reportData) {
			d.Virgin.Relations = append(d.Virgin.Relations, relation{Name: "orphan", Kind: "r", Owner: ownerRole, Grants: map[string]bool{"SELECT": true}})
		}},
		{"a history that declares a table the catalog does not have", "schema-matches-the-history", func(d *reportData) { d.Declared["ghost"] = 2 }},
		{"a runtime that cannot read a table", "runtime-reads-every-table", func(d *reportData) { d.Virgin.Relations[0].Grants["SELECT"] = false }},
		{"a runtime that may truncate", "runtime-holds-no-structural-privilege", func(d *reportData) { d.Virgin.Relations[0].Grants["TRUNCATE"] = true }},
		{"a runtime that may move a sequence", "sequences-are-usable-and-immutable", func(d *reportData) { d.Virgin.Sequences[0].Grants["UPDATE"] = true }},
		{"a runtime that may use a sequence it never needs", "sequences-are-usable-and-immutable", func(d *reportData) { d.Virgin.Sequences[0].OwnedBy = "categories" }},
		{"a table owned by the runtime", "owner-is-the-schema-owner", func(d *reportData) { d.Virgin.Relations[0].Owner = runtimeRole }},
		{"a superuser runtime", "runtime-is-least-privilege", func(d *reportData) { d.Virgin.Role.Super = true }},
		{"a runtime that may create in the schema", "schema-usage-without-create", func(d *reportData) { d.Virgin.SchemaGrants["CREATE"] = true }},
		{"a privilege held by PUBLIC", "no-public-grants", func(d *reportData) { d.Virgin.PublicGrants = []string{"arenas:SELECT"} }},
		{"a version table the runtime may write", "history-is-read-only-for-the-runtime", func(d *reportData) { d.Virgin.VersionTable.Grants["UPDATE"] = true }},
		{"an invalid index", "indexes-are-valid", func(d *reportData) { d.Virgin.Relations[0].Indexes[0].Valid = false }},
		{"a duplicated index", "indexes-are-not-duplicated", func(d *reportData) {
			d.Virgin.Relations[0].Indexes = append(d.Virgin.Relations[0].Indexes, index{Name: "arenas_pkey_copy", Columns: []string{"id"}, Valid: true, Ready: true})
		}},
		{"a foreign key that leaves the schema", "foreign-keys-are-local", func(d *reportData) {
			for index := range d.Virgin.Relations {
				if len(d.Virgin.Relations[index].Constraints) > 0 {
					d.Virgin.Relations[index].Constraints[0].Referenced = "public.accounts"
					return
				}
			}
		}}, {"a dataset below the floor", "dataset-floor", func(d *reportData) { d.Dataset.Seeded = []string{"categories"} }},
		{"a dataset that wrote nothing at all", "dataset-travelled", func(d *reportData) { d.Dataset.Seeded = nil }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			data := fixture()
			testCase.broken(data)
			applyRules(data)
			rule, present := rulesByName(data)[testCase.rule]
			if !present {
				t.Fatalf("the rule %s did not run", testCase.rule)
			}
			if rule.Passed {
				t.Fatalf("%s passed on a broken measurement: %q", testCase.rule, rule.Detail)
			}
			if rule.Severity != severityGate {
				t.Fatalf("%s is %s, and every rule in this table has to refuse", testCase.rule, rule.Severity)
			}
			if findings, _ := evaluateRules(data); len(findings) == 0 {
				t.Fatal("a failed gate rule produced no finding")
			}
		})
	}
}

// TestRenderWritesTheSections: the report is the deliverable, so its shape is
// asserted rather than assumed, including the two sections that only exist when
// there is something to say.
func TestRenderWritesTheSections(t *testing.T) {
	data := fixture()
	data.Commit = "0123456789abcdef"
	applyRules(data)
	data.Findings, data.Advisories = evaluateRules(data)

	var output bytes.Buffer
	render(data, &output)
	rendered := output.String()

	for _, section := range []string{
		"# Relatório da auditoria de migrations",
		"## 1. Banco vazio",
		"## 2. Degrau",
		"## 3. Snapshot e upgrade",
		"## 4. Falha simulada",
		"## 5. Dataset",
		"## 6. Regras",
		"## 6b. Substituições",
		"## 7. Ledgers",
	} {
		if !strings.Contains(rendered, section) {
			t.Fatalf("the report has no %q section", section)
		}
	}
	if !strings.Contains(rendered, "0123456") {
		t.Fatal("the report does not name the commit it measured")
	}
	// The advisory sections are absent when there is nothing to report: a
	// section that always prints "none" is a section nobody reads.
	if strings.Contains(rendered, "## 9. Advertências") {
		t.Fatal("a clean run printed the advisory section")
	}

	data.Advisories = []string{"foreign-keys-are-indexed: one key uncovered"}
	data.Rules = append(data.Rules, ruleResult{Name: "foreign-keys-are-indexed", Subject: "foreign keys", Severity: severityAdvisory, Passed: false, Detail: "one key uncovered"})
	output.Reset()
	render(data, &output)
	rendered = output.String()
	if !strings.Contains(rendered, "## 9. Advertências") {
		t.Fatal("an advisory run has to print the advisory section")
	}
	if !strings.Contains(rendered, "Trabalho seguinte") {
		t.Fatal("the advisory section has to name the work the finding is handed to")
	}
}

// TestSummarizeAndEscape holds the two pieces of the report that are not a
// table: the summary an operator watches, and the escaping that keeps a pipe in
// a detail from breaking the row it is printed in.
func TestSummarizeAndEscape(t *testing.T) {
	data := fixture()
	applyRules(data)
	var output bytes.Buffer
	summarize(data, &output)
	if !strings.Contains(output.String(), "rule(s) checked") {
		t.Fatalf("the summary does not count the rules: %q", output.String())
	}
	if got, want := escape("a | b\nc"), "a \\| b c"; got != want {
		t.Fatalf("escape = %q, want %q: a pipe in a detail would break the table it is printed in", got, want)
	}
	if got, want := oneLine("two\nlines\tand a tab"), "two lines and a tab"; got != want {
		t.Fatalf("oneLine = %q, want %q", got, want)
	}
}
