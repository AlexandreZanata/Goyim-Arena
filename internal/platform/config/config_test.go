package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func environ(entries ...string) []string {
	return append([]string{
		"HOME=/home/operator",
		"PATH=/usr/bin",
		"OTHER_SERVICE_TOKEN=not-ours",
	}, entries...)
}

// requiredTest covers "ausente" (missing variables) and "default seguro"
// (safe defaults): without any ARENA_* variable the config must be the
// safe development baseline.
func TestLoadWithoutVariablesAppliesSafeDefaults(t *testing.T) {
	t.Parallel()

	config, err := Load(environ())
	if err != nil {
		t.Fatalf("load with no ARENA_* variables: %v", err)
	}
	if config.Env() != EnvDevelopment {
		t.Fatalf("env = %q, want %q", config.Env(), EnvDevelopment)
	}
	if config.Addr() != "127.0.0.1:8080" {
		t.Fatalf("addr = %q, want loopback dev address", config.Addr())
	}
	if config.LogLevel() != LogLevelInfo {
		t.Fatalf("log level = %q, want %q", config.LogLevel(), LogLevelInfo)
	}
	if config.DatabaseURL().IsSet() {
		t.Fatal("database url should be unset without configuration")
	}
	if config.IsProduction() {
		t.Fatal("defaults must never be production")
	}
}

func TestLoadReadsProvidedVariables(t *testing.T) {
	t.Parallel()

	config, err := Load(environ(
		"ARENA_ENV=production",
		"ARENA_ADDR=0.0.0.0:443",
		"ARENA_DATABASE_URL=postgres://arena:secret@db.internal:5432/arena",
		"ARENA_LOG_LEVEL=warn",
	))
	if err != nil {
		t.Fatalf("load valid production variables: %v", err)
	}
	if config.Env() != EnvProduction || !config.IsProduction() {
		t.Fatalf("env = %q, want production", config.Env())
	}
	if config.Addr() != "0.0.0.0:443" {
		t.Fatalf("addr = %q", config.Addr())
	}
	if config.LogLevel() != LogLevelWarn {
		t.Fatalf("log level = %q", config.LogLevel())
	}
	if !config.DatabaseURL().IsSet() {
		t.Fatal("database url should be set")
	}
}

// invalidCases covers "inválido": each bad variable must produce a named,
// accumulated problem — never a panic and never a partial Config.
func TestLoadAccumulatesAllProblems(t *testing.T) {
	t.Parallel()

	_, err := Load(environ(
		"ARENA_ENV=staging",
		"ARENA_ADDR=no-port",
		"ARENA_DATABASE_URL=mysql://wrong:scheme@db:5432/arena",
		"ARENA_LOG_LEVEL=verbose",
		"ARENA_TYPO_VAR=1",
	))
	if err == nil {
		t.Fatal("expected accumulated validation errors")
	}

	var validationErrors ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error must be ValidationErrors, got %T: %v", err, err)
	}

	byVariable := make(map[string]string, len(validationErrors))
	for _, validationError := range validationErrors {
		byVariable[validationError.Variable] = validationError.Problem
	}
	for _, variable := range []string{
		"ARENA_ENV", "ARENA_ADDR", "ARENA_DATABASE_URL", "ARENA_LOG_LEVEL", "ARENA_TYPO_VAR",
	} {
		if _, ok := byVariable[variable]; !ok {
			t.Errorf("missing accumulated problem for %s in:\n%v", variable, err)
		}
	}
	if len(validationErrors) != 5 {
		t.Errorf("accumulated %d problems, want 5:\n%v", len(validationErrors), err)
	}
}

func TestLoadRejectsInvalidPorts(t *testing.T) {
	t.Parallel()

	for _, badAddr := range []string{"127.0.0.1:0", "127.0.0.1:99999", "127.0.0.1:http", ":8080"} {
		_, err := Load(environ("ARENA_ADDR=" + badAddr))
		if err == nil {
			t.Errorf("addr %q must be rejected", badAddr)
			continue
		}
		if !strings.Contains(err.Error(), "ARENA_ADDR") {
			t.Errorf("addr %q: error must name the variable: %v", badAddr, err)
		}
	}
}

// insecureProduction covers "produção insegura": production without the
// required secret must fail loudly; development without it keeps working.
func TestLoadRejectsInsecureProduction(t *testing.T) {
	t.Parallel()

	_, err := Load(environ("ARENA_ENV=production"))
	if err == nil {
		t.Fatal("production without ARENA_DATABASE_URL must fail")
	}
	if !strings.Contains(err.Error(), "ARENA_DATABASE_URL") ||
		!strings.Contains(err.Error(), "required when ARENA_ENV=production") {
		t.Fatalf("error must name the missing production requirement: %v", err)
	}
}

func TestLoadAcceptsProductionWithSecrets(t *testing.T) {
	t.Parallel()

	config, err := Load(environ(
		"ARENA_ENV=production",
		"ARENA_DATABASE_URL=postgres://arena:secret@db.internal:5432/arena",
	))
	if err != nil {
		t.Fatalf("load secure production: %v", err)
	}
	if !config.IsProduction() || !config.DatabaseURL().IsSet() {
		t.Fatal("secure production should load cleanly")
	}
}

// redaction covers "redaction": neither the Config, nor the Secret, nor any
// error path may print the raw secret value.
func TestConfigAndSecretsNeverPrintRawValues(t *testing.T) {
	t.Parallel()

	const secretDSN = "postgres://arena:super-secret-password@db.internal:5432/arena"

	config, err := Load(environ("ARENA_ENV=production", "ARENA_DATABASE_URL="+secretDSN))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	redactedPaths := []string{
		fmt.Sprintf("%v", config),
		fmt.Sprintf("%s", config),
		fmt.Sprintf("%v", config.DatabaseURL()),
		fmt.Sprintf("%s", config.DatabaseURL()),
		fmt.Sprintf("%q", config.DatabaseURL()),
		fmt.Sprintf("%+v", config.DatabaseURL()),
		fmt.Sprintf("%#v", config.DatabaseURL()),
	}
	for index, rendered := range redactedPaths {
		if strings.Contains(rendered, "super-secret-password") {
			t.Errorf("rendered path %d leaked the secret: %q", index, rendered)
		}
	}
	if !strings.Contains(redactedPaths[3], "[REDACTED]") {
		t.Errorf("secret String() should be explicitly redacted, got %q", redactedPaths[3])
	}

	// The explicit hand-off must still expose the exact value.
	if got := string(config.DatabaseURL().Unredacted()); got != secretDSN {
		t.Errorf("Unredacted() = %q, want the exact configured DSN", got)
	}

	// Secret values never reach validation error messages either.
	_, loadErr := Load(environ("ARENA_ENV=production", "ARENA_DATABASE_URL=postgres://user:topsecret@h/x", "ARENA_LOG_LEVEL=bogus"))
	if loadErr == nil {
		t.Fatal("expected unrelated validation error")
	}
	if strings.Contains(loadErr.Error(), "topsecret") {
		t.Errorf("validation error leaked the secret: %v", loadErr)
	}
}

func TestValidationErrorsFormat(t *testing.T) {
	t.Parallel()

	_, err := Load(environ("ARENA_ENV=production"))
	var validationErrors ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("error must be ValidationErrors, got %T", err)
	}
	rendered := validationErrors.Error()
	if !strings.HasPrefix(rendered, "invalid configuration (1 problem(s)):") {
		t.Errorf("summary header missing: %q", rendered)
	}
	if !strings.Contains(rendered, "- ARENA_DATABASE_URL") {
		t.Errorf("named variable missing: %q", rendered)
	}
}

func TestMustLoadPanicsOnInvalidEnvironment(t *testing.T) {
	// t.Setenv cannot combine with t.Parallel, so this test stays serial.
	t.Setenv("ARENA_ENV", "nonsense")

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("MustLoad must panic on invalid environment")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("panic value must be an error, got %T", recovered)
		}
		if !strings.Contains(err.Error(), "ARENA_ENV") {
			t.Errorf("panic error must name the variable: %v", err)
		}
	}()

	MustLoad()
}
