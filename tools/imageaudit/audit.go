// The audit of the production image (P19-T01). It judges the image twice: the
// recipe that builds it and the artifact that comes out.
//
// The two halves are not redundant. The recipe says what the image is supposed
// to be — a base pinned by digest, a stage that never copies the build context
// wholesale, a runtime stage that declares a non-root user. The exported
// filesystem says what it actually is — the base tag could have moved, a
// `COPY` could have pulled in a local `.env`, a `RUN` could have left a package
// cache behind. A gate that only read the Dockerfile would approve a build that
// did none of it; a gate that only read the image would approve a recipe that
// happens to work today.
//
// Every rule answers a question someone would otherwise answer by reading:
//
//	from_pinned          is every base identified by digest, not by a tag?
//	from_malformed       does every FROM name an image at all?
//	stage_duplicate      do two stages answer to the same name?
//	copy_from_unknown    does a COPY --from name a stage that exists?
//	copy_context_root    does a COPY take the whole build context?
//	add_instruction      is ADD used, which can fetch a remote URL?
//	secret_name          is a secret-shaped ARG or ENV declared?
//	stage_user           does the last stage declare a non-root user?
//	dockerignore_missing is there a .dockerignore at all?
//	dockerignore_coverage does it exclude the paths that must never enter a context?
//	image_user           is the user of the built image non-root?
//	image_entrypoint     does the image declare an entrypoint?
//	image_env            does a secret-shaped environment variable appear in the image?
//	entrypoint_missing   does the declared entrypoint exist in the filesystem?
//	rootfs_tool          does the runtime carry a compiler, interpreter or shell?
//	rootfs_cache         does it carry a package or build cache?
//	rootfs_env           does it carry an environment file?
//	rootfs_vcs           does it carry version control data?
//	rootfs_source        does it carry the sources instead of the built artifact?
//	rootfs_secret        does a file contain a credential or private key?
//	rootfs_unscanned     was a file too large to scan for credentials?
//
// The last one is a rule and not a note because "we could not look" is not
// "there is nothing there": a file this gate refuses to read is a file nobody
// has read.
package main

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// maxScannedBytes bounds the content scan. The arena binary is far below it, so
// in practice every file of the image is read; the bound exists so a rule
// cannot turn an audit into a way to exhaust memory, and the file it skips is
// reported as a violation instead of being forgotten.
const maxScannedBytes = 64 << 20

// Options names the four artifacts the audit reads. All of them are required:
// a gate that cannot see the recipe or the result measures nothing, and one
// that passes without measuring is worse than no gate.
type Options struct {
	Dockerfile   string
	Dockerignore string
	Config       string
	Rootfs       string
}

// Violation is one broken rule, named by where it was found.
type Violation struct {
	Where  string
	Rule   string
	Detail string
}

func (violation Violation) String() string {
	return fmt.Sprintf("%s: %s: %s", violation.Where, violation.Rule, violation.Detail)
}

// Report is the measurement, always produced; the violations are the verdict.
type Report struct {
	Stages      []string
	User        string
	Entrypoint  string
	RootfsFiles int
	RootfsBytes int64
	Violations  []Violation
}

// Audit reads every required artifact and returns what it found. A missing
// artifact is an error, never an empty report: the caller decides what a gate
// that could not run means, and the honest answer is failure.
func Audit(options Options) (Report, error) {
	var report Report

	source, err := os.ReadFile(options.Dockerfile)
	if err != nil {
		return Report{}, fmt.Errorf("read Dockerfile: %w", err)
	}
	stages, err := parseStages(string(source))
	if err != nil {
		return Report{}, fmt.Errorf("%s: %w", options.Dockerfile, err)
	}
	auditDockerfile(&report, options.Dockerfile, stages)

	if err := auditDockerignore(&report, options.Dockerignore); err != nil {
		return Report{}, err
	}

	config, err := readImageConfig(options.Config)
	if err != nil {
		return Report{}, err
	}

	present, err := auditRootfs(&report, options.Rootfs)
	if err != nil {
		return Report{}, err
	}

	auditImageConfig(&report, options.Config, config, present)
	return report, nil
}

// ---------------------------------------------------------------------------
// The recipe: Dockerfile
// ---------------------------------------------------------------------------

// instruction is one logical Dockerfile instruction: a continuation is joined
// into the line it started on, so a rule reports the line a person edits.
type instruction struct {
	Line int
	Name string // upper case
	Args string
}

// stage is one FROM and everything it runs until the next FROM.
type stage struct {
	Name         string
	Line         int
	Instructions []instruction
}

// parseStages splits a Dockerfile into its stages. Comments are ignored, and a
// trailing backslash joins the next line. It never fails on content: a
// malformed instruction is a violation the audit reports with its line, not an
// error that stops the measurement of everything else.
func parseStages(source string) ([]stage, error) {
	instructions := parseInstructions(source)
	var stages []stage
	for _, instruction := range instructions {
		if instruction.Name == "FROM" {
			stages = append(stages, stage{Line: instruction.Line})
			stages[len(stages)-1].Name = fromName(instruction.Args)
		}
		if len(stages) == 0 {
			// An instruction before the first FROM cannot be part of any
			// stage; Docker itself refuses such a file.
			continue
		}
		stages[len(stages)-1].Instructions = append(stages[len(stages)-1].Instructions, instruction)
	}
	if len(stages) == 0 {
		return nil, errors.New("no FROM instruction: the file describes no image")
	}
	return stages, nil
}

func parseInstructions(source string) []instruction {
	lines := strings.Split(source, "\n")
	var instructions []instruction
	var builder strings.Builder
	startLine := 0

	for index, raw := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		insideContinuation := builder.Len() > 0
		if !insideContinuation {
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			startLine = index + 1
		}
		continued := strings.HasSuffix(trimmed, "\\")
		builder.WriteString(strings.TrimSuffix(trimmed, "\\"))
		builder.WriteString(" ")
		if continued {
			continue
		}

		full := strings.TrimSpace(builder.String())
		builder.Reset()
		fields := strings.Fields(full)
		if len(fields) == 0 {
			continue
		}
		instructions = append(instructions, instruction{
			Line: startLine,
			Name: strings.ToUpper(fields[0]),
			Args: strings.Join(fields[1:], " "),
		})
	}
	return instructions
}

// fromName returns the stage name of a FROM instruction, or "" when it is
// anonymous.
func fromName(args string) string {
	fields := strings.Fields(args)
	if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
		return fields[2]
	}
	return ""
}

// secretNameFragments are the name fragments a build argument or environment
// variable may not use. A credential passed to a build is a credential written
// into a layer, where it stays readable to anyone who can pull the image.
var secretNameFragments = []string{
	"SECRET", "TOKEN", "PASSWORD", "PASSWD", "CREDENTIAL",
	"API_KEY", "APIKEY", "PRIVATE_KEY", "ACCESS_KEY", "AUTH_KEY",
}

func secretShapedName(name string) string {
	upper := strings.ToUpper(name)
	for _, fragment := range secretNameFragments {
		if strings.Contains(upper, fragment) {
			return fragment
		}
	}
	return ""
}

func auditDockerfile(report *Report, file string, stages []stage) {
	names := map[string]int{}
	for index, stage := range stages {
		report.Stages = append(report.Stages, describeStage(index, stage))
		if stage.Name == "" {
			continue
		}
		if _, duplicate := names[stage.Name]; duplicate {
			report.violate(file, "stage_duplicate", fmt.Sprintf("line %d: two stages are named %q", stage.Line, stage.Name))
			continue
		}
		names[stage.Name] = index
	}

	for index, stage := range stages {
		for _, instruction := range stage.Instructions {
			switch instruction.Name {
			case "FROM":
				auditFrom(report, file, instruction)
			case "ADD":
				report.violate(file, "add_instruction", fmt.Sprintf("line %d: ADD can fetch a remote URL and unpack an archive; COPY moves a local path and nothing else", instruction.Line))
			case "COPY":
				auditCopy(report, file, instruction, index, names)
			case "ARG", "ENV":
				auditDeclaredName(report, file, instruction)
			}
		}
	}

	auditRuntimeUser(report, file, stages[len(stages)-1])
}

func describeStage(index int, stage stage) string {
	if stage.Name == "" {
		return fmt.Sprintf("%d (unnamed)", index)
	}
	return fmt.Sprintf("%d %s", index, stage.Name)
}

// auditFrom enforces the pinning rule: a tag is a name someone can repoint, a
// digest is not. `image:tag@sha256:...` is accepted because the tag then only
// labels a digest that is checked.
func auditFrom(report *Report, file string, instruction instruction) {
	fields := strings.Fields(instruction.Args)
	if len(fields) == 0 {
		report.violate(file, "from_malformed", fmt.Sprintf("line %d: FROM names no image", instruction.Line))
		return
	}
	reference := fields[0]
	at := strings.LastIndex(reference, "@sha256:")
	if at < 0 {
		report.violate(file, "from_pinned", fmt.Sprintf("line %d: %s is not pinned by digest (append @sha256:...)", instruction.Line, reference))
		return
	}
	digest := reference[at+len("@sha256:"):]
	if !isHexDigest(digest) {
		report.violate(file, "from_pinned", fmt.Sprintf("line %d: %s does not end in a sha256 digest", instruction.Line, reference))
	}
}

func isHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'f':
		default:
			return false
		}
	}
	return true
}

func auditCopy(report *Report, file string, instruction instruction, stageIndex int, names map[string]int) {
	from, operands := copyOperands(instruction.Args)
	if from != "" {
		if index, ok := names[from]; ok {
			if index >= stageIndex {
				report.violate(file, "copy_from_unknown", fmt.Sprintf("line %d: COPY --from=%s reads stage %d, which is not built yet", instruction.Line, from, index))
			}
		} else if isStageIndex(from) {
			index := atoi(from)
			if index >= stageIndex {
				report.violate(file, "copy_from_unknown", fmt.Sprintf("line %d: COPY --from=%s reads stage %d, which is not built yet", instruction.Line, from, index))
			}
		} else if !strings.Contains(from, "@sha256:") {
			report.violate(file, "copy_from_unknown", fmt.Sprintf("line %d: COPY --from=%s names neither a stage of this file nor a digest-pinned image", instruction.Line, from))
		}
	}
	// The last operand is the destination, so only the sources are asked
	// whether they name the build context itself.
	for _, source := range operands[:max(len(operands)-1, 0)] {
		if isContextRoot(source) {
			report.violate(file, "copy_context_root", fmt.Sprintf("line %d: COPY %s takes the whole build context; copy the paths the stage reads", instruction.Line, source))
		}
	}
}

// copyOperands separates the flags from the sources and destination of a COPY.
func copyOperands(args string) (string, []string) {
	var from string
	var operands []string
	for _, field := range strings.Fields(args) {
		switch {
		case strings.HasPrefix(field, "--from="):
			from = strings.TrimPrefix(field, "--from=")
		case strings.HasPrefix(field, "--"):
			// --chown, --chmod, --link and friends say nothing about what is
			// copied from where.
		default:
			operands = append(operands, field)
		}
	}
	return from, operands
}

func isContextRoot(operand string) bool {
	cleaned := cleanPath(operand)
	return cleaned == "" || cleaned == "."
}

func isStageIndex(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func atoi(value string) int {
	result := 0
	for _, character := range value {
		result = result*10 + int(character-'0')
	}
	return result
}

func auditDeclaredName(report *Report, file string, instruction instruction) {
	// Both instructions declare names, and both spell them two ways: as
	// assignments (`ENV A=1 B=2`) and as a name followed by its value
	// (`ENV NAME value`). Only the first token can be that bare name — a
	// name after it would be a value.
	for index, field := range strings.Fields(instruction.Args) {
		name := field
		if earlier, _, assigned := strings.Cut(field, "="); assigned {
			name = earlier
		} else if index != 0 {
			continue
		}
		if fragment := secretShapedName(name); fragment != "" {
			report.violate(file, "secret_name", fmt.Sprintf("line %d: %s declares %s, whose name carries %q; a build argument is written into a layer", instruction.Line, instruction.Name, name, fragment))
		}
	}
}

// auditRuntimeUser requires the last stage to say who it runs as. The value is
// checked here and the image config is checked again after the build, because
// the recipe can be right while the base image's own user default is not.
func auditRuntimeUser(report *Report, file string, final stage) {
	user := ""
	line := 0
	for _, instruction := range final.Instructions {
		if instruction.Name == "USER" {
			user = instruction.Args
			line = instruction.Line
		}
	}
	if user == "" {
		report.violate(file, "stage_user", fmt.Sprintf("line %d: the runtime stage declares no USER; an image that does not name its user runs as root", final.Line))
		return
	}
	if isRootUser(user) {
		report.violate(file, "stage_user", fmt.Sprintf("line %d: USER %s runs the container as root", line, user))
	}
}

// isRootUser reports whether a USER value resolves to the root account. A name
// that is not `root` and a numeric id that is not 0 are taken as non-root:
// resolving an arbitrary name against /etc/passwd of an image this gate has not
// mounted would guess, and guessing is what the image config check is for.
func isRootUser(value string) bool {
	name, _, _ := strings.Cut(strings.TrimSpace(value), ":")
	switch name {
	case "", "root", "0":
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// The build context: .dockerignore
// ---------------------------------------------------------------------------

// requiredExclusions are the paths the context must not offer to any COPY. Each
// one is a file that would be wrong in an image for a different reason —
// a credential, the Git history, a dependency tree, a stale build artifact, the
// local plan — and `auditDockerignore` requires a pattern that excludes it.
var requiredExclusions = []string{
	".env",
	".env.local",
	".git/config",
	".local/MASTER_PLAN.md",
	"node_modules/typescript/package.json",
	"web/dist/manifest.json",
	"web/generated/pages/auth.js",
}

func auditDockerignore(report *Report, file string) error {
	source, err := os.ReadFile(file)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read .dockerignore: %w", err)
		}
		report.violate(file, "dockerignore_missing", "the build context has no .dockerignore: every file of the working tree is offered to the build")
		return nil
	}
	patterns := parseIgnorePatterns(string(source))
	for _, target := range requiredExclusions {
		if !contextExcludes(patterns, target) {
			report.violate(file, "dockerignore_coverage", fmt.Sprintf("%s is not excluded: a COPY could reach it", target))
		}
	}
	return nil
}

func parseIgnorePatterns(source string) []string {
	var patterns []string
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	return patterns
}

// contextExcludes answers whether .dockerignore would keep a path out of the
// context. Docker's matching is more forgiving than path.Match, so the checks
// here are the three shapes a person writes: an exact path, a directory that
// takes everything under it, and a glob that matches the whole path or one of
// its segments. The last pattern to match decides, which is how a `!` line
// re-includes something an earlier line excluded.
func contextExcludes(patterns []string, target string) bool {
	target = cleanPath(target)
	excluded := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		if matchesIgnorePattern(strings.TrimPrefix(pattern, "!"), target) {
			excluded = !negated
		}
	}
	return excluded
}

func matchesIgnorePattern(pattern, target string) bool {
	pattern = cleanPath(pattern)
	if pattern == "" || target == "" {
		return false
	}
	// `**/name` matches the name at any depth.
	if rest, found := strings.CutPrefix(pattern, "**/"); found {
		return matchesIgnorePattern(rest, target)
	}
	if pattern == target {
		return true
	}
	if strings.HasPrefix(target, pattern+"/") {
		return true
	}
	segments := strings.Split(target, "/")
	for index := range segments {
		prefix := strings.Join(segments[:index+1], "/")
		if matched, _ := path.Match(pattern, prefix); matched {
			return true
		}
	}
	if !strings.Contains(pattern, "/") {
		for _, segment := range segments {
			if matched, _ := path.Match(pattern, segment); matched {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The artifact: image config and exported filesystem
// ---------------------------------------------------------------------------

type imageConfig struct {
	Config struct {
		User       string            `json:"User"`
		Env        []string          `json:"Env"`
		Entrypoint []string          `json:"Entrypoint"`
		Cmd        []string          `json:"Cmd"`
		WorkingDir string            `json:"WorkingDir"`
		Labels     map[string]string `json:"Labels"`
	} `json:"Config"`
	Architecture string `json:"Architecture"`
	Os           string `json:"Os"`
}

func readImageConfig(file string) (imageConfig, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return imageConfig{}, fmt.Errorf("read image config: %w", err)
	}
	var list []imageConfig
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		return list[0], nil
	}
	var single imageConfig
	if err := json.Unmarshal(data, &single); err != nil {
		return imageConfig{}, fmt.Errorf("parse image config %s: %w", file, err)
	}
	if single.Config.User == "" && len(single.Config.Entrypoint) == 0 {
		return imageConfig{}, fmt.Errorf("parse image config %s: the document describes no image (no user and no entrypoint)", file)
	}
	return single, nil
}

func auditImageConfig(report *Report, file string, config imageConfig, present map[string]bool) {
	report.User = config.Config.User
	if config.Config.User == "" {
		report.violate(file, "image_user", "the image declares no user: the process would run as root")
	} else if isRootUser(config.Config.User) {
		report.violate(file, "image_user", fmt.Sprintf("the image runs as %s, which is root", config.Config.User))
	}

	for _, entry := range config.Config.Env {
		name, _, _ := strings.Cut(entry, "=")
		if fragment := secretShapedName(name); fragment != "" {
			report.violate(file, "image_env", fmt.Sprintf("the image declares %s, whose name carries %q: a credential baked into an image is readable by anyone who pulls it", name, fragment))
		}
	}

	if len(config.Config.Entrypoint) == 0 {
		report.violate(file, "image_entrypoint", "the image declares no entrypoint")
		return
	}
	entrypoint := config.Config.Entrypoint[0]
	report.Entrypoint = entrypoint
	if !strings.HasPrefix(entrypoint, "/") {
		report.violate(file, "image_entrypoint", fmt.Sprintf("the entrypoint %q is not an absolute path", entrypoint))
		return
	}
	if !present[cleanPath(entrypoint)] {
		report.violate(file, "entrypoint_missing", fmt.Sprintf("the entrypoint %q does not exist in the filesystem of the image", entrypoint))
	}
}

// The runtime may not carry anything that turns a defect into a shell: a
// compiler, an interpreter or a shell itself. The check is limited to the
// directories a program lives in, so a data file that happens to share a tool's
// name is not mistaken for the tool.
var (
	toolDirectories = []string{"bin", "sbin", "usr/bin", "usr/sbin", "usr/local/bin", "usr/local/sbin"}
	toolNames       = []string{
		"node", "npm", "npx", "yarn", "pnpm", "corepack", "bun", "deno",
		"go", "gofmt", "gcc", "cc", "c++", "g++", "clang", "make", "ld",
		"git", "python", "python3", "pip", "pip3", "perl", "ruby", "php",
		"sh", "bash", "dash", "ash", "busybox", "cargo", "rustc", "java",
		"dotnet", "curl", "wget", "ssh", "nc",
	}
	cacheDirectories = []string{
		"usr/local/go", "go/pkg", "root/.cache", "root/.npm", "root/.yarn",
		"root/.cargo", "home/nonroot/.cache", "home/nonroot/.npm",
		"usr/lib/node_modules", "usr/local/lib/node_modules",
		"var/cache/apt", "var/lib/apt/lists", "var/cache/dnf", "var/cache/yum",
	}
	sourceDirectories = []string{"web/src", "src"}
	sourceSuffixes    = []string{".ts", ".tsx", ".go", ".sql", ".mod", ".sum"}
)

// credentialMarkers are the strings whose presence in a delivered file means a
// credential was baked into the image. They are the values this repository can
// produce by accident: the development DSN, a payment provider key, a private
// key. The list cannot be complete — nothing can prove the absence of an
// unknown secret — so it is documented as what it is: a check for the known
// shapes, and one more reason no credential may enter the context at all.
var credentialMarkers = []struct {
	Marker string
	Detail string
}{
	{"arena-local-dev", "the development database credential"},
	{"ARENA_STRIPE_SECRET_KEY=", "a payment provider key assignment"},
	{"sk_test_", "a payment provider test key"},
	{"sk_live_", "a payment provider live key"},
	{"resend_", "an email provider key"},
	{"-----BEGIN PRIVATE KEY-----", "a private key"},
	{"-----BEGIN RSA PRIVATE KEY-----", "a private key"},
	{"-----BEGIN OPENSSH PRIVATE KEY-----", "a private key"},
	{"-----BEGIN EC PRIVATE KEY-----", "a private key"},
}

// auditRootfs reads the exported filesystem and returns the set of paths it
// holds, which the entrypoint rule needs. Content is scanned while streaming:
// the archive of an image with a Go binary in it is tens of megabytes, and
// holding it in memory to look at it twice would be a measurement with a cost
// nobody asked for.
func auditRootfs(report *Report, file string) (map[string]bool, error) {
	handle, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("read exported filesystem: %w", err)
	}
	defer handle.Close()

	present := map[string]bool{}
	reader := tar.NewReader(handle)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read exported filesystem %s: %w", file, err)
		}

		name := cleanPath(header.Name)
		if name == "" {
			continue
		}
		report.RootfsFiles++
		present[name] = true
		report.violate(file, rootfsPathRule(name), rootfsPathDetail(name))

		if header.Typeflag != tar.TypeReg || header.Size <= 0 {
			continue
		}
		report.RootfsBytes += header.Size
		if header.Size > maxScannedBytes {
			report.violate(file, "rootfs_unscanned", fmt.Sprintf("%s is %d bytes, above the %d-byte scan bound: a credential in it would not have been looked for", name, header.Size, maxScannedBytes))
			continue
		}
		content, err := io.ReadAll(io.LimitReader(reader, header.Size))
		if err != nil {
			return nil, fmt.Errorf("read %s from the exported filesystem: %w", name, err)
		}
		for _, marker := range credentialMarkers {
			if strings.Contains(string(content), marker.Marker) {
				report.violate(file, "rootfs_secret", fmt.Sprintf("%s contains %q (%s)", name, marker.Marker, marker.Detail))
			}
		}
	}
	return present, nil
}

// rootfsPathRule names the rule a path breaks, or "" when it breaks none.
func rootfsPathRule(name string) string {
	segments := strings.Split(name, "/")
	base := segments[len(segments)-1]

	for _, segment := range segments[:len(segments)-1] {
		if segment == ".git" {
			return "rootfs_vcs"
		}
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return "rootfs_env"
	}
	if directoryContains(name, sourceDirectories) {
		return "rootfs_source"
	}
	for _, suffix := range sourceSuffixes {
		if strings.HasSuffix(base, suffix) {
			return "rootfs_source"
		}
	}
	if directoryContains(name, cacheDirectories) || containsSegment(name, "node_modules") {
		return "rootfs_cache"
	}
	if directoryOf(name, toolDirectories) {
		for _, tool := range toolNames {
			if base == tool {
				return "rootfs_tool"
			}
		}
	}
	return ""
}

func rootfsPathDetail(name string) string {
	rule := rootfsPathRule(name)
	switch rule {
	case "":
		return ""
	case "rootfs_vcs":
		return fmt.Sprintf("%s is version control data", name)
	case "rootfs_env":
		return fmt.Sprintf("%s is an environment file", name)
	case "rootfs_source":
		return fmt.Sprintf("%s is a source file: the image delivers built artifacts, not the tree they were built from", name)
	case "rootfs_cache":
		return fmt.Sprintf("%s is a package or build cache", name)
	case "rootfs_tool":
		base := path.Base(name)
		return fmt.Sprintf("%s is a compiler, interpreter or shell: the runtime needs none of them, and each one turns a defect into an interactive session", base)
	default:
		return name
	}
}

func directoryOf(name string, directories []string) bool {
	directory := path.Dir(name)
	for _, candidate := range directories {
		if directory == candidate {
			return true
		}
	}
	return false
}

func directoryContains(name string, directories []string) bool {
	for _, directory := range directories {
		if strings.HasPrefix(name, directory+"/") {
			return true
		}
	}
	return false
}

func containsSegment(name, segment string) bool {
	for _, candidate := range strings.Split(name, "/") {
		if candidate == segment {
			return true
		}
	}
	return false
}

// cleanPath normalizes an archive or ignore path to the form the rules compare
// against: no leading "./", no trailing slash, no double separators.
func cleanPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "./")
	cleaned := path.Clean("/" + value)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

// violate records a violation once. The same path can appear twice in an
// archive written by two layers, and a report that repeats itself invites a
// reader to assume the count means something.
func (report *Report) violate(where, rule, detail string) {
	if detail == "" {
		return
	}
	violation := Violation{Where: where, Rule: rule, Detail: detail}
	for _, existing := range report.Violations {
		if existing == violation {
			return
		}
	}
	report.Violations = append(report.Violations, violation)
}
