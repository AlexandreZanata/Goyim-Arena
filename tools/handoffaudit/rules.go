// The rule catalogue of the README handoff (P20-T08).
//
// The phase's minimum validation is a person or an agent following the README
// in a clean checkout and executing the smoke without tacit knowledge. That
// cannot be asserted: it has to be made falsifiable, and this file is the half
// of it that reads the document. Each rule is a claim the README makes that the
// tree can contradict — a command that does not exist in the Makefile, a
// variable that is not in the environment template, a path that is not there, a
// subcommand the binary does not have — plus the declarations the walkthrough
// needs to run the document instead of a paraphrase of it.
//
// The other half is `handoffaudit walkthrough`, which checks out the commit,
// follows the commands the document itself declares and smokes the surfaces it
// names.
package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// violation is one claim the document makes that the tree refuses.
type violation struct {
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}

// rule is one question about the document.
type rule struct {
	name  string
	check func(*facts) []violation
}

// catalogue is every rule, in the order the report presents them.
var catalogue = []rule{
	{name: "section-missing", check: ruleSections},
	{name: "command-unknown", check: ruleCommands},
	{name: "quickstart-markers", check: ruleQuickstartMarkers},
	{name: "serve-markers", check: ruleServeMarkers},
	{name: "quickstart-command-missing", check: ruleQuickstartCommands},
	{name: "quickstart-export-missing", check: ruleQuickstartExports},
	{name: "serve-command-missing", check: ruleServeCommand},
	{name: "subcommand-unknown", check: ruleSubcommands},
	{name: "subcommand-missing", check: ruleSubcommandsMissing},
	{name: "env-unknown", check: ruleEnvUnknown},
	{name: "env-missing", check: ruleEnvMissing},
	{name: "path-missing", check: rulePaths},
	{name: "evidence-missing", check: ruleEvidence},
	{name: "surface-missing", check: ruleSurfaces},
	{name: "address-not-reserved", check: ruleAddresses},
	{name: "secret-shaped", check: ruleSecrets},
}

// requiredSections are the topics the phase names, one level-two heading each.
var requiredSections = []string{
	"Índice",
	"Arquitetura",
	"Configuração",
	"Desenvolvimento",
	"Teste",
	"Operação",
}

// requiredQuickstartCommands are the commands a person runs in a clean
// checkout, in the order the document presents them. Hardcoded on purpose:
// dropping one, or adding one, has to be a deliberate change here — the
// walkthrough executes exactly what this block holds.
var requiredQuickstartCommands = []string{
	"docker compose up -d db",
	"go run ./cmd/arena migrate up",
	"make test-unit",
	"make build-web",
	"make image-verify",
}

// requiredQuickstartExports are the variables the walkthrough cannot run
// without, so a quickstart that assumes them is a quickstart with tacit
// knowledge.
var requiredQuickstartExports = []string{
	"ARENA_DATABASE_URL",
	"ARENA_CURSOR_SECRET",
}

// requiredSubcommands are the subcommands an operator needs to know about: the
// two processes, the migrations, the administrative act and the projections.
var requiredSubcommands = []string{"server", "worker", "migrate", "admin", "projections"}

// requiredEnvVars are the variables a developer sets by hand. The complete
// matrix lives in the environment template the rule below resolves against.
var requiredEnvVars = []string{
	"ARENA_ENV",
	"ARENA_ADDR",
	"ARENA_DATABASE_URL",
	"ARENA_CURSOR_SECRET",
	"ARENA_ASSETS_DIR",
	"ARENA_LOG_LEVEL",
	"ARENA_ADMIN_ADDR",
}

// requiredEvidence are the documents a handoff points at: what was measured
// before publishing, the decisions that still block the beta, and the pipeline
// the gates run in. A handoff that does not cite its own evidence is the
// handoff these rules exist to refuse.
var requiredEvidence = []string{
	"docs/RELEASE_CHECKLIST.md",
	"docs/GOVERNANCE.md",
	"docs/CI.md",
}

// requiredSurfaces are the paths the walkthrough smokes, so the document has to
// name them.
var requiredSurfaces = []string{"/health/live", "/login"}

// minimumQuickstartLines is how many lines the declared walkthrough needs to be
// a walkthrough.
const minimumQuickstartLines = 5

func ruleSections(f *facts) []violation {
	var violations []violation
	for _, section := range requiredSections {
		if !f.doc.hasHeading(section) {
			violations = append(violations, violation{
				Rule:   "section-missing",
				Detail: fmt.Sprintf("the phase names %q and the document has no `## %s` heading", section, section),
			})
		}
	}
	return violations
}

func ruleCommands(f *facts) []violation {
	var violations []violation
	for _, name := range mentionedMakeTargets(f) {
		if !f.makeTarget[name] {
			violations = append(violations, violation{
				Rule:   "command-unknown",
				Detail: fmt.Sprintf("`make %s` does not exist in the Makefile: a command the reader cannot run", name),
			})
		}
	}
	return violations
}

// mentionedMakeTargets lists the targets the document tells the reader to run,
// in sorted order so the report is deterministic.
func mentionedMakeTargets(f *facts) []string {
	seen := map[string]bool{}
	for _, line := range f.doc.code() {
		for _, match := range makeCommandPattern.FindAllStringSubmatch(line, -1) {
			seen[match[1]] = true
		}
	}
	return sortedKeys(seen)
}

func ruleQuickstartMarkers(f *facts) []violation {
	var violations []violation
	count := f.doc.count("quickstart")
	if count != 1 {
		violations = append(violations, violation{
			Rule:   "quickstart-markers",
			Detail: fmt.Sprintf("the document declares %d quickstart block(s); it must declare exactly one, between `%s` and `%s`", count, quickstartBegin, quickstartEnd),
		})
		return violations
	}
	block := f.doc.block("quickstart")
	if len(block.lines) < minimumQuickstartLines {
		violations = append(violations, violation{
			Rule:   "quickstart-markers",
			Detail: fmt.Sprintf("the quickstart holds %d line(s); a walkthrough needs at least %d", len(block.lines), minimumQuickstartLines),
		})
	}
	return violations
}

func ruleServeMarkers(f *facts) []violation {
	if f.doc.count("serve") == 1 {
		return nil
	}
	return []violation{{
		Rule:   "serve-markers",
		Detail: fmt.Sprintf("the document declares %d serve block(s); the smoke needs exactly one, between `%s` and `%s`", f.doc.count("serve"), serveBegin, serveEnd),
	}}
}

func ruleQuickstartCommands(f *facts) []violation {
	return requiredInBlock(f, "quickstart", requiredQuickstartCommands, "quickstart-command-missing",
		"the walkthrough the phase names does not run `%s`: what a person pastes has to be the verification")
}

func ruleQuickstartExports(f *facts) []violation {
	required := make([]string, 0, len(requiredQuickstartExports))
	for _, name := range requiredQuickstartExports {
		required = append(required, "export "+name+"=")
	}
	return requiredInBlock(f, "quickstart", required, "quickstart-export-missing",
		"the quickstart does not set `%s`: a walkthrough that assumes a variable is one that needs tacit knowledge")
}

func ruleServeCommand(f *facts) []violation {
	return requiredInBlock(f, "serve", []string{"./cmd/arena server"}, "serve-command-missing",
		"the serve block does not start the server with `%s`")
}

// requiredInBlock refuses every required fragment a declared block does not
// hold. A block that is missing entirely is already the markers rule's finding,
// and reporting it once is enough.
func requiredInBlock(f *facts, kind string, required []string, name, detail string) []violation {
	block := f.doc.block(kind)
	if block == nil {
		return nil
	}
	body := strings.Join(block.lines, "\n")
	var violations []violation
	for _, fragment := range required {
		if !strings.Contains(body, fragment) {
			violations = append(violations, violation{
				Rule:   name,
				Detail: fmt.Sprintf(detail, fragment),
			})
		}
	}
	return violations
}

func ruleSubcommands(f *facts) []violation {
	var violations []violation
	for _, name := range mentionedSubcommands(f) {
		if !f.subcommand[name] {
			violations = append(violations, violation{
				Rule:   "subcommand-unknown",
				Detail: fmt.Sprintf("`arena %s` is not one of the binary's subcommands", name),
			})
		}
	}
	return violations
}

func ruleSubcommandsMissing(f *facts) []violation {
	mentioned := map[string]bool{}
	for _, name := range mentionedSubcommands(f) {
		mentioned[name] = true
	}
	var violations []violation
	for _, name := range requiredSubcommands {
		if !mentioned[name] {
			violations = append(violations, violation{
				Rule:   "subcommand-missing",
				Detail: fmt.Sprintf("the document never names `arena %s`, which the binary has and an operator needs", name),
			})
		}
	}
	return violations
}

// mentionedSubcommands lists the subcommands the document names, read from the
// code only: the prose may talk about the binary without prescribing a command.
func mentionedSubcommands(f *facts) []string {
	seen := map[string]bool{}
	for _, line := range f.doc.code() {
		for _, match := range arenaCommandRegexp.FindAllStringSubmatch(line, -1) {
			seen[match[1]] = true
		}
	}
	return sortedKeys(seen)
}

func ruleEnvUnknown(f *facts) []violation {
	var violations []violation
	seen := map[string]bool{}
	for _, name := range mentionedEnvNames(f.doc.text) {
		if seen[name] {
			continue
		}
		seen[name] = true
		if prefix, isPrefix := envPrefix(name); isPrefix {
			if hasVariableWithPrefix(f.envVars, prefix) {
				continue
			}
			violations = append(violations, violation{
				Rule:   "env-unknown",
				Detail: fmt.Sprintf("`%s` names a family of variables and .env.example has none of them", name),
			})
			continue
		}
		if !f.envVars[name] {
			violations = append(violations, violation{
				Rule:   "env-unknown",
				Detail: fmt.Sprintf("`%s` is not in .env.example: the configuration the document explains is not the one the process reads", name),
			})
		}
	}
	return violations
}

// envPrefix reports whether a name written in the document declares a family —
// the prefix itself (`ARENA_`) or a wildcard (`ARENA_DB_*`) — instead of one
// variable.
func envPrefix(name string) (string, bool) {
	if strings.HasSuffix(name, "*") {
		return strings.TrimSuffix(name, "*"), true
	}
	if strings.HasSuffix(name, "_") {
		return name, true
	}
	return "", false
}

// hasVariableWithPrefix reports whether the environment template holds any
// variable of a family.
func hasVariableWithPrefix(variables map[string]bool, prefix string) bool {
	for name := range variables {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// mentionedEnvNames lists every ARENA_* name the document writes. A name that
// ends in a wildcard — `ARENA_DB_*` — is a family: it is honest when the
// environment template holds a variable of that family, and the family is what
// the reader is pointed at.
func mentionedEnvNames(text string) []string {
	names := []string{}
	for _, match := range envNameRegexp.FindAllStringIndex(text, -1) {
		name := text[match[0]:match[1]]
		if match[1] < len(text) && text[match[1]] == '*' {
			name += "*"
		}
		names = append(names, name)
	}
	return names
}

func ruleEnvMissing(f *facts) []violation {
	var violations []violation
	for _, name := range requiredEnvVars {
		if !strings.Contains(f.doc.text, name) {
			violations = append(violations, violation{
				Rule:   "env-missing",
				Detail: fmt.Sprintf("the document never names `%s`, which a developer has to set", name),
			})
		}
	}
	return violations
}

func rulePaths(f *facts) []violation {
	var violations []violation
	seen := map[string]bool{}
	for _, candidate := range mentionedPaths(f) {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if f.resolvable(candidate) {
			continue
		}
		violations = append(violations, violation{
			Rule:   "path-missing",
			Detail: fmt.Sprintf("`%s` is in no commit, in no working tree and in nothing the tree ignores: a path the reader cannot open", candidate),
		})
	}
	return violations
}

// mentionedPaths lists the repository paths the document names in code or as a
// link target. A path that this checkout does not hold is still a path of the
// repository when a commit holds it, and it is fine when the tree ignores it —
// `web/dist` is a build output and `.env` is local configuration. It is a
// violation only when it names nothing at all.
func mentionedPaths(f *facts) []string {
	paths := []string{}
	for _, line := range f.doc.code() {
		for _, span := range inlineSpans(line) {
			if candidate := cleanPath(span); f.looksLikePath(candidate) {
				paths = append(paths, candidate)
			}
		}
		if candidate := cleanPath(strings.TrimSpace(line)); f.looksLikePath(candidate) {
			paths = append(paths, candidate)
		}
	}
	for _, target := range f.doc.linkTargets() {
		if candidate := cleanPath(target); f.looksLikePath(candidate) {
			paths = append(paths, candidate)
		}
	}
	sort.Strings(paths)
	return paths
}

// cleanPath trims the punctuation a sentence puts around a path.
func cleanPath(token string) string {
	return strings.Trim(token, " \t`.,;:()[]{}\"'")
}

func ruleEvidence(f *facts) []violation {
	var violations []violation
	for _, path := range requiredEvidence {
		if !strings.Contains(f.doc.text, path) {
			violations = append(violations, violation{
				Rule:   "evidence-missing",
				Detail: fmt.Sprintf("the document does not cite `%s`: a handoff without its evidence is a claim about nothing", path),
			})
		}
	}
	return violations
}

func ruleSurfaces(f *facts) []violation {
	var violations []violation
	for _, surface := range requiredSurfaces {
		if !strings.Contains(f.doc.text, surface) {
			violations = append(violations, violation{
				Rule:   "surface-missing",
				Detail: fmt.Sprintf("the document does not name `%s`, which the walkthrough smokes", surface),
			})
		}
	}
	return violations
}

// reservedZones are the domains RFC 2606 reserves for documentation. An address
// outside them is somebody's real mailbox, and a handoff is not the place for
// one.
var reservedZones = []string{"example.com", "example.org", "example.net", ".invalid", ".test", ".localhost"}

func ruleAddresses(f *facts) []violation {
	var violations []violation
	seen := map[string]bool{}
	for _, address := range addressPattern.FindAllString(f.doc.text, -1) {
		if seen[address] {
			continue
		}
		seen[address] = true
		reserved := false
		for _, zone := range reservedZones {
			if strings.HasSuffix(strings.ToLower(address), zone) || strings.Contains(strings.ToLower(address), "@"+strings.TrimPrefix(zone, ".")+".") {
				reserved = true
				break
			}
		}
		if !reserved {
			violations = append(violations, violation{
				Rule:   "address-not-reserved",
				Detail: fmt.Sprintf("`%s` is not in a domain RFC 2606 reserves: real addresses do not ship", address),
			})
		}
	}
	return violations
}

// secretShapes are the literal shapes of provider credentials. A document that
// shows one is a document that leaks one.
var secretShapes = []*regexp.Regexp{
	regexp.MustCompile(`\bsk_live_[A-Za-z0-9]{10,}`),
	regexp.MustCompile(`\bsk_test_[A-Za-z0-9]{10,}`),
	regexp.MustCompile(`\brk_live_[A-Za-z0-9]{10,}`),
	regexp.MustCompile(`\bre_[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`\bwhsec_[A-Za-z0-9]{10,}`),
	regexp.MustCompile(`\bphc_[A-Za-z0-9]{10,}`),
	regexp.MustCompile(`\b[0-9a-f]{40,}\b`),
}

func ruleSecrets(f *facts) []violation {
	var violations []violation
	for _, shape := range secretShapes {
		for _, match := range shape.FindAllString(f.doc.text, -1) {
			violations = append(violations, violation{
				Rule:   "secret-shaped",
				Detail: fmt.Sprintf("the document carries `%s…`, which has the shape of a provider credential", truncate(match, 8)),
			})
		}
	}
	return violations
}

var (
	addressPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

func truncate(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[:length]
}

// audit runs every rule and returns what they found, in catalogue order.
func audit(f *facts) []violation {
	violations := []violation{}
	for _, candidate := range catalogue {
		violations = append(violations, candidate.check(f)...)
	}
	return violations
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
