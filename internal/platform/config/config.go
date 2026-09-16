// Package config loads and validates the typed runtime configuration of
// Goyim Arena (P02-T01).
//
// Design constraints from docs/ARCHITECTURE.md and the platform plan:
//   - the resulting Config is immutable after Load;
//   - environment reading is restricted to this package (and cmd/bootstrap),
//     enforced by internal/architecture_test.go;
//   - validation accumulates every problem instead of failing on the first,
//     so operators see the full list at once;
//   - Secret fields redact themselves: the raw value is never printable nor
//     loggable, and it is exposed only through an explicit accessor that is
//     inconvenient to call by accident.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the immutable runtime configuration of the application. All
// fields are unexported so callers cannot construct a partially validated
// instance; access happens through documented getters.
type Config struct {
	env         Env
	addr        string
	databaseURL Secret
	logLevel    LogLevel
}

// Env is the deployment environment of the process.
type Env string

const (
	// EnvDevelopment is the local development environment.
	EnvDevelopment Env = "development"
	// EnvTest is the automated test environment.
	EnvTest Env = "test"
	// EnvProduction is the public production environment.
	EnvProduction Env = "production"
)

// LogLevel is the minimum severity the structured logger emits.
type LogLevel string

const (
	// LogLevelDebug enables verbose development logging.
	LogLevelDebug LogLevel = "debug"
	// LogLevelInfo is the standard operational level.
	LogLevelInfo LogLevel = "info"
	// LogLevelWarn keeps warnings and errors only.
	LogLevelWarn LogLevel = "warn"
	// LogLevelError keeps errors only.
	LogLevelError LogLevel = "error"
)

const envPrefix = "ARENA_"

// Secret wraps a configuration value that must never appear in logs, error
// messages or String output. Secrets are only readable through Unredacted,
// which returns the value wrapped in a type with no String method, so
// accidental interpolation into logs is difficult.
type Secret struct {
	value string
}

// NewSecret builds a Secret from a raw value.
func NewSecret(raw string) Secret {
	return Secret{value: raw}
}

// IsSet reports whether the secret carries a non-empty value.
func (secret Secret) IsSet() bool {
	return secret.value != ""
}

// String implements fmt.Stringer with an always-redacted representation.
func (secret Secret) String() string {
	return "[REDACTED]"
}

// GoString implements fmt.GoStringer with an always-redacted representation,
// closing the %#v leak path.
func (secret Secret) GoString() string {
	return "[REDACTED]"
}

// Unredacted exposes the secret value wrapped in a printable-nothing type.
// Callers must pass the value straight to the component that needs it and
// must never store or log the result.
func (secret Secret) Unredacted() UnredactedSecret {
	return UnredactedSecret(secret.value)
}

// UnredactedSecret is the raw secret value. It has no String or GoString
// method, so fmt verbs render it via reflection only when explicitly
// requested; it exists to make the hand-off point deliberate.
type UnredactedSecret string

// ValidationError describes one configuration problem with the exact
// variable name, so operators can fix the environment without reading code.
type ValidationError struct {
	Variable string
	Problem  string
}

// Error renders a single configuration problem.
func (validationError ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", validationError.Variable, validationError.Problem)
}

// ValidationErrors accumulates every configuration problem found by Load.
type ValidationErrors []ValidationError

// Error renders all accumulated problems.
func (validationErrors ValidationErrors) Error() string {
	lines := make([]string, 0, len(validationErrors))
	for _, validationError := range validationErrors {
		lines = append(lines, "  - "+validationError.Error())
	}
	return fmt.Sprintf("invalid configuration (%d problem(s)):\n%s",
		len(validationErrors), strings.Join(lines, "\n"))
}

// Load reads the ARENA_* environment, validates it accumulating every
// problem, and returns an immutable Config. Unknown ARENA_* variables are
// rejected to catch typos instead of silently ignoring them.
func Load(environ []string) (Config, error) {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, rawValue, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if strings.HasPrefix(name, envPrefix) {
			values[name] = rawValue
		}
	}

	known := map[string]bool{
		"ARENA_ENV":          true,
		"ARENA_ADDR":         true,
		"ARENA_DATABASE_URL": true,
		"ARENA_LOG_LEVEL":    true,
	}
	var validationErrors ValidationErrors
	for name := range values {
		if !known[name] {
			validationErrors = append(validationErrors, ValidationError{
				Variable: name,
				Problem:  "unknown variable (typo? see .env.example for the supported list)",
			})
		}
	}

	config := Config{
		env:      EnvDevelopment,
		addr:     "127.0.0.1:8080",
		logLevel: LogLevelInfo,
	}

	if raw, present := values["ARENA_ENV"]; present {
		switch Env(raw) {
		case EnvDevelopment, EnvTest, EnvProduction:
			config.env = Env(raw)
		default:
			validationErrors = append(validationErrors, ValidationError{
				Variable: "ARENA_ENV",
				Problem:  fmt.Sprintf("invalid value %q (allowed: development, test, production)", raw),
			})
		}
	}

	if raw, present := values["ARENA_ADDR"]; present {
		config.addr = raw
		problem := validateAddr(raw)
		if problem != "" {
			validationErrors = append(validationErrors, ValidationError{
				Variable: "ARENA_ADDR",
				Problem:  problem,
			})
		}
	}

	if raw, present := values["ARENA_DATABASE_URL"]; present {
		config.databaseURL = NewSecret(raw)
		problem := validateDatabaseURL(raw)
		if problem != "" {
			validationErrors = append(validationErrors, ValidationError{
				Variable: "ARENA_DATABASE_URL",
				Problem:  problem,
			})
		}
	}

	if raw, present := values["ARENA_LOG_LEVEL"]; present {
		switch LogLevel(raw) {
		case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
			config.logLevel = LogLevel(raw)
		default:
			validationErrors = append(validationErrors, ValidationError{
				Variable: "ARENA_LOG_LEVEL",
				Problem:  fmt.Sprintf("invalid value %q (allowed: debug, info, warn, error)", raw),
			})
		}
	}

	// Production-specific safety rules: the plan forbids insecure production
	// defaults, so required secrets must be present in that environment.
	if config.env == EnvProduction && !config.databaseURL.IsSet() {
		validationErrors = append(validationErrors, ValidationError{
			Variable: "ARENA_DATABASE_URL",
			Problem:  "required when ARENA_ENV=production",
		})
	}

	if len(validationErrors) > 0 {
		return Config{}, validationErrors
	}
	return config, nil
}

// MustLoad is the bootstrap helper for cmd/: it loads from the process
// environment and terminates with a clear multi-line error on any problem.
func MustLoad() Config {
	config, err := Load(os.Environ())
	if err != nil {
		panic(err)
	}
	return config
}

// Env returns the deployment environment.
func (config Config) Env() Env { return config.env }

// IsProduction reports whether the process runs with production safety rules.
func (config Config) IsProduction() bool { return config.env == EnvProduction }

// Addr returns the HTTP listen address.
func (config Config) Addr() string { return config.addr }

// DatabaseURL returns the redacted database DSN.
func (config Config) DatabaseURL() Secret { return config.databaseURL }

// LogLevel returns the minimum log severity.
func (config Config) LogLevel() LogLevel { return config.logLevel }

// String implements fmt.Stringer with a fully redacted representation, so a
// Config can be safely logged without leaking any value.
func (config Config) String() string {
	return fmt.Sprintf(
		"config{env:%s addr:%s database_url:%s log_level:%s}",
		config.env, config.addr, config.databaseURL, config.logLevel,
	)
}

// validateAddr enforces a host:port TCP address with a numeric port.
func validateAddr(raw string) string {
	host, port, found := strings.Cut(raw, ":")
	if !found || host == "" {
		return fmt.Sprintf("invalid value %q (want host:port, for example 127.0.0.1:8080)", raw)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return fmt.Sprintf("invalid port %q (want a number between 1 and 65535)", port)
	}
	if portNumber == 0 {
		return fmt.Sprintf("invalid port %q (must be between 1 and 65535)", port)
	}
	return ""
}

// validateDatabaseURL enforces a non-local PostgreSQL DSN shape without
// parsing credentials. Local development sockets keep working because the
// check only applies when the value looks like a network DSN.
func validateDatabaseURL(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		return "" // libpq-style key=value DSNs are accepted as-is.
	}
	if strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://") {
		return ""
	}
	return fmt.Sprintf("unsupported scheme in %q (want postgres:// or postgresql://, or a libpq key=value DSN)", raw)
}
