// Reading the migration history as the source of the audit (P20-T03).
//
// The runner applies migrations from the embedded filesystem, so the audit has
// to look at the same bytes an operator would look at. It reads them from the
// repository and then *asks the runner* whether they are the history it embeds:
// a file that is on disk but not embedded is not part of this database's past,
// and auditing it as if it were would produce a report about a history that
// does not exist.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// migrationsDir is where the forward-only history lives, relative to the
// repository root. It is the same directory dbmigrate embeds.
const migrationsDir = "internal/platform/dbmigrate/migrations"

// source is one migration file, split into the two halves goose understands.
type source struct {
	Version int64
	// Path is repository-relative, so a finding can be opened.
	Path string
	// Name is the file name, which is what the runner's log prints.
	Name string
	// Up is the forward half: exactly what goose executes, comments included.
	Up string
	// Down is the rollback half. It is read only so the destructive scan can
	// say it looked at the right half; production never runs it (there is no
	// `migrate down` in the runner).
	Down string
	// Bytes is the size of the whole file, which is what the report measures.
	Bytes int
}

var (
	upMarker   = regexp.MustCompile(`(?m)^--\s*\+goose\s+Up\s*$`)
	downMarker = regexp.MustCompile(`(?m)^--\s*\+goose\s+Down\s*$`)
	versionRE  = regexp.MustCompile(`^(\d+)_.*\.sql$`)
	// createTableRE finds the tables a forward half declares. It is what makes
	// "the schema matches the history" a claim about the files as well as
	// about the catalog.
	createTableRE = regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?` + schemaName + `\.("?[\w$]+"?)`)
)

// readSources reads every migration file of the repository, in version order.
func readSources(root string) ([]source, error) {
	directory := filepath.Join(root, migrationsDir)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", migrationsDir, err)
	}

	sources := make([]source, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := versionRE.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("%s/%s does not carry a version", migrationsDir, entry.Name())
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: version is not a number: %w", migrationsDir, entry.Name(), err)
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s/%s: %w", migrationsDir, entry.Name(), err)
		}
		up, down, err := splitHalves(string(body))
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", migrationsDir, entry.Name(), err)
		}
		sources = append(sources, source{
			Version: version,
			Path:    migrationsDir + "/" + entry.Name(),
			Name:    entry.Name(),
			Up:      up,
			Down:    down,
			Bytes:   len(body),
		})
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("%s carries no migration", migrationsDir)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Version < sources[j].Version })
	for i := 1; i < len(sources); i++ {
		if sources[i].Version == sources[i-1].Version {
			return nil, fmt.Errorf("version %d appears twice (%s and %s)", sources[i].Version, sources[i-1].Name, sources[i].Name)
		}
	}
	return sources, nil
}

// splitHalves cuts the file at the two goose markers. A file without them is an
// error: guessing which half is the forward one would mean auditing statements
// the runner may never run.
func splitHalves(body string) (up, down string, err error) {
	upLocation := upMarker.FindStringIndex(body)
	if upLocation == nil {
		return "", "", fmt.Errorf("no %q marker", "-- +goose Up")
	}
	rest := body[upLocation[1]:]
	downLocation := downMarker.FindStringIndex(rest)
	if downLocation == nil {
		return rest, "", nil
	}
	return rest[:downLocation[0]], rest[downLocation[1]:], nil
}

// destructivePatterns are the statements that can remove structure or data.
// They are not forbidden: the phase asks for an audit, and a migration that
// removes something is a decision. What is forbidden is a decision nobody
// recorded, which is what the ledger checks.
//
// The DROP patterns capture the object's *name*, because the name is what tells
// a removal apart from a replacement. This history replaces its triggers and
// its CHECK constraints by dropping and recreating them under the same name in
// the same migration — a pattern the SQL needs, since PostgreSQL cannot alter
// either — and counting those as removals would bury the real ones.
var destructivePatterns = []struct {
	What    string
	Pattern *regexp.Regexp
}{
	{"DROP", regexp.MustCompile(`(?is)\bDROP\s+(?:TABLE|COLUMN|CONSTRAINT|INDEX|SCHEMA|TYPE|DOMAIN|FUNCTION|PROCEDURE|TRIGGER|VIEW|MATERIALIZED\s+VIEW|SEQUENCE|ROLE|POLICY|RULE|EXTENSION)\s+(?:IF\s+EXISTS\s+)?("[^"]*"|[\w$]+(?:\s*\.\s*(?:"[^"]*"|[\w$]+))?)`)},
	{"TRUNCATE", regexp.MustCompile(`(?is)\bTRUNCATE\b[^;]{0,40}`)},
	{"DELETE FROM", regexp.MustCompile(`(?is)\bDELETE\s+FROM\s+("[^"]*"|[\w$.]+)`)},
	{"ALTER COLUMN TYPE", regexp.MustCompile(`(?is)\bALTER\s+(?:TABLE\s+[^;\s]+\s+)?ALTER\s+COLUMN\s+("[^"]*"|[\w$]+)[^;]{0,80}?\bTYPE\b`)},
	{"ALTER COLUMN DROP DEFAULT", regexp.MustCompile(`(?is)\bALTER\s+(?:TABLE\s+[^;\s]+\s+)?ALTER\s+COLUMN\s+("[^"]*"|[\w$]+)[^;]{0,40}?\bDROP\s+DEFAULT\b`)},
	{"RENAME", regexp.MustCompile(`(?is)\bRENAME\s+(?:TO|COLUMN)\b[^;]{0,60}`)},
}

// destructive is one statement of a forward half that can remove something.
type destructive struct {
	Version int64
	Path    string
	Name    string
	What    string
	// Object is the name of what the statement touches, without its schema
	// qualifier; empty for statements that carry no name (renames).
	Object string
	// Statement is the matched text with its whitespace collapsed, which is
	// what identifies the entry of the ledger.
	Statement string
	// Key is the ledger key: version plus the normalized statement.
	Key string
	// Replaced is true when the same migration recreates this object. A
	// replacement is not a contraction: the database still has the trigger and
	// the constraint, with a different body. It is reported, because a
	// replacement is still a statement that could have been written as a
	// removal, and the ledger only has to justify the removals.
	Replaced bool
}

// scanDestructive lists every destructive statement of every forward half,
// in version order, and marks the ones that recreate what they drop. Only the
// Up halves are scanned: the Down halves of this history are full of drops, and
// production cannot run them. Comments are removed first, because these
// migrations explain themselves in prose that quotes the DDL — a `DROP` a
// comment warns about is not a statement, and reading one as such would put a
// removal in the report that never happened.
func scanDestructive(sources []source) []destructive {
	found := make([]destructive, 0)
	for _, migration := range sources {
		scanned := stripComments(migration.Up)
		for _, pattern := range destructivePatterns {
			for _, match := range pattern.Pattern.FindAllStringSubmatch(scanned, -1) {
				statement := collapse(match[0])
				object := ""
				if len(match) > 1 {
					object = unqualify(match[1])
				}
				found = append(found, destructive{
					Version:   migration.Version,
					Path:      migration.Path,
					Name:      migration.Name,
					What:      pattern.What,
					Object:    object,
					Statement: statement,
					Key:       fmt.Sprintf("%d %s", migration.Version, statement),
					Replaced:  object != "" && recreates(scanned, object),
				})
			}
		}
	}
	return found
}

// contractions are the statements that remove something for good: the ones the
// ledger has to justify.
func contractions(statements []destructive) []destructive {
	kept := make([]destructive, 0, len(statements))
	for _, statement := range statements {
		if !statement.Replaced {
			kept = append(kept, statement)
		}
	}
	return kept
}

// replacements are the drop-and-recreate statements, which the report counts
// and names but the ledger does not have to justify.
func replacements(statements []destructive) []destructive {
	kept := make([]destructive, 0, len(statements))
	for _, statement := range statements {
		if statement.Replaced {
			kept = append(kept, statement)
		}
	}
	return kept
}

// recreates answers whether a forward half creates the object it just dropped.
// The test is deliberately textual and narrow: a CREATE (or ADD CONSTRAINT /
// ADD COLUMN) within the same half that names the same object. A comment that
// mentions the name is not a creation, because the pattern requires the keyword
// in front of it.
func recreates(up, object string) bool {
	pattern := regexp.MustCompile(`(?is)\b(?:CREATE\s+(?:OR\s+REPLACE\s+)?[A-Z][A-Z\s]*?\b(?:IF\s+NOT\s+EXISTS\s+)?|ADD\s+(?:CONSTRAINT|COLUMN)\s+(?:IF\s+NOT\s+EXISTS\s+)?)` +
		regexp.QuoteMeta(object) + `\b`)
	return pattern.MatchString(up)
}

// declaredTables lists the tables the forward halves create, mapped to the
// migration that creates them.
func declaredTables(sources []source) map[string]int64 {
	declared := map[string]int64{}
	for _, migration := range sources {
		for _, match := range createTableRE.FindAllStringSubmatch(stripComments(migration.Up), -1) {
			declared[unqualify(match[1])] = migration.Version
		}
	}
	return declared
}

// commentRE matches a line comment and a block comment. SQL strings never
// carry a `--` inside them in this history, and a `'...'` that did would be a
// literal — the scan would then miss a removal, which is why the report also
// prints every statement it did find, with its migration.
var commentRE = regexp.MustCompile(`(?s)--[^\n]*|/\*.*?\*/`)

func stripComments(sql string) string {
	return commentRE.ReplaceAllString(sql, "")
}

// unqualify drops the schema and the quoting of a captured identifier, so
// `app."wallet_operations"` and `wallet_operations` are the same object.
func unqualify(identifier string) string {
	name := strings.TrimSpace(identifier)
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	return strings.Trim(strings.TrimSpace(name), `"`)
}

// collapse turns a matched statement into the one line the ledger names.
func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
