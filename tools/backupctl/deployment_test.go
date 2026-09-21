package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// committedCompose is the file the audit exists for. The tests mutate the real
// file rather than a fixture: a rule that only holds for a fixture holds for
// nothing, and this file is the one that gets deployed.
const committedCompose = "../../compose.production.yaml"

func composeSource(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(committedCompose)
	if err != nil {
		t.Fatalf("read the committed Compose file: %v", err)
	}
	return string(raw)
}

func writeCompose(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "compose.production.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write the mutated Compose file: %v", err)
	}
	return path
}

// replaceLineOnce rewrites the one line whose content is exactly old, and fails
// when it is not there or is there more than once. The failure mode matters: a
// mutation that never applied would be reported as "the rule did not fire",
// which is the wrong conclusion about the gate.
func replaceLineOnce(t *testing.T, content, old, replacement string) string {
	t.Helper()

	lines := strings.Split(content, "\n")
	matches := 0
	for index, line := range lines {
		if line != old {
			continue
		}
		matches++
		lines[index] = replacement
	}
	if matches != 1 {
		t.Fatalf("the mutation is ambiguous: %d line(s) equal %q", matches, old)
	}
	return strings.Join(lines, "\n")
}

func removeLineOnce(t *testing.T, content, old string) string {
	t.Helper()

	return replaceLineOnce(t, content, old, "")
}

// TestTheCommittedDeploymentArchives is the baseline the mutations are measured
// against: the file this repository commits breaks no rule.
func TestTheCommittedDeploymentArchives(t *testing.T) {
	t.Parallel()

	deployment, violations, err := AuditDeployment(committedCompose)
	if err != nil {
		t.Fatalf("AuditDeployment() error = %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("the committed deployment breaks %d rule(s): %v", len(violations), violations)
	}
	if deployment.Service != "db" {
		t.Fatalf("the audit read service %q, want db", deployment.Service)
	}
	// The command is read, not guessed: the gate starts its server from it.
	joined := strings.Join(deployment.Command, " ")
	for _, required := range []string{"postgres", "archive_mode=on", "wal_level=replica", "archive_timeout=300", "wal_keep_size="} {
		if !strings.Contains(joined, required) {
			t.Fatalf("the deployment's command is %v, and it does not carry %q", deployment.Command, required)
		}
	}
	if len(deployment.Volumes) < 4 {
		t.Fatalf("the audit read %d mount(s), want the data volume and the three the archive needs: %v", len(deployment.Volumes), deployment.Volumes)
	}
}

// TestEachArchiveRuleRefusesItsOwnMutation runs one mutation per rule. Each
// mutation breaks that rule and no other, so a rule that fired for the wrong
// reason is caught.
func TestEachArchiveRuleRefusesItsOwnMutation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		rule   string
		mutate func(t *testing.T, content string) string
	}{
		{
			name: "a database that does not archive at all",
			rule: "db_archives_wal",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - archive_mode=on")
			},
		},
		{
			name: "an archive mode this audit does not recognise",
			rule: "db_archives_wal",
			mutate: func(t *testing.T, content string) string {
				return replaceLineOnce(t, content, "      - archive_mode=on", "      - archive_mode=off")
			},
		},
		{
			name: "a write-ahead level that logs too little to replay",
			rule: "db_archives_wal",
			mutate: func(t *testing.T, content string) string {
				return replaceLineOnce(t, content, "      - wal_level=replica", "      - wal_level=minimal")
			},
		},
		{
			name: "an archive command that is not the reviewed script",
			rule: "db_archives_wal",
			mutate: func(t *testing.T, content string) string {
				return replaceLineOnce(t, content, "      - archive_command=/opt/backup/archive-wal.sh %p %f", "      - archive_command=/bin/true")
			},
		},
		{
			name: "no archive command at all",
			rule: "db_archives_wal",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - archive_command=/opt/backup/archive-wal.sh %p %f")
			},
		},
		{
			name: "a segment the server would recycle in a stall",
			rule: "db_archives_wal",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - wal_keep_size=512MB")
			},
		},
		{
			name: "an archive interval that exceeds the published RPO",
			rule: "archive_timeout_bounded",
			mutate: func(t *testing.T, content string) string {
				return replaceLineOnce(t, content, "      - archive_timeout=300", "      - archive_timeout=3600")
			},
		},
		{
			name: "an archive interval in a unit that hides the RPO",
			rule: "archive_timeout_bounded",
			mutate: func(t *testing.T, content string) string {
				return replaceLineOnce(t, content, "      - archive_timeout=300", "      - archive_timeout=1h")
			},
		},
		{
			name: "no archive interval at all",
			rule: "archive_timeout_bounded",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - archive_timeout=300")
			},
		},
		{
			name: "an archive command the container cannot reach",
			rule: "backup_scripts_mounted",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - \"${COMPOSE_BACKUP_SCRIPTS_DIR:-./deploy/backup}:/opt/backup:ro\"")
			},
		},
		{
			name: "an archive command whose tool is missing",
			rule: "backup_tool_mounted",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - \"${COMPOSE_BACKUP_TOOL:-./bin/backupctl}:/opt/backup-tool/backupctl:ro\"")
			},
		},
		{
			name: "a sealing key that is not mounted",
			rule: "sealing_key_is_a_file",
			mutate: func(t *testing.T, content string) string {
				return removeLineOnce(t, content, "      - \"${COMPOSE_BACKUP_KEY_FILE:-./secrets/backup.key}:/run/secrets/backup_key:ro\"")
			},
		},
		{
			name: "a file that declares no database at all",
			rule: "db_declared",
			mutate: func(t *testing.T, content string) string {
				lines := strings.Split(content, "\n")
				start := -1
				for index, line := range lines {
					if line == "  db:" {
						start = index
						break
					}
				}
				if start < 0 {
					t.Fatal("the fixture has no db block to remove")
				}
				end := len(lines)
				for index := start + 1; index < len(lines); index++ {
					if strings.TrimSpace(lines[index]) == "" {
						continue
					}
					if len(lines[index])-len(strings.TrimLeft(lines[index], " ")) <= 2 {
						end = index
						break
					}
				}
				return strings.Join(append(lines[:start:start], lines[end:]...), "\n")
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeCompose(t, testCase.mutate(t, composeSource(t)))
			_, violations, err := AuditDeployment(path)
			if err != nil {
				t.Fatalf("AuditDeployment() error = %v", err)
			}
			found := false
			for _, violation := range violations {
				if violation.Rule == testCase.rule {
					found = true
				}
			}
			if !found {
				t.Fatalf("the mutation broke no %q rule: %v", testCase.rule, violations)
			}
			if len(violations) != 1 {
				t.Fatalf("the mutation breaks %d rule(s), and the probe attributes one: %v", len(violations), violations)
			}
		})
	}
}

// TestArchiveIntervalUnitsAreReadNotAssumed covers the interval syntax: the RPO
// bound is only a bound if every spelling of "an hour" is read as an hour.
func TestArchiveIntervalUnitsAreReadNotAssumed(t *testing.T) {
	t.Parallel()

	cases := map[string]int{
		"300":      300,
		"300s":     300,
		"5min":     300,
		"5minutes": 300,
		"15min":    900,
		"1h":       3600,
		"2hours":   7200,
	}
	for value, want := range cases {
		got, err := archiveTimeoutSeconds(value)
		if err != nil {
			t.Fatalf("archiveTimeoutSeconds(%q) error = %v", value, err)
		}
		if got != want {
			t.Fatalf("archiveTimeoutSeconds(%q) = %d, want %d", value, got, want)
		}
	}
	for _, bad := range []string{"", "soon", "5 fortnights"} {
		if _, err := archiveTimeoutSeconds(bad); err == nil {
			t.Fatalf("archiveTimeoutSeconds(%q) accepted something that is not an interval", bad)
		}
	}
}

// TestAuditRefusesAFileItCannotRead is the fail-closed half: a file the audit
// cannot read is an error, and a file without the block is a violation, never a
// pass.
func TestAuditRefusesAFileItCannotRead(t *testing.T) {
	t.Parallel()

	if _, _, err := AuditDeployment(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("AuditDeployment() accepted a file that does not exist")
	}

	_, violations, err := AuditDeployment(writeCompose(t, "services:\n  app:\n    image: x\n"))
	if err != nil {
		t.Fatalf("AuditDeployment() error = %v", err)
	}
	if len(violations) != 1 || violations[0].Rule != "db_declared" {
		t.Fatalf("violations = %v, want the one that says the database is not declared", violations)
	}
}
