// The README as the rules read it (P20-T08).
//
// The document is parsed once into the shapes the rules need — the headings,
// the fenced blocks, the walkthrough the document declares about itself — so a
// rule is a question about a parsed document instead of a regexp over a file.
// Nothing here writes: the README is delivered by a person, and the tool only
// refuses it.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Marker comments are how the document declares its own walkthrough: what a
// person copies and runs, and what a process starts to smoke the surfaces. They
// are comments so they disappear from the rendered page.
const (
	quickstartBegin = "<!-- quickstart:begin -->"
	quickstartEnd   = "<!-- quickstart:end -->"
	serveBegin      = "<!-- serve:begin -->"
	serveEnd        = "<!-- serve:end -->"
)

// block is a fenced block attributed to a marker.
type block struct {
	kind  string // "quickstart", "serve" or ""
	lines []string
	start int
}

// document is the parsed README.
type document struct {
	path     string
	text     string
	lines    []string
	headings []string
	blocks   []block
}

// loadDocument reads and parses the document.
func loadDocument(path string) (*document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("handoffaudit: read %s: %w", path, err)
	}
	return parseDocument(path, string(raw)), nil
}

// parseDocument parses the document text, which is what lets a test hold a
// mutation of the delivered document without writing a file.
func parseDocument(path, text string) *document {
	doc := &document{path: path, text: text, lines: strings.Split(text, "\n")}

	marker := ""
	inFence := false
	current := block{}
	for index, line := range doc.lines {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case quickstartBegin:
			marker = "quickstart"
			continue
		case serveBegin:
			marker = "serve"
			continue
		case quickstartEnd, serveEnd:
			marker = ""
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			doc.headings = append(doc.headings, strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")))
		}
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				inFence = false
				current.kind = marker
				doc.blocks = append(doc.blocks, current)
				current = block{}
			} else {
				inFence = true
				current = block{start: index + 1}
			}
			continue
		}
		if inFence {
			current.lines = append(current.lines, line)
		}
	}
	return doc
}

// block returns the single block of a kind, or nil.
func (doc *document) block(kind string) *block {
	for index := range doc.blocks {
		if doc.blocks[index].kind == kind {
			return &doc.blocks[index]
		}
	}
	return nil
}

// count returns how many blocks carry a kind.
func (doc *document) count(kind string) int {
	total := 0
	for _, candidate := range doc.blocks {
		if candidate.kind == kind {
			total++
		}
	}
	return total
}

// hasHeading reports whether the document carries a level-two heading with that
// exact title.
func (doc *document) hasHeading(title string) bool {
	for _, heading := range doc.headings {
		if heading == title {
			return true
		}
	}
	return false
}

// code returns every line that the reader sees as code: the fenced blocks and
// the inline spans. A rule that asks "which commands does this document tell a
// person to run" has to read exactly those, and not the prose that discusses
// them.
func (doc *document) code() []string {
	lines := []string{}
	inFence := false
	for _, line := range doc.lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, inlineSpans(line)...)
	}
	return lines
}

// inlineSpans returns the contents of the `backticked` spans of a line.
func inlineSpans(line string) []string {
	spans := []string{}
	for {
		start := strings.Index(line, "`")
		if start < 0 {
			return spans
		}
		line = line[start+1:]
		end := strings.Index(line, "`")
		if end < 0 {
			return spans
		}
		spans = append(spans, line[:end])
		line = line[end+1:]
	}
}

// linkTargets returns the local targets of the markdown links of the document.
func (doc *document) linkTargets() []string {
	targets := []string{}
	for _, match := range linkPattern.FindAllStringSubmatch(doc.text, -1) {
		target := strings.TrimSpace(match[1])
		if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

var linkPattern = regexp.MustCompile(`\]\(([^)]+)\)`)

// facts is everything the rules read besides the document: the two files the
// document's claims are judged against, and the tree.
type facts struct {
	root       string
	doc        *document
	makeTarget map[string]bool
	envVars    map[string]bool
	subcommand map[string]bool
	topLevel   []string
}

// loadFacts reads the document and the files that judge it.
func loadFacts(root, path string) (*facts, error) {
	doc, err := loadDocument(path)
	if err != nil {
		return nil, err
	}
	return newFacts(root, doc)
}

// newFacts reads the files that judge a document: the Makefile whose targets
// the commands must name, the environment template whose variables the
// configuration may name, the binary's own usage, and the root of the tree.
func newFacts(root string, doc *document) (*facts, error) {
	loaded := &facts{
		root:       root,
		doc:        doc,
		makeTarget: map[string]bool{},
		envVars:    map[string]bool{},
		subcommand: map[string]bool{},
	}

	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return nil, fmt.Errorf("handoffaudit: read the Makefile: %w", err)
	}
	for _, match := range makeTargetPattern.FindAllStringSubmatch(string(makefile), -1) {
		loaded.makeTarget[match[1]] = true
	}
	for _, match := range makePhonyPattern.FindAllStringSubmatch(string(makefile), -1) {
		for _, name := range strings.Fields(match[1]) {
			loaded.makeTarget[name] = true
		}
	}

	envExample, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil {
		return nil, fmt.Errorf("handoffaudit: read .env.example: %w", err)
	}
	for _, name := range envVarPattern.FindAllString(string(envExample), -1) {
		loaded.envVars[name] = true
	}

	usage, err := os.ReadFile(filepath.Join(root, "cmd", "arena", "main.go"))
	if err != nil {
		return nil, fmt.Errorf("handoffaudit: read the command's usage: %w", err)
	}
	for _, name := range usageSubcommands(string(usage)) {
		loaded.subcommand[name] = true
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("handoffaudit: read the tree: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" || name == ".local" {
			continue
		}
		loaded.topLevel = append(loaded.topLevel, name)
	}
	sort.Strings(loaded.topLevel)
	return loaded, nil
}

var (
	makeTargetPattern  = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*):`)
	makePhonyPattern   = regexp.MustCompile(`(?m)^\.PHONY: *(.+)$`)
	envVarPattern      = regexp.MustCompile(`\bARENA_[A-Z0-9_]+`)
	makeCommandPattern = regexp.MustCompile(`\bmake +([a-z][a-z0-9-]*)`)
	arenaCommandRegexp = regexp.MustCompile(`\barena +([a-z][a-z-]*)\b`)
	envNameRegexp      = regexp.MustCompile(`\bARENA_[A-Z0-9_]*`)
)

// usageSubcommands reads the command list out of the binary's own help text:
// the lines between "The commands are:" and the blank line that follows them.
// The document may name exactly the commands that exist, and this is where the
// list of them lives.
func usageSubcommands(source string) []string {
	lines := strings.Split(source, "\n")
	start := -1
	for index, line := range lines {
		if strings.Contains(line, "The commands are:") {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return nil
	}
	names := []string{}
	for _, line := range lines[start:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			if len(names) > 0 {
				break
			}
			continue
		}
		if len(fields) < 2 || !strings.HasPrefix(line, "  ") {
			break
		}
		names = append(names, fields[0])
	}
	return names
}

// exists reports whether a path exists under the root.
func (f *facts) exists(path string) bool {
	_, err := os.Stat(filepath.Join(f.root, filepath.Clean(path)))
	return err == nil
}

// tracked reports whether the commit holds a path, which is what makes a path a
// file of the repository even when this particular checkout does not have it on
// disk: a verification run that produces an artifact removes the previous one
// from its own checkout, and the document that cites the artifact is right about
// the repository all the same.
func (f *facts) tracked(path string) bool {
	command := exec.Command("git", "ls-files", "--", path)
	command.Dir = f.root
	out, err := command.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// resolvable reports whether a path the document cites names something a reader
// can open: a file of the working tree, a file of the commit, or a path the tree
// ignores (a build output, local configuration).
func (f *facts) resolvable(path string) bool {
	return f.exists(path) || f.tracked(path) || f.ignored(path)
}

// ignored reports whether the tree ignores a path, which is how the document
// may name a build output (`web/dist`) or a local file (`.env`) that a fresh
// checkout does not hold.
//
// Both forms are probed, and the second one is not a detail: a pattern that ends
// in a slash — `/web/dist/` — matches a directory, so in a clean checkout, where
// the directory does not exist yet, git answers "not ignored" for the bare path
// and "ignored" for the path a reader would open. A rule that only asked the
// first question would refuse the document in exactly the checkout the phase's
// validation is about.
func (f *facts) ignored(path string) bool {
	return f.gitIgnores(path) || f.gitIgnores(strings.TrimSuffix(path, "/")+"/")
}

func (f *facts) gitIgnores(path string) bool {
	command := exec.Command("git", "check-ignore", "-q", "--", path)
	command.Dir = f.root
	return command.Run() == nil
}

// looksLikePath reports whether a token is a path of this repository: it starts
// with one of the root's entries and holds no wildcard, placeholder or
// whitespace. It is the guard that keeps the rule from judging prose.
func (f *facts) looksLikePath(token string) bool {
	if token == "" || strings.ContainsAny(token, "*{}<>| \t") {
		return false
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "http") {
		return false
	}
	for _, entry := range f.topLevel {
		if token == entry || strings.HasPrefix(token, entry+"/") {
			return true
		}
	}
	return false
}
