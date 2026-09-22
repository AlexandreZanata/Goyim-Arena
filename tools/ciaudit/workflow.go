// Reading of the delivered CI workflows (P19-T08).
//
// The gate is `make audit-ci`, which runs inside `make verify`, and it answers
// one question: can a change reach `main` while a gate the phase requires was
// never run? Every rule below closes one way of answering "no" falsely:
//
//   - every required gate is wired to the CI (gates-wired), and every target a
//     workflow invokes exists in the Makefile (make-targets-exist), so the
//     workflow cannot drift from the file it calls;
//   - a third-party action is a reviewed commit, not a moving tag
//     (actions-pinned);
//   - the token is read-only (permissions-minimal) and no step reads a secret
//     other than the one GitHub issues (trigger-and-secret-surface);
//   - no step can swallow its own failure (failure-never-masked);
//   - a job that runs a gate needing a database declares one
//     (database-service);
//   - the complete verification skips draft pull requests and never skips
//     anything else (draft-skip-without-reduction), inside the phase's time
//     budget (job-budget).
//
// It reads the files as they are and never writes: the fix belongs to the
// author, in the commit that changed the workflow. A gate that edited its own
// input would be auditing itself.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// entry is one scalar of a workflow document, addressed by its YAML path. The
// path is the whole contract between the reader and the rules: `jobs.browser.env.
// ARENA_DATABASE_URL`, `on.pull_request.types[3]`, `jobs.image.steps[2].uses`.
type entry struct {
	Path    string
	Key     string
	Value   string
	Comment string // the trailing comment, without the `#`
	Line    int
}

// workflow is one delivered file, with every scalar it declares.
type workflow struct {
	Path    string
	entries []entry
}

// loadWorkflows reads every workflow the repository delivers, in a stable
// order. A file that cannot be read or parsed is an error, never a skip: a
// workflow nobody could read would be a gate nobody could verify.
func loadWorkflows(root string) ([]workflow, error) {
	dir := filepath.Join(root, ".github", "workflows")
	names, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no workflow is delivered; the CI is the gate this tool audits", dir)
	}
	sort.Strings(names)

	workflows := make([]workflow, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			rel = name
		}
		parsed, err := parseWorkflow(rel, data)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, parsed)
	}
	return workflows, nil
}

// frame is the open nesting level the reader is inside: the indentation that
// opened it, the path it addresses, whether its value was a scalar (so nothing
// may nest under it), and how many sequence items it has emitted.
type frame struct {
	indent int
	path   string
	scalar bool
	items  int
}

// parseWorkflow reads the indentation-based subset of YAML a workflow file uses:
// mappings, sequences of mappings, sequences of scalars, flow lists, block
// scalars and comments. It is deliberately strict — it refuses tabs, refuses
// nesting under a scalar, and refuses a line that opens no mapping — because a
// reader that guessed would let a rule pass over a file it never understood.
func parseWorkflow(path string, data []byte) (workflow, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	wf := workflow{Path: path}
	stack := []frame{{indent: -1}}

	for i := 0; i < len(lines); i++ {
		raw := strings.TrimRight(lines[i], " \t")
		content := strings.TrimSpace(raw)
		if content == "" || strings.HasPrefix(content, "#") {
			continue
		}
		if strings.HasPrefix(raw, "\t") {
			return wf, fmt.Errorf("%s:%d: a tab indents this line; YAML forbids tabs", path, i+1)
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		body := raw[indent:]

		for len(stack) > 1 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		parent := &stack[len(stack)-1]
		if parent.scalar {
			return wf, fmt.Errorf("%s:%d: nesting under a scalar value", path, i+1)
		}

		if strings.HasPrefix(body, "- ") || body == "-" {
			item := strings.TrimSpace(strings.TrimPrefix(body, "-"))
			itemPath := fmt.Sprintf("%s[%d]", parent.path, parent.items)
			parent.items++

			key, value, comment := splitMapping(item)
			if key == "" {
				// A scalar item: `branches: [main]` written as a block list.
				wf.entries = append(wf.entries, entry{Path: itemPath, Value: value, Comment: comment, Line: i + 1})
				stack = append(stack, frame{indent: indent, path: itemPath, scalar: true})
				continue
			}
			if isBlockIndicator(value) {
				// A step whose whole body is a script: `- run: |`. The block
				// consumed its own lines, so the item stays open for the keys
				// that follow it (`env:`, `with:`).
				block, next := readBlock(lines, i, indent)
				wf.entries = append(wf.entries, entry{Path: itemPath + "." + key, Key: key, Value: fold(block, value), Comment: comment, Line: i + 1})
				stack = append(stack, frame{indent: indent, path: itemPath})
				i = next - 1
				continue
			}
			wf.entries = append(wf.entries, entry{Path: itemPath + "." + key, Key: key, Value: value, Comment: comment, Line: i + 1})
			// The item's remaining keys continue deeper than the dash, so the
			// item itself stays open while they are read.
			stack = append(stack, frame{indent: indent, path: itemPath})
			if value == "" {
				stack = append(stack, frame{indent: indent + 1, path: itemPath + "." + key, scalar: false})
			}
			continue
		}

		key, value, comment := splitMapping(body)
		if key == "" {
			return wf, fmt.Errorf("%s:%d: %q opens no mapping entry", path, i+1, body)
		}
		full := key
		if parent.path != "" {
			full = parent.path + "." + key
		}
		if isBlockIndicator(value) {
			block, next := readBlock(lines, i, indent)
			wf.entries = append(wf.entries, entry{Path: full, Key: key, Value: fold(block, value), Comment: comment, Line: i + 1})
			stack = append(stack, frame{indent: indent, path: full, scalar: true})
			i = next - 1
			continue
		}
		wf.entries = append(wf.entries, entry{Path: full, Key: key, Value: value, Comment: comment, Line: i + 1})
		stack = append(stack, frame{indent: indent, path: full, scalar: value != ""})
	}
	return wf, nil
}

// isBlockIndicator reports whether a value opens a block scalar, which is how a
// multi-line `run:` or `if:` is written.
func isBlockIndicator(value string) bool {
	return strings.HasPrefix(value, "|") || strings.HasPrefix(value, ">")
}

// readBlock collects the lines of a block scalar that opens on line `start`.
// Blank lines stay inside the block; the block ends at the first line that is
// not deeper than the key that opened it. The returned index is the first line
// after the block.
func readBlock(lines []string, start, keyIndent int) ([]string, int) {
	var body []string
	blockIndent := -1
	j := start + 1
	for ; j < len(lines); j++ {
		line := strings.TrimRight(lines[j], " \t")
		if strings.TrimSpace(line) == "" {
			body = append(body, "")
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent <= keyIndent {
			break
		}
		if blockIndent < 0 {
			blockIndent = indent
		}
		if indent < blockIndent {
			break
		}
		body = append(body, line[blockIndent:])
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	return body, j
}

// fold joins the lines of a block scalar. `|` keeps its newlines, which is what
// a shell script needs; `>` folds them, which is what a long condition reads
// as.
func fold(lines []string, indicator string) string {
	if strings.HasPrefix(indicator, ">") {
		return strings.TrimSpace(strings.Join(lines, " "))
	}
	return strings.Join(lines, "\n")
}

// splitMapping reads one line as `key: value # comment`. It returns an empty
// key for a line that is a bare scalar, which is how a block list of scalars is
// written.
func splitMapping(line string) (key, value, comment string) {
	body := line
	if idx := commentIndex(line); idx >= 0 {
		body = strings.TrimSpace(line[:idx])
		comment = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[idx:]), "#"))
	}
	match := mappingPattern.FindStringSubmatch(body)
	if match == nil {
		return "", unquote(body), comment
	}
	return match[1], unquote(strings.TrimSpace(body[len(match[1])+1:])), comment
}

// mappingPattern is the shape of a key: a name that starts a mapping entry. A
// colon inside a value without this shape (`ports` entries, a URL in a scalar
// list) therefore stays a value.
var mappingPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*):(\s|$)`)

// commentIndex finds the `#` that starts a trailing comment: a hash preceded by
// whitespace and outside quotes, so that `--severity CRITICAL,HIGH` and
// `"a # b"` keep their bytes.
func commentIndex(line string) int {
	single, double := false, false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !double {
				single = !single
			}
		case '"':
			if !single {
				double = !double
			}
		case '#':
			if !single && !double && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
				return i
			}
		}
	}
	return -1
}

// unquote strips the quotes that surround a scalar, so that `'1'` and `1` are
// the same value to a rule.
func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// at returns the entry exactly at a path.
func (w workflow) at(path string) (entry, bool) {
	for _, e := range w.entries {
		if e.Path == path {
			return e, true
		}
	}
	return entry{}, false
}

// has reports whether a path exists at all.
func (w workflow) has(path string) bool {
	_, ok := w.at(path)
	return ok
}

// under returns every entry below a path: the path itself, the keys under it,
// and the items of the sequences under it.
func (w workflow) under(path string) []entry {
	var out []entry
	for _, e := range w.entries {
		switch {
		case e.Path == path,
			strings.HasPrefix(e.Path, path+"."),
			strings.HasPrefix(e.Path, path+"["):
			out = append(out, e)
		}
	}
	return out
}

// jobIDs lists the jobs the file declares, in name order.
func (w workflow) jobIDs() []string {
	seen := map[string]bool{}
	var ids []string
	for _, e := range w.entries {
		parts := strings.SplitN(e.Path, ".", 3)
		if len(parts) < 2 || parts[0] != "jobs" || parts[1] == "" {
			continue
		}
		job := parts[1]
		if idx := strings.Index(job, "["); idx >= 0 {
			job = job[:idx]
		}
		if !seen[job] {
			seen[job] = true
			ids = append(ids, job)
		}
	}
	sort.Strings(ids)
	return ids
}

// stepPaths lists the items of a job's `steps:` sequence, in order.
func (w workflow) stepPaths(job string) []string {
	prefix := "jobs." + job + ".steps["
	var paths []string
	for _, e := range w.entries {
		if !strings.HasPrefix(e.Path, prefix) {
			continue
		}
		rest := e.Path[len(prefix):]
		idx := strings.Index(rest, "]")
		if idx < 0 {
			continue
		}
		path := prefix + rest[:idx] + "]"
		if len(paths) == 0 || paths[len(paths)-1] != path {
			paths = append(paths, path)
		}
	}
	return paths
}

// usesEntries returns every action the file invokes.
func (w workflow) usesEntries() []entry {
	var out []entry
	for _, e := range w.entries {
		if e.Key == "uses" {
			out = append(out, e)
		}
	}
	return out
}

// flowList reads a list written either inline (`[a, b]`) or as a block
// sequence, which is how the two forms of `branches:` and `types:` are read as
// one.
func (w workflow) flowList(path string) []string {
	var out []string
	if e, ok := w.at(path); ok && e.Value != "" {
		out = append(out, splitFlow(e.Value)...)
	}
	for _, e := range w.under(path + "[") {
		if e.Value != "" {
			out = append(out, splitFlow(e.Value)...)
		}
	}
	return out
}

// splitFlow reads a flow list into its entries, and leaves a plain scalar
// alone.
func splitFlow(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = unquote(strings.TrimSpace(part)); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parentPath drops the last segment of a path.
func parentPath(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return path[:idx]
	}
	return ""
}
