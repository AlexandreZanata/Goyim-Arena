// The deployment half of the backup pipeline (P19-T04): the settings the
// committed Compose file must declare for continuous WAL archiving to exist at
// all.
//
// Why this is an audit and not a comment: the archive is only as real as the
// server's configuration. A `db` service that starts without `archive_mode=on`
// is a database whose WAL is recycled unwritten, and every object in the store
// then restores to a point that is already too old — the failure that is
// discovered during the incident. The rules below read the committed file and
// refuse the drift, and they run inside `make verify`, which is what CI runs.
//
// The composed stack is not rendered here (that needs a Docker daemon, which CI
// does not have); the file is read as text, with a reader that fails closed:
// a block or a key it cannot find is a violation, never a pass. The behaviour
// half — a real server archiving real segments, and a real restore — lives in
// deploy/backup/verify.sh, and it takes the server's flags from this same
// reader so that the two cannot disagree.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Violation is one rule the committed file breaks.
type Violation struct {
	Rule    string
	Line    int
	Detail  string
	Service string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: line %d: %s", v.Rule, v.Line, v.Detail)
}

// Deployment is what the backup audit reads out of the Compose file.
type Deployment struct {
	Path string
	// Service is the block the rules are about.
	Service string
	// Command is the service's command, one argument per entry, which is how
	// the gate runs the same server the deployment runs.
	Command []string
	// Volumes are the service's mounts, in the order the file lists them.
	Volumes []string
}

const (
	// backupMount is where the scripts are mounted inside the database
	// container. It is a fixed path because the archive_command in the Compose
	// file names the script there.
	backupMount = "/opt/backup"
	// toolMount is where the backup tool is mounted, and it is deliberately
	// *outside* backupMount: a bind mount nested inside a read-only bind mount
	// fails at container start ("make mountpoint: read-only file system"), so a
	// tool placed under /opt/backup could never be mounted by a deployment that
	// mounts the scripts read-only. The scripts' own default (BACKUP_BIN) names
	// the same path.
	toolMount = "/opt/backup-tool/backupctl"
	// keyMount is where the sealing key is mounted. The key is a file and never
	// a variable: a key in the environment is a key in `docker inspect`.
	keyMount = "/run/secrets/backup_key"
	// rpoCeiling is the largest archive_timeout the audit accepts, in seconds.
	// docs/DEPLOYMENT.md §7 publishes a target RPO of fifteen minutes, and the
	// archive interval is what bounds it: an unarchived minute is an
	// unrecoverable minute.
	rpoCeiling = 900
)

// ReadDeployment reads the deployment without judging it: the gate needs the
// database's command in order to run the server the file declares, and a caller
// that only wants the flags should not have to ignore a list of violations.
func ReadDeployment(path string) (Deployment, error) {
	deployment, _, err := AuditDeployment(path)
	if err != nil {
		return Deployment{}, err
	}
	return deployment, nil
}

// AuditDeployment reads the committed Compose file and returns what it breaks.
func AuditDeployment(path string) (Deployment, []Violation, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Deployment{}, nil, fmt.Errorf("read %s: %w", path, err)
	}
	deployment := Deployment{Path: path, Service: "db"}
	lines := strings.Split(string(raw), "\n")

	block, start, found := serviceBlock(lines, deployment.Service)
	if !found {
		return deployment, []Violation{{
			Rule:   "db_declared",
			Detail: fmt.Sprintf("the file declares no %q service block, so nothing about the database's archive can be read", deployment.Service),
		}}, nil
	}
	deployment.Command = listValue(block, "command")
	deployment.Volumes = listValue(block, "volumes")
	// Line numbers are reported relative to the file, because an operator
	// reading a violation opens the file.
	commandLine := lineOf(block, "command", start)

	violations := []Violation{}
	// flag reads one `-c name=value` of the service's command.
	flag := func(name string) (string, bool) {
		prefix := name + "="
		for _, argument := range deployment.Command {
			if strings.HasPrefix(argument, prefix) {
				return strings.TrimPrefix(argument, prefix), true
			}
		}
		return "", false
	}

	// archive_mode: without it the server recycles WAL without writing it, and
	// the store silently stops receiving anything.
	switch mode, ok := flag("archive_mode"); {
	case !ok:
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  "the database's command does not set archive_mode=on: a server that does not archive recycles WAL unwritten, and a store that stops receiving segments looks exactly like a quiet database",
		})
	case mode != "on" && mode != "always":
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  fmt.Sprintf("archive_mode is %q; only on or always archive the segments this pipeline ships", mode),
		})
	}

	// wal_level: the archive carries what the level produces, and `minimal`
	// produces log records a recovery cannot replay.
	switch level, ok := flag("wal_level"); {
	case !ok:
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  "the database's command does not set wal_level=replica, so it may run at the level that logs too little to replay",
		})
	case level != "replica" && level != "logical":
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  fmt.Sprintf("wal_level is %q; replica is the floor for an archive a restore can replay", level),
		})
	}

	// archive_command: the one place the archive and the deployment meet.
	switch command, ok := flag("archive_command"); {
	case !ok:
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  "the database's command sets no archive_command, so archive_mode has nothing to call",
		})
	case !strings.Contains(command, backupMount+"/archive-wal.sh"):
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  fmt.Sprintf("archive_command is %q; it must call the script the deployment mounts at %s/archive-wal.sh, so that the archive is the reviewed pipeline and not an inline shell line", command, backupMount),
		})
	}

	// archive_timeout: the only bound on how much committed history can be lost.
	switch timeout, ok := flag("archive_timeout"); {
	case !ok:
		violations = append(violations, Violation{
			Rule:    "archive_timeout_bounded",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  fmt.Sprintf("the database's command sets no archive_timeout: a segment is only archived when it fills, which on a quiet database is hours — and the RPO target is %ds", rpoCeiling),
		})
	default:
		seconds, err := archiveTimeoutSeconds(timeout)
		if err != nil {
			violations = append(violations, Violation{
				Rule:    "archive_timeout_bounded",
				Line:    commandLine,
				Service: deployment.Service,
				Detail:  fmt.Sprintf("archive_timeout is %q, which this audit cannot read as a number of seconds", timeout),
			})
			break
		}
		if seconds > rpoCeiling {
			violations = append(violations, Violation{
				Rule:    "archive_timeout_bounded",
				Line:    commandLine,
				Service: deployment.Service,
				Detail:  fmt.Sprintf("archive_timeout is %ds and the published RPO is %ds: the interval is the bound on lost history", seconds, rpoCeiling),
			})
		}
	}

	// wal_keep_size: a server that recycles a segment the archive has not taken
	// yet makes a temporary stall into a permanent gap.
	if _, ok := flag("wal_keep_size"); !ok {
		violations = append(violations, Violation{
			Rule:    "db_archives_wal",
			Line:    commandLine,
			Service: deployment.Service,
			Detail:  "the database's command sets no wal_keep_size: an archive that stalls would have every segment it still needs recycled underneath it",
		})
	}

	// mounts: the archive_command can only run if the script and the tool are
	// inside the container's filesystem.
	if !hasMount(deployment.Volumes, backupMount) {
		violations = append(violations, Violation{
			Rule:    "backup_scripts_mounted",
			Line:    lineOf(block, "volumes", start),
			Service: deployment.Service,
			Detail:  fmt.Sprintf("the database does not mount %s, so the archive_command in its own command does not exist inside the container", backupMount),
		})
	}
	if !strings.Contains(strings.Join(deployment.Volumes, "\n"), toolMount) {
		violations = append(violations, Violation{
			Rule:    "backup_tool_mounted",
			Line:    lineOf(block, "volumes", start),
			Service: deployment.Service,
			Detail:  fmt.Sprintf("the backup tool is not mounted at %s: the scripts call it, and a script that shells out to a missing binary fails the archive without saying so to the gate", toolMount),
		})
	}
	if !hasMount(deployment.Volumes, keyMount) {
		violations = append(violations, Violation{
			Rule:    "sealing_key_is_a_file",
			Line:    lineOf(block, "volumes", start),
			Service: deployment.Service,
			Detail:  fmt.Sprintf("the sealing key is not mounted at %s: a key passed as an environment variable is a key in docker inspect, and everything sealed with it is only as private as the host", keyMount),
		})
	}
	return deployment, violations, nil
}

// archiveTimeoutSeconds reads PostgreSQL's interval syntax for the range this
// setting actually uses, and refuses to guess at anything else.
func archiveTimeoutSeconds(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	// Longest unit first: "5minutes" ends with "s" too, and a reader that
	// matched the shortest suffix would report a unit it invented.
	for _, suffix := range []struct {
		unit string
		size int
	}{{"minutes", 60}, {"minute", 60}, {"hours", 3600}, {"hour", 3600}, {"min", 60}, {"h", 3600}, {"s", 1}} {
		if !strings.HasSuffix(trimmed, suffix.unit) {
			continue
		}
		number := strings.TrimSuffix(trimmed, suffix.unit)
		seconds, err := strconv.Atoi(strings.TrimSpace(number))
		if err != nil {
			return 0, err
		}
		return seconds * suffix.size, nil
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	return seconds, nil
}

// serviceBlock returns the lines of one service, the file line where it starts,
// and whether it was found. A service starts at two-space indentation under
// `services:` and ends at the next line at or above that indentation that is not
// blank or a comment.
func serviceBlock(lines []string, service string) ([]string, int, bool) {
	header := "  " + service + ":"
	start := -1
	for index, line := range lines {
		if line == header {
			start = index
			break
		}
	}
	if start < 0 {
		return nil, 0, false
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if len(line)-len(strings.TrimLeft(line, " ")) <= 2 {
			end = index
			break
		}
	}
	return lines[start:end], start + 1, true
}

func lineOf(block []string, key string, start int) int {
	for index, line := range block {
		if strings.HasPrefix(strings.TrimSpace(line), key+":") {
			return start + index
		}
	}
	return start
}

// listValue reads a YAML list of scalars under `key:` inside a block, in either
// the block form or the inline flow form. It returns nil when the key is not
// there, which the rules treat as a violation rather than as an empty list.
func listValue(block []string, key string) []string {
	values := []string{}
	inside := false
	for _, line := range block {
		trimmed := strings.TrimSpace(line)
		if !inside {
			if trimmed == key+":" {
				inside = true
				continue
			}
			if strings.HasPrefix(trimmed, key+":") && strings.Contains(trimmed, "[") {
				inline := strings.TrimSpace(strings.TrimPrefix(trimmed, key+":"))
				inline = strings.TrimPrefix(inline, "[")
				inline = strings.TrimSuffix(inline, "]")
				for _, item := range strings.Split(inline, ",") {
					item = strings.TrimSpace(item)
					item = strings.Trim(item, "\"'")
					if item != "" {
						values = append(values, item)
					}
				}
				return values
			}
			continue
		}
		// A comment between the key and the first item is part of how this
		// file is written — every mount here carries one — and a reader that
		// stopped at it would report a mount that is plainly there.
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		item = strings.Trim(item, "\"'")
		if item != "" {
			values = append(values, item)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// hasMount reports whether one of the mounts lands on a container path,
// accepting both the short syntax and the long one.
func hasMount(volumes []string, containerPath string) bool {
	for _, volume := range volumes {
		parts := strings.Split(volume, ":")
		for _, part := range parts {
			if strings.TrimSpace(part) == containerPath {
				return true
			}
		}
	}
	return false
}
