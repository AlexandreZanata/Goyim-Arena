// The rule catalogue of the release checklist (P20-T07).
//
// The phase's minimum validation is short and entirely mechanical: every command
// passes twice, the working tree is clean, the local plan is not tracked, and
// `git fsck` reports no error. This file turns each of those into a rule that
// reads the delivered document, plus the rules that keep the document honest —
// the commit it describes must exist, the dependencies must come from the
// lockfiles the commit pins, the image must be the one that was built and
// smoked, and the limitations must be declared instead of implied.
//
// One function per rule, each returning the violations it found, and the tests
// hold one mutation for each name.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// requiredCommands are the commands the phase names, in the order the document
// presents them. Hardcoded on purpose: dropping one, or adding one, has to be a
// deliberate change here.
var requiredCommands = []string{
	"go-modules",
	"web-modules",
	"e2e-modules",
	"database",
	"verify",
	"image-build",
	"image-smoke",
}

// requiredLockfiles are the lockfiles the repository pins its dependencies with.
var requiredLockfiles = []string{
	"go.sum",
	"web/package-lock.json",
	"tools/e2e/package-lock.json",
}

// minimumRuns is how many times the phase asks every command to pass.
const minimumRuns = 2

// minimumLimits is how many limitations the document has to declare. A
// checklist that claims everything and admits nothing is the document this rule
// exists to refuse.
const minimumLimits = 5

// dateFormat is the only date shape the block may use.
const dateFormat = "2006-01-02"

// commitSHA matches a full commit identifier.
var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// digestSHA256 matches a sha256 identifier, with or without its prefix.
var digestSHA256 = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)

// notFromLockfile matches an install that resolves versions at run time. The
// phase says "instalar somente lockfiles", and `npm install` is the command that
// does the opposite of that.
var notFromLockfile = regexp.MustCompile(`\bnpm\s+(install|i|add)\b`)

// emailShape matches anything that looks like an address. The checklist is a
// public document about a private system: no address of anybody belongs in it.
var emailShape = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// reservedDomains are the domains reserved for documentation and tests.
var reservedDomains = []string{
	"example.com", "example.org", "example.net", "example.edu",
	"example", "invalid", "test", "localhost",
}

// reservedDomain reports whether a domain may hold a synthetic address.
func reservedDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if contains(reservedDomains, domain) {
		return true
	}
	for _, reserved := range reservedDomains {
		if strings.HasSuffix(domain, "."+reserved) {
			return true
		}
	}
	return false
}

// Violation is one rule violation, addressed to what caused it.
type Violation struct {
	// Path is the file the violation is about. Empty means the document.
	Path string
	// Rule is the name of the rule that refused.
	Rule string
	// Subject is the command, lockfile or field the violation is about.
	Subject string
	// Detail states what was found and what was expected.
	Detail string
}

// String renders a violation the way the other auditors do.
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

// contains reports whether values holds want.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// Check judges the delivered document against the phase and against the
// repository it describes. It never writes.
func Check(root string, document Document) []Violation {
	var violations []Violation
	refuse := func(rule, subject, detail string) {
		violations = append(violations, Violation{Rule: rule, Subject: subject, Detail: detail})
	}

	facts := document.Facts

	if facts.Version != factsVersion {
		refuse("version", "", fmt.Sprintf("the block declares version %d and this tool judges version %d",
			facts.Version, factsVersion))
	}
	if _, err := time.Parse(dateFormat, facts.VerifiedOn); err != nil {
		refuse("verified-on", "", fmt.Sprintf("verified_on is %q and the document only accepts YYYY-MM-DD", facts.VerifiedOn))
	}
	if strings.TrimSpace(document.Prose) == "" {
		refuse("prose", "", "the document is the block alone: the reader has no checklist to read")
	}

	violations = append(violations, ruleCommit(root, facts)...)
	violations = append(violations, ruleHost(facts)...)
	violations = append(violations, ruleLockfiles(facts)...)
	violations = append(violations, ruleCommands(facts)...)
	violations = append(violations, ruleCheckout(root, facts)...)
	violations = append(violations, ruleRepository(facts)...)
	violations = append(violations, ruleImage(facts)...)
	violations = append(violations, ruleGovernance(facts)...)
	violations = append(violations, ruleLimits(facts)...)
	violations = append(violations, rulePII(document)...)
	return violations
}

// ruleCommit resolves the commit the document describes. The checklist is about
// a tree; if the commit does not exist, the document is about nothing.
func ruleCommit(root string, facts Facts) []Violation {
	if !commitSHA.MatchString(facts.Commit) {
		return []Violation{{
			Rule:   "commit-format",
			Detail: fmt.Sprintf("the commit is %q and the document records a full 40-character identifier", facts.Commit),
		}}
	}
	if strings.TrimSpace(facts.Branch) == "" {
		return []Violation{{Rule: "commit-branch", Detail: "the document does not record the branch the run started from"}}
	}
	command := exec.Command("git", "-C", root, "cat-file", "-e", facts.Commit+"^{commit}")
	if err := command.Run(); err != nil {
		return []Violation{{
			Rule: "commit-exists",
			Detail: fmt.Sprintf("the commit %s is not in this repository: the checklist describes a tree nobody can check out",
				facts.Commit),
		}}
	}
	return nil
}

// ruleHost pins the toolchain. Two runs of the same commit on different
// machines are different facts, and a document that does not say which machine
// it measured cannot be compared with the next one.
func ruleHost(facts Facts) []Violation {
	var violations []Violation
	refuse := func(rule, detail string) {
		violations = append(violations, Violation{Rule: rule, Detail: detail})
	}
	if !strings.HasPrefix(facts.Host.Postgres, "18") {
		refuse("host-postgres", fmt.Sprintf("the gates ran against PostgreSQL %q and the plan requires 18",
			facts.Host.Postgres))
	}
	for name, value := range map[string]string{
		"host-go": facts.Host.Go, "host-node": facts.Host.Node, "host-npm": facts.Host.NPM,
		"host-docker": facts.Host.Docker, "host-sqlc": facts.Host.Sqlc,
	} {
		if strings.TrimSpace(value) == "" {
			refuse(name, "the document records no version for this tool")
		}
	}
	if facts.Host.CPUs < 1 {
		refuse("host-cpus", "the document records a machine with no processor")
	}
	return violations
}

// ruleLockfiles judges the dependency story: the three lockfiles the repository
// pins, each with its digest and with the command that installs from it, and no
// recorded command that resolves versions at run time.
func ruleLockfiles(facts Facts) []Violation {
	var violations []Violation
	seen := map[string]bool{}
	for _, lock := range facts.Lockfiles {
		seen[lock.Path] = true
		if strings.TrimSpace(lock.Path) == "" || strings.TrimSpace(lock.Installs) == "" {
			violations = append(violations, Violation{
				Rule: "lockfile-incomplete", Subject: lock.Path,
				Detail: "the lockfile has no path or no command that installs from it",
			})
			continue
		}
		if !digestSHA256.MatchString(lock.SHA256) {
			violations = append(violations, Violation{
				Rule: "lockfile-digest", Subject: lock.Path,
				Detail: fmt.Sprintf("the digest is %q, which is not a sha256", lock.SHA256),
			})
		}
	}
	for _, required := range requiredLockfiles {
		if !seen[required] {
			violations = append(violations, Violation{
				Rule: "lockfile-missing", Subject: required,
				Detail: "the repository pins this lockfile and the document does not record it",
			})
		}
	}
	for _, command := range facts.Commands {
		if notFromLockfile.MatchString(command.Command) {
			violations = append(violations, Violation{
				Rule: "lockfile-bypassed", Subject: command.Key,
				Detail: fmt.Sprintf("%q installs outside the lockfiles, and the phase installs only from them",
					command.Command),
			})
		}
	}
	return violations
}

// ruleCommands is the phase's own minimum validation on the executions: the
// commands it names are all there, and each one passed twice.
func ruleCommands(facts Facts) []Violation {
	var violations []Violation
	seen := map[string]bool{}
	order := make([]string, 0, len(requiredCommands))
	for _, command := range facts.Commands {
		if seen[command.Key] {
			violations = append(violations, Violation{
				Rule: "command-duplicate", Subject: command.Key,
				Detail: "the same command appears twice in the document",
			})
			continue
		}
		seen[command.Key] = true
		if contains(requiredCommands, command.Key) {
			order = append(order, command.Key)
		}
	}
	for _, key := range requiredCommands {
		if !seen[key] {
			violations = append(violations, Violation{
				Rule: "command-missing", Subject: key,
				Detail: "the phase names this command and the document does not record it",
			})
		}
	}
	// The order the phase names and the order the document presents have to be
	// the same, so two checklists are read the same way. Extra commands are
	// allowed: a run may record more than the phase asks for.
	if len(order) == len(requiredCommands) {
		for index, key := range requiredCommands {
			if order[index] != key {
				violations = append(violations, Violation{
					Rule:   "command-order",
					Detail: fmt.Sprintf("the commands are published in a different order than the phase names them: %v", order),
				})
				break
			}
		}
	}

	for _, command := range facts.Commands {
		if len(command.Runs) < minimumRuns {
			violations = append(violations, Violation{
				Rule: "command-twice", Subject: command.Key,
				Detail: fmt.Sprintf("%q ran %d time(s) and the phase asks every command to pass twice",
					command.Command, len(command.Runs)),
			})
		}
		if strings.TrimSpace(command.Command) == "" {
			violations = append(violations, Violation{
				Rule: "command-empty", Subject: command.Key, Detail: "the command is not recorded",
			})
		}
		for index, run := range command.Runs {
			if run.ExitCode != 0 {
				violations = append(violations, Violation{
					Rule: "command-red", Subject: command.Key,
					Detail: fmt.Sprintf("run %d of %q answered exit %d, and every command has to pass",
						index+1, command.Command, run.ExitCode),
				})
			}
			if run.Seconds == nil || *run.Seconds < 0 {
				violations = append(violations, Violation{
					Rule: "command-timing", Subject: command.Key,
					Detail: fmt.Sprintf("run %d of %q records no duration, which is how a run that never happened is written",
						index+1, command.Command),
				})
			}
		}
	}
	return violations
}

// ruleCheckout is "working tree limpa" and "nenhum .local rastreado": the
// checkout of the commit was clean, the local plan is untracked, and after the
// document was written the repository holds nothing but the document.
func ruleCheckout(root string, facts Facts) []Violation {
	var violations []Violation
	if !facts.Checkout.Clean {
		violations = append(violations, Violation{
			Rule: "checkout-dirty", Detail: "the checkout of the commit was not clean",
		})
	}
	violations = append(violations, ruleOverlay(root, facts)...)
	if facts.Checkout.LocalTrackedFiles != 0 {
		violations = append(violations, Violation{
			Rule: "local-tracked",
			Detail: fmt.Sprintf("the commit tracks %d file(s) of the local directory, and the local plan never ships",
				facts.Checkout.LocalTrackedFiles),
		})
	}
	if facts.Checkout.Generated != documentPath {
		violations = append(violations, Violation{
			Rule:   "checkout-generated",
			Detail: fmt.Sprintf("the document names %q as the file it generated", facts.Checkout.Generated),
		})
	}
	// After the run the repository holds the document and the files the run
	// copied over the commit, and nothing else: a verification that dirties
	// the tree leaves the next one dirty.
	expected := []string{facts.Checkout.Generated}
	for _, entry := range facts.Checkout.Overlay {
		expected = append(expected, entry.Path)
	}
	if !sameSet(expected, facts.Checkout.Dirty) {
		violations = append(violations, Violation{
			Rule: "tree-after",
			Detail: fmt.Sprintf("the run left the repository holding %v, and the document accounts for %v",
				facts.Checkout.Dirty, expected),
		})
	}
	return violations
}

// ruleOverlay judges what the run copied over the commit. The tool that performs
// the verification cannot be inside the commit it verifies — it is the
// instrument, and it is committed together with the document it produces — so a
// run that has one is honest only if it says so, names every file, and stays
// inside the tooling and the evidence. A production file copied in would make
// the whole verification a statement about a tree nobody reviewed.
func ruleOverlay(root string, facts Facts) []Violation {
	var violations []Violation
	if len(facts.Checkout.Overlay) > 0 && !strings.Contains(facts.Checkout.Kind, "sobreposição") {
		violations = append(violations, Violation{
			Rule:   "overlay-undeclared",
			Detail: "the run copied files over the commit and the document does not say so where it describes the tree",
		})
	}
	for _, entry := range facts.Checkout.Overlay {
		if strings.TrimSpace(entry.Path) == "" || !digestSHA256.MatchString(entry.SHA256) {
			violations = append(violations, Violation{
				Rule: "overlay-incomplete", Subject: entry.Path,
				Detail: "the overlaid file has no path or no sha256",
			})
			continue
		}
		if !overlayAllowed(entry.Path) {
			violations = append(violations, Violation{
				Rule: "overlay-scope", Subject: entry.Path,
				Detail: "a file copied over the commit has to be the verification tooling, the evidence or the Makefile: the product code is what the commit is",
			})
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Path))
		if err != nil {
			violations = append(violations, Violation{
				Path: entry.Path, Rule: "overlay-stale", Subject: entry.Path,
				Detail: "the overlaid file is not in the tree any more",
			})
			continue
		}
		if digest := sha256Of(raw); digest != entry.SHA256 {
			violations = append(violations, Violation{
				Path: entry.Path, Rule: "overlay-stale", Subject: entry.Path,
				Detail: fmt.Sprintf("the document records the digest %s and the tree holds %s", entry.SHA256, digest),
			})
		}
	}
	return violations
}

// overlayAllowed reports whether a path may be copied over the commit: the
// tooling of this verification, the evidence it writes, and the Makefile that
// names its target.
func overlayAllowed(path string) bool {
	if path == "Makefile" {
		return true
	}
	for _, prefix := range []string{"tools/releaseverify/", "docs/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// sha256Of is the digest the overlay records.
func sha256Of(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ruleRepository is `git fsck` with no error, and a repository that is not
// empty: an empty tree would satisfy every other rule.
func ruleRepository(facts Facts) []Violation {
	var violations []Violation
	if facts.Repository.FsckErrors != 0 || !strings.HasPrefix(facts.Repository.Fsck, "ok") {
		violations = append(violations, Violation{
			Rule: "fsck",
			Detail: fmt.Sprintf("git fsck answered %q with %d error(s), and the phase requires none",
				facts.Repository.Fsck, facts.Repository.FsckErrors),
		})
	}
	if facts.Repository.TrackedFiles < 1 {
		violations = append(violations, Violation{
			Rule: "repository-empty", Detail: "the commit tracks no file at all",
		})
	}
	return violations
}

// ruleImage judges the build and the smoke: an image that was built, identified
// by a digest, and a smoke that ran against it.
func ruleImage(facts Facts) []Violation {
	var violations []Violation
	if strings.TrimSpace(facts.Image.Reference) == "" || strings.TrimSpace(facts.Image.ID) == "" {
		violations = append(violations, Violation{
			Rule: "image-unidentified", Detail: "the document names no built image",
		})
	}
	if !digestSHA256.MatchString(facts.Image.Digest) {
		violations = append(violations, Violation{
			Rule:   "image-digest",
			Detail: fmt.Sprintf("the image digest is %q, which is not a sha256", facts.Image.Digest),
		})
	}
	if strings.TrimSpace(facts.Image.Smoke) == "" || strings.TrimSpace(facts.Image.SmokeDetail) == "" {
		violations = append(violations, Violation{
			Rule: "image-smoke-absent", Detail: "the document records no smoke of the image",
		})
	}
	if !facts.Image.SmokeOK {
		violations = append(violations, Violation{
			Rule: "image-smoke-red", Detail: "the smoke of the image did not pass",
		})
	}
	if facts.Image.SizeBytes < 1 {
		violations = append(violations, Violation{
			Rule: "image-size", Detail: "the image has no size, which is how an image nobody built is written",
		})
	}
	return violations
}

// ruleGovernance keeps the two halves of the release apart. The gate is about
// decisions; the phase is about code. A document that reports a green
// verification and a red gate has to say both, and one that reports the gate as
// open while naming nothing says nothing.
func ruleGovernance(facts Facts) []Violation {
	var violations []Violation
	if strings.TrimSpace(facts.Governance.Command) == "" {
		violations = append(violations, Violation{
			Rule: "governance-command", Detail: "the document records no release gate",
		})
	}
	if facts.Governance.Blocked && len(facts.Governance.Open) == 0 {
		violations = append(violations, Violation{
			Rule:   "governance-open",
			Detail: "the release gate is recorded as refusing and the document names no open decision",
		})
	}
	for _, open := range facts.Governance.Open {
		if strings.TrimSpace(open) == "" {
			violations = append(violations, Violation{
				Rule: "governance-open", Detail: "an open decision is recorded with no name",
			})
		}
	}
	for _, pending := range facts.Governance.Pending {
		if strings.TrimSpace(pending) == "" {
			violations = append(violations, Violation{
				Rule: "governance-pending", Detail: "a pending step is recorded with no name",
			})
		}
	}
	return violations
}

// ruleLimits is the honesty rule: the document declares what the verification
// does not establish, and it declares it in the plural.
func ruleLimits(facts Facts) []Violation {
	var violations []Violation
	declared := 0
	for _, limit := range facts.Limits {
		if strings.TrimSpace(limit) == "" {
			violations = append(violations, Violation{
				Rule: "limit-empty", Detail: "a declared limitation states nothing",
			})
			continue
		}
		declared++
	}
	if declared < minimumLimits {
		violations = append(violations, Violation{
			Rule: "limits",
			Detail: fmt.Sprintf("the document declares %d limitation(s) and a release checklist that admits nothing is not a checklist",
				declared),
		})
	}
	return violations
}

// rulePII refuses an address outside the reserved domains: the checklist is
// about data, and it is not a place for anybody's data to live.
func rulePII(document Document) []Violation {
	var violations []Violation
	seen := map[string]bool{}
	for _, match := range emailShape.FindAllString(document.Text, -1) {
		if seen[match] {
			continue
		}
		seen[match] = true
		at := strings.LastIndex(match, "@")
		if !reservedDomain(match[at+1:]) {
			violations = append(violations, Violation{
				Rule:   "checklist-pii",
				Detail: fmt.Sprintf("the document carries the address %q, whose domain is not reserved", match),
			})
		}
	}
	return violations
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

// CheckFile reads the document and judges it, which is what the gate runs.
func CheckFile(root, path string) ([]Violation, error) {
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, path)
	}
	if _, err := os.Stat(resolved); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	document, err := ReadDocument(resolved)
	if err != nil {
		return nil, err
	}
	return Check(root, document), nil
}
