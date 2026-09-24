package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The rules of this gate. Every finding names one of them, and the report has no
// bucket called "other": a finding the gate cannot classify is a finding the gate
// refuses to have.
const (
	RulePlaceholderPanic = "placeholder-panic"
	RuleFalseSuccess     = "false-success"
	RuleDeferred         = "deferred-without-reference"
	RuleUnreachable      = "unreachable-statement"
	RuleConstantBranch   = "constant-branch"
	RuleConfigurationKey = "configuration-key"
)

// rules is the declared vocabulary, printed by every run so that the report can
// be read without the source. A rule that is not here cannot be produced: the
// scan returns findings by name, and a name outside this table is a programming
// error the gate turns into a refusal instead of a line.
var rules = []rule{
	{RulePlaceholderPanic, "um panic que diz que o trabalho não foi feito é a falha mais barata de escrever e a mais cara de encontrar em produção"},
	{RuleFalseSuccess, "a função que se anuncia como não pronta e devolve sucesso é a que faz o chamador seguir como se tivesse funcionado"},
	{RuleDeferred, "o adiamento só é honesto quando nomeia quem o resolve: marcador sem referência é uma frase que ninguém cobra"},
	{RuleUnreachable, "código depois de um término incondicional nunca roda, e o leitor paga por ele em toda revisão"},
	{RuleConstantBranch, "um desvio sobre literal constante é uma decisão que já foi tomada e ficou escrita como se não fosse"},
	{RuleConfigurationKey, "a variável que o operador define e o processo ignora, ou a que o processo aceita e ninguém documenta, é configuração que mente"},
}

type rule struct {
	Name   string
	Reason string
}

func ruleKnown(name string) bool {
	for _, entry := range rules {
		if entry.Name == name {
			return true
		}
	}
	return false
}

// finding is one refusal: where it is, which rule it breaks and what the gate
// read to decide. The detail is part of the finding on purpose — a refusal that
// does not show the text it refused is a refusal nobody can argue with.
type finding struct {
	Rule   string
	Path   string
	Line   int
	Detail string
}

func (f finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Rule, f.Detail)
}

// measured is everything one run observed.
type measured struct {
	Files      int
	Functions  int
	Comments   int
	Generated  []string
	Accepted   []finding
	Findings   []finding
	Unreadable map[string]string
	// Exported counts the package-level declarations a consumer can name and
	// Unconsumed lists the ones no other package of this tree mentions. The two
	// numbers are the measurement behind the gap this gate declares: what they
	// describe is not dead code, it is the surface the consumers outside the
	// tree are written against.
	Exported   int
	Unconsumed []string
}

// The two vocabularies of the gate. A marker opens a deferral in a comment; a
// placeholder says in the message of a panic, or in the comment a function
// documents itself with, that the work is not done. The second one never matches
// a bare Portuguese "todo" — "todo o estado" is a sentence — so the code markers
// are matched in upper case only, the same decision the requirement audit of
// P20-T02 records for the same words.
var (
	deferredMarkerNames = []string{"TODO", "FIXME", "HACK", "XXX"}

	placeholderPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bnot implemented\b`),
		regexp.MustCompile(`(?i)\bunimplemented\b`),
		regexp.MustCompile(`(?i)\bnot ready\b`),
		regexp.MustCompile(`(?i)\bplaceholder\b`),
		regexp.MustCompile(`(?i)\bstub\b`),
		regexp.MustCompile(`(?i)\bpor enquanto\b`),
		regexp.MustCompile(`(?i)\bn[ãa]o implementad`),
		regexp.MustCompile(`(?i)\bsem implementa[çc][ãa]o\b`),
		regexp.MustCompile(`\b(TODO|FIXME|XXX)\b`),
	}
)

// deferredPattern is the shape a deferral has to have: the marker, immediately
// followed by the reference that resolves it in parentheses. `TODO(P23-T04):`
// and `TODO(#85):` are references; `TODO: fix this later` is a sentence.
var deferredPattern = regexp.MustCompile(`\b(TODO|FIXME|HACK|XXX)\(([^)]*)\)`)

// referencePattern is what a reference may be: the task of this repository's
// plan, or the issue it is tracked in.
var referencePattern = regexp.MustCompile(`^(P[0-9]{1,3}-T[0-9]{1,3}[A-Za-z0-9-]*|#[0-9]{1,6})$`)

// supportsPlaceholder reports whether a text carries a marker. The marker is the
// evidence that the function does not do the work.
func supportsPlaceholder(text string) bool {
	for _, pattern := range placeholderPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// commentBody is the text of a comment without its delimiters, so that `//`,
// `/* */` and the asterisks of a block of prose read the same way.
func commentBody(text string) string {
	body := text
	switch {
	case strings.HasPrefix(body, "//"):
		body = body[2:]
	case strings.HasPrefix(body, "/*"):
		body = strings.TrimSuffix(strings.TrimPrefix(body, "/*"), "*/")
	}
	body = strings.TrimSpace(strings.TrimLeft(body, "* \t"))
	return body
}

// deferralMarker answers the marker a comment proposes a deferral with, or "".
//
// The marker has to open the comment. `// TODO(P23-T04): close the port` is a
// deferral; a comment that *discusses* the word — the marker rule of another
// gate, or this gate's own documentation — is prose, and reading it as a promise
// would make the gate refuse the documents that explain the convention. Case is
// part of the decision too: the code markers are conventions of the source,
// while the lowercase word is ordinary Portuguese ("todo requisito").
func deferralMarker(text string) string {
	body := commentBody(text)
	for _, marker := range deferredMarkerNames {
		if !strings.HasPrefix(body, marker) {
			continue
		}
		rest := body[len(marker):]
		if rest == "" || !isWordByte(rest[0]) {
			return marker
		}
	}
	return ""
}

// isWordByte reports whether a byte continues an identifier, which is how the
// gate tells the marker `XXX` from the token of another vocabulary, `XXXX`.
func isWordByte(character byte) bool {
	return character == '_' ||
		(character >= '0' && character <= '9') ||
		(character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		character >= 0x80
}

// skippedDirectories are the directories the walk does not enter. Generated code
// is excluded by provenance (the marker in the file), never by where it lives.
var skippedDirectories = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "testdata": true, ".local": true, "dist": true,
}

var generatedLine = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// isGenerated reads the generated marker the way the Go toolchain does: in the
// comments before the package clause. A file that merely mentions the text — the
// generators of this repository do, inside a string — is code and is judged.
func isGenerated(file *ast.File) bool {
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		for _, comment := range group.List {
			if generatedLine.MatchString(strings.TrimRight(comment.Text, " \t")) {
				return true
			}
		}
	}
	return false
}

// scanTree judges the repository.
func scanTree() (measured, error) {
	files, err := goFiles(".", skippedDirectories)
	if err != nil {
		return measured{}, err
	}
	return scan(files)
}

// scanDirectory judges one directory and everything under it, which is how the
// gate exercises its own fixtures: a fixture lives under testdata, exactly the
// place the tree walk refuses to enter.
func scanDirectory(directory string) (measured, error) {
	files, err := goFiles(directory, nil)
	if err != nil {
		return measured{}, err
	}
	return scan(files)
}

// goFiles lists every Go file under a root, refusing to enter the directories a
// caller names. The list is ordered, because two runs of a gate that answers
// differently are two gates.
func goFiles(root string, skipped map[string]bool) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skipped[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}

// scan reads every file and returns what it found. A file it cannot read is a
// file whose silence nobody can vouch for, and the answer to that is a refusal.
func scan(files []string) (measured, error) {
	result := measured{Unreadable: map[string]string{}}
	positions := token.NewFileSet()
	mentions := map[string]map[string]bool{}
	declarations := []declared{}
	for _, path := range files {
		source, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			return measured{}, fmt.Errorf("read %s: %w", path, err)
		}
		file, err := parser.ParseFile(positions, path, source, parser.ParseComments)
		if err != nil {
			result.Unreadable[path] = err.Error()
			continue
		}
		result.Files++
		if isGenerated(file) {
			result.Generated = append(result.Generated, path)
			continue
		}
		scanFile(&result, positions, path, file)
		indexMentions(mentions, path, file)
		declarations = append(declarations, exportedDeclarations(file)...)
	}
	result.Exported = len(declarations)
	for _, entry := range declarations {
		if len(mentions[entry.Name]) <= 1 {
			result.Unconsumed = append(result.Unconsumed, entry.Name)
		}
	}
	sort.Strings(result.Unconsumed)
	sortFindings(result.Findings)
	sortFindings(result.Accepted)
	return result, nil
}

// sortFindings orders findings by path, line and rule. Two runs of a gate that
// answers in a different order are two gates.
func sortFindings(findings []finding) {
	sort.Slice(findings, func(one, other int) bool {
		if findings[one].Path != findings[other].Path {
			return findings[one].Path < findings[other].Path
		}
		if findings[one].Line != findings[other].Line {
			return findings[one].Line < findings[other].Line
		}
		return findings[one].Rule < findings[other].Rule
	})
}

func scanFile(result *measured, positions *token.FileSet, path string, file *ast.File) {
	// 1. The comments: a deferral names who resolves it, and the deferral the
	// gate accepts is printed — a silence nobody sees is a silence nobody
	// revisits.
	for _, group := range file.Comments {
		for _, comment := range group.List {
			result.Comments++
			line := positions.Position(comment.Slash).Line
			if marker := deferralMarker(comment.Text); marker != "" {
				result.deferral(path, line, marker, comment.Text)
			}
		}
	}

	// 2. The declarations.
	for _, declaration := range file.Decls {
		declared, ok := declaration.(*ast.FuncDecl)
		if !ok || declared.Body == nil {
			continue
		}
		result.Functions++
		if text := placeholderText(declared, file); text != "" && returnsSuccessWithoutWork(declared.Body) {
			result.Findings = append(result.Findings, finding{
				Rule: RuleFalseSuccess, Path: path,
				Line:   positions.Position(declared.Pos()).Line,
				Detail: fmt.Sprintf("%s anuncia %q e devolve sucesso sem fazer o trabalho", funcName(declared), text),
			})
		}
		scanStatements(result, positions, path, declared.Body)
	}
}

// deferral files one comment that opens with a marker: either the deferral is
// tracked and the gate prints it, or the gate refuses it and says what a
// reference looks like.
func (result *measured) deferral(path string, line int, marker, text string) {
	if reference := deferredPattern.FindStringSubmatch(text); reference != nil {
		who := strings.TrimSpace(reference[2])
		if referencePattern.MatchString(who) {
			result.Accepted = append(result.Accepted, finding{
				Rule: RuleDeferred, Path: path, Line: line,
				Detail: fmt.Sprintf("adiado com dono declarado: %s", who),
			})
			return
		}
		result.Findings = append(result.Findings, finding{
			Rule: RuleDeferred, Path: path, Line: line,
			Detail: fmt.Sprintf("%q nomeia %q, que não é uma tarefa do plano (Pnn-Tnn) nem uma issue (#nn)", marker+"("+who+")", who),
		})
		return
	}
	result.Findings = append(result.Findings, finding{
		Rule: RuleDeferred, Path: path, Line: line,
		Detail: fmt.Sprintf("%s sem referência — escreva %s(P23-Tnn) ou %s(#nn), ou remova o adiamento (%q)", marker, marker, marker, commentBody(text)),
	})
}

// placeholderText answers what a function says about itself: a marker in the
// comment it documents itself with, or in a comment inside its body.
func placeholderText(declared *ast.FuncDecl, file *ast.File) string {
	for _, comment := range commentList(declared.Doc) {
		if supportsPlaceholder(comment.Text) {
			return commentBody(comment.Text)
		}
	}
	for _, group := range file.Comments {
		if group.Pos() <= declared.Body.Pos() || group.End() >= declared.Body.End() {
			continue
		}
		for _, comment := range group.List {
			if supportsPlaceholder(comment.Text) {
				return commentBody(comment.Text)
			}
		}
	}
	return ""
}

func commentList(group *ast.CommentGroup) []*ast.Comment {
	if group == nil {
		return nil
	}
	return group.List
}

// returnsSuccessWithoutWork reports whether a body is exactly one return of a
// success value: nil, true or a zero literal. A body that returns an error is
// not a false success — it is a refusal, which is what an unfinished function
// owes its caller.
func returnsSuccessWithoutWork(body *ast.BlockStmt) bool {
	if len(body.List) != 1 {
		return false
	}
	statement, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) == 0 {
		return false
	}
	for _, result := range statement.Results {
		if !isSuccessValue(result) {
			return false
		}
	}
	return true
}

// isSuccessValue reports whether an expression is a value a caller reads as
// "it worked": nil, true, or the zero of a type.
func isSuccessValue(expression ast.Expr) bool {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name == "nil" || typed.Name == "true"
	case *ast.BasicLit:
		return typed.Value == `""` || typed.Value == "0" || typed.Value == "0.0"
	case *ast.UnaryExpr:
		return typed.Op == token.SUB && isSuccessValue(typed.X)
	}
	return false
}

// scanStatements walks the blocks of one function looking for code that cannot
// run and for branches decided by a literal.
func scanStatements(result *measured, positions *token.FileSet, path string, body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BlockStmt:
			scanBlock(result, positions, path, typed.List)
		case *ast.CaseClause:
			// The body of a case clause is a list and not a block, so the
			// fixture of the fallthrough found this hole: a scan that only
			// walked blocks judged every statement inside a switch except
			// the ones a reader skims.
			scanBlock(result, positions, path, typed.Body)
		case *ast.CommClause:
			scanBlock(result, positions, path, typed.Body)
		case *ast.IfStmt:
			if literal, ok := typed.Cond.(*ast.Ident); ok && (literal.Name == "false" || literal.Name == "true") {
				result.Findings = append(result.Findings, finding{
					Rule: RuleConstantBranch, Path: path,
					Line:   positions.Position(typed.If).Line,
					Detail: fmt.Sprintf("if %s: a condição é o literal %s, então o desvio já está decidido", literal.Name, literal.Name),
				})
			}
		case *ast.CallExpr:
			if isPlaceholderPanic(typed) {
				result.Findings = append(result.Findings, finding{
					Rule: RulePlaceholderPanic, Path: path,
					Line:   positions.Position(typed.Pos()).Line,
					Detail: fmt.Sprintf("panic(%s): a mensagem diz que o trabalho não foi feito", messageOf(typed)),
				})
			}
		}
		return true
	})
}

// scanBlock refuses the statements that follow a statement which always
// terminates. A label below the terminating statement is left alone on purpose:
// it is a jump target, so the code after it can run. The list is a list of
// statements and not a block, because a case clause holds one too.
func scanBlock(result *measured, positions *token.FileSet, path string, statements []ast.Stmt) {
	for index, statement := range statements {
		if !terminates(statement) {
			continue
		}
		for _, following := range statements[index+1:] {
			if _, labelled := following.(*ast.LabeledStmt); labelled {
				return
			}
			result.Findings = append(result.Findings, finding{
				Rule: RuleUnreachable, Path: path,
				Line:   positions.Position(following.Pos()).Line,
				Detail: fmt.Sprintf("depois de %s na linha %d nada mais roda", describeTerminator(statement), positions.Position(statement.Pos()).Line),
			})
		}
	}
}

// terminates reports whether a statement always ends the flow of the block it is
// in. Only the syntactic terminators are here: a call to a function that ends
// the process, like `t.Fatal`, is not one of them, and pretending otherwise
// would refuse code that runs.
func terminates(statement ast.Stmt) bool {
	switch typed := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return typed.Tok == token.FALLTHROUGH
	case *ast.ExprStmt:
		call, ok := typed.X.(*ast.CallExpr)
		return ok && isPanic(call)
	}
	return false
}

func describeTerminator(statement ast.Stmt) string {
	switch statement.(type) {
	case *ast.ReturnStmt:
		return "um return"
	case *ast.BranchStmt:
		return "um fallthrough"
	case *ast.ExprStmt:
		return "um panic"
	}
	return "um término"
}

func isPanic(call *ast.CallExpr) bool {
	identifier, ok := call.Fun.(*ast.Ident)
	return ok && identifier.Name == "panic"
}

// isPlaceholderPanic reports whether a panic call says the work is not done.
func isPlaceholderPanic(call *ast.CallExpr) bool {
	if !isPanic(call) || len(call.Args) == 0 {
		return false
	}
	return supportsPlaceholder(literalText(call.Args[0]))
}

// messageOf renders the first argument of a call for the report.
func messageOf(call *ast.CallExpr) string {
	if len(call.Args) == 0 {
		return "(sem mensagem)"
	}
	return literalText(call.Args[0])
}

// literalText reads the text of an expression when it is a literal, and answers
// with a description when it is not: the report shows what the gate read.
func literalText(expression ast.Expr) string {
	literal, ok := expression.(*ast.BasicLit)
	if !ok {
		return "(expressão)"
	}
	return strings.Trim(literal.Value, `"`+"`")
}

// funcName names a function the way a reviewer reads it.
func funcName(declaration *ast.FuncDecl) string {
	name := declaration.Name.Name
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return name
	}
	switch receiver := declaration.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return receiver.Name + "." + name
	case *ast.StarExpr:
		if identifier, ok := receiver.X.(*ast.Ident); ok {
			return identifier.Name + "." + name
		}
	}
	return name
}

// declared is one package-level declaration a consumer can name.
type declared struct {
	Name string
}

// exportedDeclarations lists the declarations a consumer of the package can
// name. Methods are deliberately absent: a method is reached through a value or
// an interface, so counting them would mix two different questions.
func exportedDeclarations(file *ast.File) []declared {
	declarations := []declared{}
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			if typed.Recv == nil && ast.IsExported(typed.Name.Name) {
				declarations = append(declarations, declared{Name: typed.Name.Name})
			}
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				declarations = append(declarations, declaredInSpec(spec)...)
			}
		}
	}
	return declarations
}

// declaredInSpec reads the names one spec declares, which is the split the
// aggregate budget of this package asked for: the nesting of a switch inside
// two loops is the shape the complexity gate refuses in everybody's code.
func declaredInSpec(spec ast.Spec) []declared {
	switch entry := spec.(type) {
	case *ast.TypeSpec:
		if ast.IsExported(entry.Name.Name) {
			return []declared{{Name: entry.Name.Name}}
		}
	case *ast.ValueSpec:
		declarations := []declared{}
		for _, name := range entry.Names {
			if ast.IsExported(name.Name) {
				declarations = append(declarations, declared{Name: name.Name})
			}
		}
		return declarations
	}
	return nil
}

// indexMentions records every name a file mentions, with the package the file
// belongs to. A name mentioned by no package other than the one that declares it
// is an export with no consumer inside this tree — which is a measurement, not a
// refusal: see the gap this gate prints.
func indexMentions(mentions map[string]map[string]bool, path string, file *ast.File) {
	directory := filepath.ToSlash(filepath.Dir(path))
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if mentions[identifier.Name] == nil {
			mentions[identifier.Name] = map[string]bool{}
		}
		mentions[identifier.Name][directory] = true
		return true
	})
}
