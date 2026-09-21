package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureApplicationImage mirrors the image the fixture pins: the audit is told
// which artifact is the product, it never guesses.
const fixtureApplicationImage = "registry.invalid/goyim-arena@sha256:" + "0000000000000000000000000000000000000000000000000000000000000000"

// committedComposeFile is the artifact this gate exists for. The unit tests
// audit the real file, not a copy of it: a rule that only holds for a fixture
// holds for nothing.
const committedComposeFile = "../../compose.production.yaml"

// renderedFixture reads the rendered document the tests mutate. It is written
// by hand — not captured from a live Docker — because the audit's input is a
// document format, and a fixture that needed a daemon could not run in
// `make test-unit`.
func renderedFixture(t *testing.T) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", "rendered.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return document
}

// rendered runs one mutation and writes the result where the audit can read it.
func rendered(t *testing.T, mutate func(document map[string]any)) string {
	t.Helper()

	document := renderedFixture(t)
	if mutate != nil {
		mutate(document)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode mutated fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "rendered.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write mutated fixture: %v", err)
	}
	return path
}

// serviceOf returns one service of a fixture, failing when it is not there:
// a mutation that silently edits nothing would make its test pass for the wrong
// reason.
func serviceOf(t *testing.T, document map[string]any, name string) map[string]any {
	t.Helper()

	services, ok := document["services"].(map[string]any)
	if !ok {
		t.Fatal("the fixture has no services")
	}
	instance, ok := services[name].(map[string]any)
	if !ok {
		t.Fatalf("the fixture has no service %q", name)
	}
	return instance
}

func environmentOf(t *testing.T, document map[string]any, name string) map[string]any {
	t.Helper()

	environment, ok := serviceOf(t, document, name)["environment"].(map[string]any)
	if !ok {
		t.Fatalf("service %q of the fixture has no environment", name)
	}
	return environment
}

func auditFixture(t *testing.T, documentPath string) Report {
	t.Helper()

	report, err := Audit(Options{
		File:             committedComposeFile,
		Document:         documentPath,
		ApplicationImage: fixtureApplicationImage,
	})
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	return report
}

// TestTheCommittedTopologyPasses is the baseline: the file this repository
// commits, rendered as the fixture renders it, breaks no rule. Every probe
// below is the difference between this run and one broken rule.
func TestTheCommittedTopologyPasses(t *testing.T) {
	t.Parallel()

	report := auditFixture(t, rendered(t, nil))
	if len(report.Violations) > 0 {
		t.Fatalf("the committed topology breaks %d rule(s): %v", len(report.Violations), report.Violations)
	}
	if report.Ingress != "caddy" {
		t.Errorf("ingress = %q, want the service that publishes the ports", report.Ingress)
	}
	if report.Database != "db" {
		t.Errorf("database = %q, want the service running the PostgreSQL image", report.Database)
	}
	if len(report.Services) != 4 {
		t.Errorf("services = %v, want the four roles of docs/DEPLOYMENT.md §1", report.Services)
	}
	if len(report.PublishedPorts) != 2 {
		t.Errorf("published ports = %v, want the ingress's HTTP and HTTPS only", report.PublishedPorts)
	}
}

// TestTheReportNeverPrintsAnEnvironmentValue is the redaction rule of this
// gate. The rendered document has inlined the operator's environment files, so
// it carries real credentials: the report names what is wrong, never what the
// value is. The assertion is not vacuous — the fixture is checked to contain
// the sentinels the report must not.
func TestTheReportNeverPrintsAnEnvironmentValue(t *testing.T) {
	t.Parallel()

	const dsnSentinel = "redaction-sentinel"
	const databaseSentinel = "database-sentinel"

	document := renderedFixture(t)
	if !strings.Contains(mustEncode(t, document), dsnSentinel) {
		t.Fatal("the fixture does not carry the sentinel this test needs")
	}

	// A document that breaks rules produces the fullest possible report.
	path := rendered(t, func(document map[string]any) {
		serviceOf(t, document, "app")["image"] = "goyim-arena:latest"
		environmentOf(t, document, "worker")["POSTGRES_PASSWORD"] = databaseSentinel
		delete(environmentOf(t, document, "app"), "ARENA_RESEND_API_KEY")
	})

	report := auditFixture(t, path)
	if len(report.Violations) == 0 {
		t.Fatal("the mutated document was expected to break rules")
	}

	var out bytes.Buffer
	report.write(&out)
	rendered := out.String()
	for _, sentinel := range []string{dsnSentinel, databaseSentinel} {
		if strings.Contains(rendered, sentinel) {
			t.Errorf("the report leaked %q:\n%s", sentinel, rendered)
		}
	}
	for _, violation := range report.Violations {
		for _, sentinel := range []string{dsnSentinel, databaseSentinel} {
			if strings.Contains(violation.String(), sentinel) {
				t.Errorf("violation %s leaked %q", violation.Rule, sentinel)
			}
		}
	}
}

func mustEncode(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(raw)
}

// TestEachRuleRefusesItsOwnMutation is the gate's metatest: for every rule
// there is a document that breaks exactly it. A rule without such a probe is a
// comment, and the table is what makes the difference between a rule and a
// claim.
func TestEachRuleRefusesItsOwnMutation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		rule   string
		only   bool
		mutate func(document map[string]any)
	}{
		{
			name: "an image that is a tag",
			rule: "image_pinned",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "caddy")["image"] = "caddy:2.10.0-alpine"
			},
		},
		{
			name: "a service that builds",
			rule: "image_build",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "caddy")["build"] = map[string]any{"context": "."}
			},
		},
		{
			name: "a second service publishing a port",
			rule: "ingress_single",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "app")["ports"] = []any{map[string]any{"target": 80.0, "published": "8080", "protocol": "tcp"}}
			},
		},
		{
			name: "a port that is not HTTP",
			rule: "published_port_not_http",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "caddy")["ports"] = []any{map[string]any{"target": 8080.0, "published": "8080", "protocol": "tcp"}}
			},
		},
		{
			name: "the database port published",
			rule: "database_port_exposed",
			mutate: func(document map[string]any) {
				serviceOf(t, document, "caddy")["ports"] = []any{map[string]any{"target": 5432.0, "published": "5432", "protocol": "tcp"}}
			},
		},
		{
			name: "a network that is not declared",
			rule: "network_declared",
			only: true,
			mutate: func(document map[string]any) {
				networks := serviceOf(t, document, "app")["networks"].(map[string]any)
				networks["undeclared"] = nil
			},
		},
		{
			name: "a database network that is not internal",
			rule: "database_internal",
			only: true,
			mutate: func(document map[string]any) {
				networks := document["networks"].(map[string]any)
				delete(networks["internal"].(map[string]any), "internal")
			},
		},
		{
			name: "an ingress sharing the database network",
			rule: "ingress_database_isolation",
			only: true,
			mutate: func(document map[string]any) {
				networks := serviceOf(t, document, "caddy")["networks"].(map[string]any)
				networks["internal"] = nil
			},
		},
		{
			name: "a service with no restart policy",
			rule: "restart_declared",
			only: true,
			mutate: func(document map[string]any) {
				delete(serviceOf(t, document, "worker"), "restart")
			},
		},
		{
			name: "a service with no limits",
			rule: "limits_declared",
			only: true,
			mutate: func(document map[string]any) {
				delete(serviceOf(t, document, "app"), "deploy")
			},
		},
		{
			name: "a service whose log grows without a bound",
			rule: "logging_bounded",
			only: true,
			mutate: func(document map[string]any) {
				delete(serviceOf(t, document, "app"), "logging")
			},
		},
		{
			name: "a logging driver declared with no max-size",
			rule: "logging_bounded",
			only: true,
			mutate: func(document map[string]any) {
				options := serviceOf(t, document, "db")["logging"].(map[string]any)["options"].(map[string]any)
				delete(options, "max-size")
			},
		},
		{
			name: "a third-party service with no healthcheck",
			rule: "healthcheck_present",
			only: true,
			mutate: func(document map[string]any) {
				delete(serviceOf(t, document, "caddy"), "healthcheck")
			},
		},
		{
			name: "an application service that cannot be probed and cannot stop gracefully",
			rule: "healthcheck_present",
			only: true,
			mutate: func(document map[string]any) {
				delete(serviceOf(t, document, "worker"), "stop_grace_period")
			},
		},
		{
			name: "a volume that is not declared",
			rule: "volume_declared",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "db")["volumes"] = []any{map[string]any{"type": "volume", "source": "undeclared", "target": "/var/lib/postgresql"}}
			},
		},
		{
			name: "the application running one role",
			rule: "application_roles",
			mutate: func(document map[string]any) {
				serviceOf(t, document, "app")["image"] = "registry.invalid/something-else@sha256:" + strings.Repeat("a", 64)
			},
		},
		{
			name: "the application serving and executing the same command",
			rule: "application_roles",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "worker")["command"] = []any{"server"}
			},
		},
		{
			name: "an application role with no route out",
			rule: "egress_declared",
			only: true,
			mutate: func(document map[string]any) {
				networks := serviceOf(t, document, "worker")["networks"].(map[string]any)
				delete(networks, "egress")
			},
		},
		{
			name: "the application missing a production requirement",
			rule: "application_environment",
			only: true,
			mutate: func(document map[string]any) {
				delete(environmentOf(t, document, "app"), "ARENA_RESEND_API_KEY")
			},
		},
		{
			name: "the application not in production",
			rule: "application_environment",
			only: true,
			mutate: func(document map[string]any) {
				environmentOf(t, document, "worker")["ARENA_ENV"] = "development"
			},
		},
		{
			name: "the database password in an application service",
			rule: "database_password_isolated",
			only: true,
			mutate: func(document map[string]any) {
				environmentOf(t, document, "worker")["POSTGRES_PASSWORD"] = "database-sentinel"
			},
		},
		{
			name: "a service running something that is not PostgreSQL as the database",
			rule: "database_missing",
			only: true,
			mutate: func(document map[string]any) {
				serviceOf(t, document, "db")["image"] = "docker.io/library/redis@sha256:" + strings.Repeat("b", 64)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report := auditFixture(t, rendered(t, testCase.mutate))
			if !hasRule(report, testCase.rule) {
				t.Fatalf("the mutation broke no %q rule: %v", testCase.rule, report.Violations)
			}
			if testCase.only && len(report.Violations) != 1 {
				t.Fatalf("the mutation breaks %d rule(s), and the probe attributes one: %v", len(report.Violations), report.Violations)
			}
		})
	}
}

func hasRule(report Report, rule string) bool {
	for _, violation := range report.Violations {
		if violation.Rule == rule {
			return true
		}
	}
	return false
}

// TestALiteralCredentialInTheCommittedFileIsRefused covers the one rule that
// reads the file instead of the rendered document: by the time a document is
// rendered, every environment file has been inlined, so the file as committed
// is the only place where "this value is written down here" is visible.
func TestALiteralCredentialInTheCommittedFileIsRefused(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
		rule    bool
	}{
		{
			name:    "a literal database password",
			content: "services:\n  db:\n    environment:\n      POSTGRES_PASSWORD: hunter2\n",
			rule:    true,
		},
		{
			name:    "a literal provider key",
			content: "services:\n  app:\n    environment:\n      ARENA_RESEND_API_KEY: \"re_live_written_down\"\n",
			rule:    true,
		},
		{
			name:    "a literal cursor secret",
			content: "services:\n  app:\n    environment:\n      ARENA_CURSOR_SECRET: a-written-down-signing-key-with-entropy\n",
			rule:    true,
		},
		{
			name:    "an interpolated value",
			content: "services:\n  app:\n    environment:\n      ARENA_RESEND_API_KEY: ${ARENA_RESEND_API_KEY:?}\n",
			rule:    false,
		},
		{
			name:    "a secret named by path",
			content: "secrets:\n  origin_key:\n    file: ${COMPOSE_TLS_KEY_FILE:?}\n",
			rule:    false,
		},
		{
			name:    "a key that only carries the word",
			content: "services:\n  app:\n    environment:\n      ARENA_KEYSTORE_PATH: /web/dist\n",
			rule:    false,
		},
		{
			name:    "a commented example",
			content: "services:\n  db:\n    # POSTGRES_PASSWORD: hunter2\n    image: postgres\n",
			rule:    false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "compose.yaml")
			if err := os.WriteFile(path, []byte(testCase.content), 0o600); err != nil {
				t.Fatalf("write compose file: %v", err)
			}
			report, err := Audit(Options{
				File:             path,
				Document:         rendered(t, nil),
				ApplicationImage: fixtureApplicationImage,
			})
			if err != nil {
				t.Fatalf("Audit() error = %v", err)
			}
			found := hasRule(report, "literal_credential")
			if found != testCase.rule {
				t.Fatalf("literal_credential reported = %v, want %v (violations: %v)", found, testCase.rule, report.Violations)
			}
			for _, violation := range report.Violations {
				if violation.Rule == "literal_credential" && strings.Contains(violation.String(), "hunter2") {
					t.Errorf("the violation quotes the value: %s", violation)
				}
			}
		})
	}
}

// TestTheAuditRefusesToApproveWhatItCannotRead is the fail-closed half: an
// input that cannot be measured is not an input that passed.
func TestTheAuditRefusesToApproveWhatItCannotRead(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "absent.json")
	if _, err := Audit(Options{File: committedComposeFile, Document: missing, ApplicationImage: fixtureApplicationImage}); err == nil {
		t.Error("a missing rendered document was approved")
	}

	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write invalid document: %v", err)
	}
	if _, err := Audit(Options{File: committedComposeFile, Document: invalid, ApplicationImage: fixtureApplicationImage}); err == nil {
		t.Error("a document that is not JSON was approved")
	}

	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(empty, []byte(`{"services":{}}`), 0o600); err != nil {
		t.Fatalf("write empty document: %v", err)
	}
	if _, err := Audit(Options{File: committedComposeFile, Document: empty, ApplicationImage: fixtureApplicationImage}); err == nil {
		t.Error("a document with no service was approved")
	}

	if _, err := Audit(Options{File: filepath.Join(t.TempDir(), "absent.yaml"), Document: rendered(t, nil), ApplicationImage: fixtureApplicationImage}); err == nil {
		t.Error("a missing compose file was approved")
	}
}
