// Command i18naudit scans the delivered tree for the two defects a catalog
// cannot catch by itself (P18-T10): a document that hardcodes the words a
// person reads, and a document that hardcodes — or omits — the language and
// the direction it claims to be written in.
//
// Why this is a scanner and not a test of one surface: the catalog is complete
// by construction (the generator refuses a locale that drifts), so the only
// way a literal sentence reaches a reader is a *new* document written outside
// the catalog. That is a property of the tree, not of a function, and the
// cheapest honest way to hold it is to read the tree: every HTML document in
// Go source is parsed, and its text nodes and its `<html>` tag are checked.
//
// Three rules, each with the failure it prevents:
//
//   - prose: a text node of a document that contains a sentence of its own.
//     Words between `>` and `<` that are not a template action are text the
//     catalog never produced, so no locale can translate them.
//   - language/direction: an `<html>` tag whose `lang` is a literal, or that
//     carries no `dir`, or whose `dir` is not derived from the same field as
//     its `lang`. A page that declares a language it was not rendered in is
//     wrong for every reader and for every assistive technology, and a page
//     without a direction is a page that will silently break the first time a
//     right-to-left locale is added.
//   - direction in CSS: a physical property (`margin-left`, `left:`,
//     `text-align: left`) in a delivered stylesheet. Logical properties are
//     what make one sheet serve both directions.
//
// The scanner is deliberately conservative: it reads only string literals that
// look like documents (they contain `<html`) and only `.css` files, and it
// never rewrites anything. It is fail-closed: a file it cannot parse is an
// error, not a skip.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Rule names the checks, so a failure says which property broke.
type Rule string

const (
	// RuleProse is a text node carrying its own sentence.
	RuleProse Rule = "prose"
	// RuleLanguage is a document whose language, direction or both are not
	// derived from the locale it was rendered in.
	RuleLanguage Rule = "language"
	// RuleDirection is a physical property in a delivered stylesheet.
	RuleDirection Rule = "direction"
	// RuleUnreadable is a file the scanner could not read, which is a
	// failure and never a skip.
	RuleUnreadable Rule = "unreadable"
)

// Finding is one violation, with the position a reviewer needs.
type Finding struct {
	// Path is the file, relative to the scanned root.
	Path string
	// Line is the 1-based line inside that file.
	Line int
	// Rule names the property that broke.
	Rule Rule
	// Detail explains the violation in one sentence, quoting the text.
	Detail string
}

// String renders a finding as one reviewable line.
func (finding Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", finding.Path, finding.Line, finding.Rule, finding.Detail)
}

// Options configures one scan.
type Options struct {
	// Skip lists directory prefixes (relative to the root, slash-separated)
	// that are never scanned. It exists for the fixtures of this tool: they
	// are documents written to violate the rules on purpose, and a
	// repository-wide scan must not read them.
	Skip []string
}

// Audit scans a tree and returns every violation, ordered by path and line.
func Audit(root string, options Options) ([]Finding, error) {
	var findings []Finding
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if ignorableDirectory(entry.Name()) || skipped(relative, options.Skip) {
				return filepath.SkipDir
			}
			return nil
		}
		if skipped(relative, options.Skip) {
			return nil
		}

		switch {
		case strings.HasSuffix(relative, ".go") && !strings.HasSuffix(relative, "_test.go"):
			found, err := auditGoFile(relative, path)
			if err != nil {
				findings = append(findings, Finding{Path: relative, Line: 1, Rule: RuleUnreadable, Detail: err.Error()})
				return nil
			}
			findings = append(findings, found...)
		case strings.HasSuffix(relative, ".css"):
			found, err := auditStylesheet(relative, path)
			if err != nil {
				findings = append(findings, Finding{Path: relative, Line: 1, Rule: RuleUnreadable, Detail: err.Error()})
				return nil
			}
			findings = append(findings, found...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(findings, func(left, right int) bool {
		if findings[left].Path != findings[right].Path {
			return findings[left].Path < findings[right].Path
		}
		return findings[left].Line < findings[right].Line
	})
	return findings, nil
}

// ignorableDirectory reports whether a directory is outside the delivered tree.
func ignorableDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "test-build", "vendor":
		return true
	default:
		return false
	}
}

// skipped reports whether a relative path is under one of the skip prefixes.
func skipped(relative string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefix = strings.TrimSuffix(filepath.ToSlash(prefix), "/")
		if prefix == "" {
			continue
		}
		if relative == prefix || strings.HasPrefix(relative, prefix+"/") {
			return true
		}
	}
	return false
}

// documentLiteralPattern recognises the string literals that are documents.
// A literal is a document when it opens an html element; anything else (a
// selector, a URL, a JSON payload) is not read by these rules.
var documentLiteralPattern = regexp.MustCompile(`(?i)<html[\s>]`)

// templateActionPattern matches one `{{ ... }}` action of html/template.
var templateActionPattern = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// fieldActionPattern matches an action that prints one field of the data
// (`.Lang`, `.Language`), which is what the language and direction rules
// compare.
var fieldActionPattern = regexp.MustCompile(`^\{\{\s*\.([A-Za-z][A-Za-z0-9_]*)\s*\}\}$`)

// htmlTagPattern matches the opening tag of the document.
var htmlTagPattern = regexp.MustCompile(`(?is)<html\b[^>]*>`)

// attributePattern matches one attribute of a tag.
var attributePattern = regexp.MustCompile(`(?is)([a-zA-Z-]+)\s*=\s*"([^"]*)"`)

// directionFunctionPattern matches the `dir` template function applied to a
// field: `{{dir .Lang}}`, the only accepted way to write the attribute.
var directionFunctionPattern = regexp.MustCompile(`^\{\{\s*dir\s+\.([A-Za-z][A-Za-z0-9_]*)\s*\}\}$`)

// embeddedBlockPattern removes the contents of style and script elements:
// CSS and JSON are not prose a person reads in the document's flow, and the
// direction rule has its own stylesheet scan.
var embeddedBlockPattern = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>|<script\b[^>]*>.*?</script>`)

// textNodePattern finds the text between tags.
var textNodePattern = regexp.MustCompile(`(?s)>([^<]+)<`)

// wordPattern matches a word of at least two letters.
var wordPattern = regexp.MustCompile(`[[:alpha:]]{2,}`)

// auditGoFile reads one Go source file and audits every document literal in it.
func auditGoFile(relative, path string) ([]Finding, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, source, 0)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var findings []Finding
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value := unquote(literal.Value)
		if !documentLiteralPattern.MatchString(value) {
			return true
		}
		// The value of a raw literal starts on the line the literal
		// starts, so counting the newlines before an offset maps a
		// finding to the line a reviewer opens.
		baseLine := set.Position(literal.Pos()).Line
		findings = append(findings, auditDocument(relative, baseLine, value)...)
		return true
	})
	return findings, nil
}

// unquote turns a Go string literal into its value, tolerating the raw and
// interpreted forms. A literal that cannot be unquoted is left as written,
// which only makes the rules see the quotes — never a silent skip.
func unquote(literal string) string {
	if len(literal) < 2 {
		return literal
	}
	switch literal[0] {
	case '`':
		return strings.TrimSuffix(strings.TrimPrefix(literal, "`"), "`")
	case '"':
		return strings.TrimSuffix(strings.TrimPrefix(literal, `"`), `"`)
	default:
		return literal
	}
}

// auditDocument applies the prose and the language rules to one document,
// anchoring every finding on the line of the literal plus the newlines the
// document has before it.
func auditDocument(relative string, baseLine int, document string) []Finding {
	var findings []Finding

	tag := htmlTagPattern.FindString(document)
	if tag == "" {
		return nil
	}

	attributes := map[string]string{}
	for _, match := range attributePattern.FindAllStringSubmatch(tag, -1) {
		attributes[strings.ToLower(match[1])] = match[2]
	}

	if detail, ok := languageViolation(attributes); !ok {
		findings = append(findings, Finding{Path: relative, Line: baseLine, Rule: RuleLanguage, Detail: detail})
	}

	// Style and script contents are blanked rather than removed: the
	// offsets of everything after them stay where they were, so a finding
	// is anchored on the line a reviewer opens.
	body := embeddedBlockPattern.ReplaceAllStringFunc(document, blankKeepingLines)
	for _, location := range textNodePattern.FindAllStringSubmatchIndex(body, -1) {
		text := templateActionPattern.ReplaceAllString(body[location[2]:location[3]], " ")
		words := wordPattern.FindAllString(text, -1)
		if len(words) < 2 {
			continue
		}
		findings = append(findings, Finding{
			Path:   relative,
			Line:   baseLine + strings.Count(document[:offsetWithin(location[2], len(document))], "\n"),
			Rule:   RuleProse,
			Detail: fmt.Sprintf("text outside the catalog: %q", strings.Join(words, " ")),
		})
	}
	return findings
}

// languageViolation checks the `<html>` tag: the language must be the rendered
// locale, and the direction must be derived from that same locale.
func languageViolation(attributes map[string]string) (string, bool) {
	language, hasLanguage := attributes["lang"]
	if !hasLanguage {
		return "the document declares no language", false
	}
	languageMatch := fieldActionPattern.FindStringSubmatch(strings.TrimSpace(language))
	if languageMatch == nil {
		return fmt.Sprintf("the language is a literal (%q) instead of the rendered locale", language), false
	}

	direction, hasDirection := attributes["dir"]
	if !hasDirection {
		return "the document declares no direction", false
	}
	directionMatch := directionFunctionPattern.FindStringSubmatch(strings.TrimSpace(direction))
	if directionMatch == nil {
		return fmt.Sprintf("the direction (%q) is not derived from the locale by the shared helper", direction), false
	}
	if directionMatch[1] != languageMatch[1] {
		return fmt.Sprintf("the direction is derived from .%s while the language is .%s", directionMatch[1], languageMatch[1]), false
	}
	return "", true
}

// physicalDirectionPatterns are the properties that only work in one writing
// direction. Each entry also names the logical replacement, so a failure tells
// the author what to write instead.
var physicalDirectionPatterns = []struct {
	pattern   *regexp.Regexp
	logical   string
	condition string
}{
	{regexp.MustCompile(`(?m)(^|[;{\s])margin-left\s*:`), "margin-inline-start", "left margin in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])margin-right\s*:`), "margin-inline-end", "right margin in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])padding-left\s*:`), "padding-inline-start", "left padding in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])padding-right\s*:`), "padding-inline-end", "right padding in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])border-left\s*:`), "border-inline-start", "left border in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])border-right\s*:`), "border-inline-end", "right border in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])text-align\s*:\s*left\b`), "text-align: start", "left alignment in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])text-align\s*:\s*right\b`), "text-align: start", "right alignment in one direction only"},
	{regexp.MustCompile(`(?m)(^|[;{\s])(left|right)\s*:`), "inset-inline-start/end", "physical offset"},
	{regexp.MustCompile(`(?m)(^|[;{\s])float\s*:\s*(left|right)\b`), "float: inline-start/inline-end", "physical float"},
}

// cssCommentPattern removes comments so a rule inside one is not a violation.
var cssCommentPattern = regexp.MustCompile(`(?s)/\*.*?\*/`)

// auditStylesheet applies the direction rule to one stylesheet.
func auditStylesheet(relative, path string) ([]Finding, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := cssCommentPattern.ReplaceAllString(string(source), "")

	var findings []Finding
	for _, rule := range physicalDirectionPatterns {
		for _, match := range rule.pattern.FindAllStringIndex(content, -1) {
			findings = append(findings, Finding{
				Path: relative,
				Line: lineAt(content, match[0]),
				Rule: RuleDirection,
				Detail: fmt.Sprintf("%s (%s); use %s instead",
					strings.TrimSpace(content[match[0]:match[1]]), rule.condition, rule.logical),
			})
		}
	}
	return findings, nil
}

// blankKeepingLines replaces a block with spaces, keeping its newlines so the
// offsets of the rest of the document do not move.
func blankKeepingLines(block string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' {
			return '\n'
		}
		return ' '
	}, block)
}

// offsetWithin clamps an offset to a length, so a defensive slice is always
// valid.
func offsetWithin(offset, length int) int {
	if offset > length {
		return length
	}
	return offset
}

// lineAt returns the 1-based line of a byte offset.
func lineAt(content string, offset int) int {
	if offset > len(content) {
		offset = len(content)
	}
	return strings.Count(content[:offset], "\n") + 1
}
