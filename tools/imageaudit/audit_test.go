package main

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests of this gate are written the way the gate is: each rule is proven
// by a fixture that breaks exactly it, and the fixture of a passing image is
// the one every case mutates. A rule whose case cannot be made to fail would
// not be a rule — it would be a comment that happens to compile.

const validDockerignore = `# fixture: the required exclusions
.git
.local
.env
.env.*
node_modules
**/node_modules
web/dist
web/generated
`

const validConfig = `[{"Config":{"User":"65532:65532",
  "Env":["PATH=/usr/bin","ARENA_ADDR=0.0.0.0:8080","ARENA_ASSETS_DIR=/web/dist"],
  "Entrypoint":["/arena"],"Cmd":["server"],"WorkingDir":"/home/nonroot"},
  "Architecture":"amd64","Os":"linux"}]`

// rootfsEntry is one file of an exported filesystem. `filler` writes that many
// zero bytes instead of `content`, so a file above the scan bound can be built
// without holding it in memory.
type rootfsEntry struct {
	name    string
	content string
	mode    int64
	filler  int64
}

// runtimeRootfs is the shape the audit requires of the image: the binary, the
// asset build it references, and the base image's own files.
func runtimeRootfs() []rootfsEntry {
	return []rootfsEntry{
		{name: "arena", content: "\x7fELF…the binary", mode: 0o755},
		{name: "web/dist/manifest.json", content: `{"version":1,"assets":{}}`},
		{name: "web/dist/pages/auth-1234abcd.js", content: "export{}"},
		{name: "etc/passwd", content: "nonroot:x:65532:65532::/home/nonroot:/sbin/nologin"},
		{name: "usr/share/zoneinfo/UTC", content: "TZif"},
		{name: "usr/bin/.keep", content: ""},
	}
}

func writeFile(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("prepare %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func writeRootfs(t *testing.T, path string, entries []rootfsEntry) string {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()

	writer := tar.NewWriter(file)
	for _, entry := range entries {
		size := int64(len(entry.content))
		if entry.filler > 0 {
			size = entry.filler
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{Name: entry.name, Mode: mode, Size: size, Typeflag: tar.TypeReg}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write header of %s: %v", entry.name, err)
		}
		if entry.filler > 0 {
			if _, err := io.CopyN(writer, zeroReader{}, entry.filler); err != nil {
				t.Fatalf("write filler of %s: %v", entry.name, err)
			}
		} else if _, err := writer.Write([]byte(entry.content)); err != nil {
			t.Fatalf("write %s: %v", entry.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive %s: %v", path, err)
	}
	return path
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for index := range p {
		p[index] = 0
	}
	return len(p), nil
}

// fixture writes one complete set of artifacts into a temporary directory and
// returns the options that read them. `mutate` rewrites whichever artifact a
// case is about; the rest stay exactly as the approved image has them.
func fixture(t *testing.T, mutate func(directory string)) Options {
	t.Helper()
	directory := t.TempDir()
	path := func(name string) string { return filepath.Join(directory, name) }

	writeFile(t, directory, "Dockerfile", readFixture(t, "Dockerfile.valid"))
	writeFile(t, directory, ".dockerignore", validDockerignore)
	writeFile(t, directory, "config.json", validConfig)
	writeRootfs(t, path("rootfs.tar"), runtimeRootfs())
	if mutate != nil {
		mutate(directory)
	}
	return Options{
		Dockerfile:   path("Dockerfile"),
		Dockerignore: path(".dockerignore"),
		Config:       path("config.json"),
		Rootfs:       path("rootfs.tar"),
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(content)
}

func auditFixture(t *testing.T, options Options) Report {
	t.Helper()
	report, err := Audit(options)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	return report
}

// rewrite replaces a fragment of a file the fixture wrote.
func rewrite(t *testing.T, path, old, replacement string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(content), old) {
		t.Fatalf("the fixture of %s does not contain %q: the mutation would change nothing", path, old)
	}
	updated := strings.Replace(string(content), old, replacement, 1)
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestAuditApprovesAHardenedImage(t *testing.T) {
	report := auditFixture(t, fixture(t, nil))
	if len(report.Violations) > 0 {
		t.Fatalf("the approved fixture broke rules: %v", report.Violations)
	}
	if report.User != "65532:65532" {
		t.Errorf("user = %q, want the non-root user of the fixture", report.User)
	}
	if report.Entrypoint != "/arena" {
		t.Errorf("entrypoint = %q, want /arena", report.Entrypoint)
	}
	if report.RootfsFiles == 0 {
		t.Error("the exported filesystem reported no paths: the archive was not read")
	}
}

// TestEveryRuleIsFalsified is the proof that the gate measures what it claims:
// one mutation per rule, each one breaking exactly the rule it names.
func TestEveryRuleIsFalsified(t *testing.T) {
	cases := []struct {
		rule   string
		why    string
		mutate func(t *testing.T, directory string)
	}{
		{
			rule: "from_pinned",
			why:  "a base identified by a tag",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "@sha256:0e0ff40c39bc087845bfb27465a0df4ea419520094bc35842ff83dd8cbe6f9b6", "")
			},
		},
		{
			rule: "from_malformed",
			why:  "a FROM that names no image",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"),
					"FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime",
					"FROM")
			},
		},
		{
			rule: "stage_duplicate",
			why:  "two stages answering to the same name",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "AS runtime", "AS web")
			},
		},
		{
			rule: "copy_from_unknown",
			why:  "a COPY reading a stage that does not exist",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "--from=build", "--from=elsewhere")
			},
		},
		{
			rule: "copy_context_root",
			why:  "a COPY that takes the whole build context",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "COPY cmd ./cmd", "COPY . ./src")
			},
		},
		{
			rule: "add_instruction",
			why:  "ADD, which can fetch a remote URL",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "COPY cmd ./cmd", "ADD https://example.invalid/toolchain.tar.gz /tmp/")
			},
		},
		{
			rule: "secret_name",
			why:  "a secret passed as a build argument",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "ARG VERSION=dev", "ARG ARENA_STRIPE_SECRET_KEY")
			},
		},
		{
			rule: "stage_user",
			why:  "a runtime stage that declares root",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "Dockerfile"), "USER 65532:65532", "USER root")
			},
		},
		{
			rule: "dockerignore_missing",
			why:  "a context with no .dockerignore",
			mutate: func(t *testing.T, directory string) {
				if err := os.Remove(filepath.Join(directory, ".dockerignore")); err != nil {
					t.Fatalf("remove .dockerignore: %v", err)
				}
			},
		},
		{
			rule: "dockerignore_coverage",
			why:  "a context that still offers the environment file",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, ".dockerignore"), ".env.*", ".env.example")
			},
		},
		{
			rule: "image_user",
			why:  "an image whose user is root",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "config.json"), `"User":"65532:65532"`, `"User":"0"`)
			},
		},
		{
			rule: "image_env",
			why:  "a credential in the environment of the image",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "config.json"), `"ARENA_ASSETS_DIR=/web/dist"`, `"ARENA_STRIPE_SECRET_KEY=sk_live_x"`)
			},
		},
		{
			rule: "image_entrypoint",
			why:  "an image that declares no entrypoint",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "config.json"), `"Entrypoint":["/arena"]`, `"Entrypoint":[]`)
			},
		},
		{
			rule: "entrypoint_missing",
			why:  "an entrypoint that is not in the filesystem",
			mutate: func(t *testing.T, directory string) {
				rewrite(t, filepath.Join(directory, "config.json"), `["/arena"]`, `["/arena-server"]`)
			},
		},
		{
			rule: "rootfs_tool",
			why:  "a runtime that carries Node",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "usr/local/bin/node", content: "\x7fELF", mode: 0o755})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_tool",
			why:  "a runtime that carries a shell",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "bin/sh", content: "\x7fELF", mode: 0o755})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_cache",
			why:  "a runtime that carries the package cache of a build",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "root/.npm/_cacache/index-v5/de/ad/entry", content: "{}"})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_env",
			why:  "a runtime that carries an environment file",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "app/.env", content: "ARENA_ENV=production"})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_vcs",
			why:  "a runtime that carries the Git history",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: ".git/config", content: "[core]"})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_source",
			why:  "a runtime that carries the sources instead of the build",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "web/src/pages/auth.ts", content: "export {}"})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_secret",
			why:  "a credential baked into a delivered file",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "etc/arena.conf", content: "dsn=postgres://arena:arena-local-dev@db/arena"})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_secret",
			why:  "a private key baked into a delivered file",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "etc/tls/private.key", content: "-----BEGIN PRIVATE KEY-----\nMIIE"})
				writeRootfs(t, path, entries)
			},
		},
		{
			rule: "rootfs_unscanned",
			why:  "a file too large for the content scan",
			mutate: func(t *testing.T, directory string) {
				path := filepath.Join(directory, "rootfs.tar")
				entries := append(runtimeRootfs(), rootfsEntry{name: "var/lib/blob", filler: maxScannedBytes + 1})
				writeRootfs(t, path, entries)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.rule+": "+testCase.why, func(t *testing.T) {
			report := auditFixture(t, fixture(t, func(directory string) { testCase.mutate(t, directory) }))

			rules := map[string]int{}
			for _, violation := range report.Violations {
				rules[violation.Rule]++
			}
			if rules[testCase.rule] == 0 {
				t.Fatalf("the mutation broke no %s rule: %v", testCase.rule, report.Violations)
			}
			for rule := range rules {
				if rule != testCase.rule {
					t.Errorf("the mutation also broke %s: %v", rule, report.Violations)
				}
			}
		})
	}
}

// TestAuditRefusesToMeasureWithoutTheArtifacts is the fail-closed half: a gate
// that cannot read the image has approved nothing, so it must not report a
// clean result.
func TestAuditRefusesToMeasureWithoutTheArtifacts(t *testing.T) {
	cases := []struct {
		name  string
		strip func(options *Options)
	}{
		{name: "no image config", strip: func(options *Options) { options.Config = filepath.Join(t.TempDir(), "absent.json") }},
		{name: "no exported filesystem", strip: func(options *Options) { options.Rootfs = filepath.Join(t.TempDir(), "absent.tar") }},
		{name: "no Dockerfile", strip: func(options *Options) { options.Dockerfile = filepath.Join(t.TempDir(), "absent") }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			options := fixture(t, nil)
			testCase.strip(&options)
			if _, err := Audit(options); err == nil {
				t.Fatal("Audit approved an image it could not read")
			}
		})
	}
}

func TestAuditRejectsAConfigThatDescribesNoImage(t *testing.T) {
	options := fixture(t, func(directory string) {
		writeFile(t, directory, "config.json", "{}")
	})
	if _, err := Audit(options); err == nil {
		t.Fatal("Audit accepted a config document that carries no image")
	}
}

// TestContextExcludesStates which patterns keep a path out of the context. The
// rule is only as good as its reading of .dockerignore, and this is where that
// reading is pinned.
func TestContextExcludes(t *testing.T) {
	patterns := parseIgnorePatterns(validDockerignore)

	excluded := []string{
		".env",
		".env.local",
		".git/config",
		".local/PROGRESS.md",
		"node_modules/typescript/package.json",
		"tools/e2e/node_modules/playwright/package.json",
		"web/dist/manifest.json",
		"web/generated/pages/auth.js",
	}
	for _, target := range excluded {
		if !contextExcludes(patterns, target) {
			t.Errorf("%s is not excluded by the fixture's patterns", target)
		}
	}

	included := []string{
		"web/src/pages/auth.ts",
		"cmd/arena/main.go",
		"internal/platform/assets/manifest.json",
	}
	for _, target := range included {
		if contextExcludes(patterns, target) {
			t.Errorf("%s is excluded by the fixture's patterns, and the build needs it", target)
		}
	}

	reIncluded := parseIgnorePatterns("node_modules\n!node_modules/keep.js\n")
	if !contextExcludes(reIncluded, "node_modules/drop.js") {
		t.Error("a directory pattern did not exclude a file under it")
	}
	if contextExcludes(reIncluded, "node_modules/keep.js") {
		t.Error("a negation did not re-include the file it names")
	}
}

func TestParseStagesJoinsContinuationsAndFindsNames(t *testing.T) {
	stages, err := parseStages(readFixture(t, "Dockerfile.valid"))
	if err != nil {
		t.Fatalf("parseStages: %v", err)
	}
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, stage.Name)
	}
	if strings.Join(names, ",") != "web,build,runtime" {
		t.Fatalf("stage names = %v, want web, build, runtime", names)
	}

	// A continuation belongs to the line it started on, which is the line a
	// violation must name — otherwise the report points at a fragment.
	joined, err := parseStages("FROM alpine@sha256:0000000000000000000000000000000000000000000000000000000000000000 AS a\nRUN set -eu \\\n  && echo one \\\n  && echo two\nUSER 1000\n")
	if err != nil {
		t.Fatalf("parseStages: %v", err)
	}
	for _, instruction := range joined[0].Instructions {
		if instruction.Name == "RUN" {
			if instruction.Line != 2 {
				t.Errorf("RUN is reported at line %d, want 2", instruction.Line)
			}
			if !strings.Contains(instruction.Args, "echo two") {
				t.Errorf("RUN args = %q, want the continuation joined", instruction.Args)
			}
		}
	}
}

func TestParseStagesRejectsAFileWithoutFrom(t *testing.T) {
	if _, err := parseStages("# only comments\nRUN echo hi\n"); err == nil {
		t.Fatal("parseStages accepted a file that describes no image")
	}
}
