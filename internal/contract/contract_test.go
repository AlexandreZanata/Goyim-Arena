// Tests of internal/contract (P02-T06): the api/openapi.json document must
// parse, satisfy the structural conventions of the master plan and match
// the routes the arena binary actually registers. Drift in either
// direction fails the build.
package contract_test

import (
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/AlexandreZanata/Goyim-Arena/internal/arenas/adapters/html"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/arenas/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/arguments/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/billing/adapters/http"
	"github.com/AlexandreZanata/Goyim-Arena/internal/contract"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/identity/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/moderation/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/persuasion/adapters/http"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/positions/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/profiles/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/transparency/adapters/http"
	_ "github.com/AlexandreZanata/Goyim-Arena/internal/wallet/adapters/http"
)

// repoRoot locates the checkout root from this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	return "../.."
}

// loadContract parses and validates the real contract document.
func loadContract(t *testing.T) *contract.Document {
	t.Helper()
	document, err := contract.Load(repoRoot(t) + "/api/openapi.json")
	if err != nil {
		t.Fatalf("load contract: %v", err)
	}
	if err := document.Validate(); err != nil {
		t.Fatalf("contract invalid: %v", err)
	}
	return document
}

func TestContractParsesAndSatisfiesConventions(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	if document.OpenAPI != contract.OpenAPIVersion {
		t.Errorf("openapi = %q, want %q", document.OpenAPI, contract.OpenAPIVersion)
	}
	if document.Info.Title == "" {
		t.Error("info.title must not be empty")
	}
	scheme, ok := document.Extensions()["x-conventions"]
	if !ok || len(scheme) == 0 {
		t.Fatal("x-conventions block is missing")
	}
}

func TestContractRoutesMatchRegisteredRoutes(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	contractRoutes, err := document.Routes()
	if err != nil {
		t.Fatalf("document routes: %v", err)
	}
	registered := httpserver.RegisteredRoutes()
	registeredRoutes := make([]contract.Route, 0, len(registered))
	for _, route := range registered {
		registeredRoutes = append(registeredRoutes, contract.Route{Method: route.Method, Path: route.Path})
	}

	if err := contract.CompareRoutes(contractRoutes, registeredRoutes); err != nil {
		t.Fatalf("route drift: %v", err)
	}

	if len(contractRoutes) == 0 {
		t.Fatal("contract declares no routes; the comparison would pass vacuously")
	}
	for _, route := range contractRoutes {
		if route.Path == "/health/live" || route.Path == "/health/ready" {
			continue
		}
		if strings.HasPrefix(route.Path, "/api/v1/auth/") {
			continue
		}
		if strings.HasPrefix(route.Path, "/api/v1/profiles/") || route.Path == "/api/v1/me/profile" {
			continue
		}
		if route.Path == "/api/v1/me/wallet" || route.Path == "/api/v1/me/wallet/transactions" {
			continue
		}
		if route.Path == "/api/v1/me/passes" || route.Path == "/api/v1/me/passes/history" {
			continue
		}
		if route.Path == "/api/v1/me/billing/checkout" || route.Path == "/api/v1/me/billing/subscription" || route.Path == "/api/v1/me/billing/portal" {
			continue
		}
		if route.Path == "/api/v1/me/moderation/reports" || route.Path == "/api/v1/me/moderation/appeals" || route.Path == "/api/v1/moderation/cases" || route.Path == "/api/v1/moderation/cases/{id}/claim" || route.Path == "/api/v1/moderation/cases/{id}/decisions" {
			continue
		}
		if strings.HasPrefix(route.Path, "/api/v1/me/arenas") || route.Path == "/api/v1/arenas" || strings.HasPrefix(route.Path, "/api/v1/arenas/") {
			continue
		}
		if route.Path == "/d/{slug}" {
			continue
		}
		if route.Path == "/api/v1/public/transparency" || route.Path == "/transparency" {
			continue
		}
		if strings.HasPrefix(route.Path, "/api/v1/me/arguments") || strings.HasPrefix(route.Path, "/api/v1/arguments") {
			continue
		}
		if strings.HasPrefix(route.Path, "/api/v1/me/position-changes") || strings.HasPrefix(route.Path, "/api/v1/profiles/{username}/reputation") {
			continue
		}
		if strings.HasPrefix(route.Path, "/api/v1/moderation/attribution-signals") {
			continue
		}
		t.Errorf("contract declares %s but it is not implemented in this stage", route.String())
	}
}

// TestContractPassSchemasExposeOnlyAllowedFields is the contract-level proof
// of P07-T06: the pass summary and history documents declare exactly the
// allowed properties, no internal or antifraud data, and both routes require
// the session cookie.
func TestContractPassSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"ArenaPassLot":          {"origin", "quantity", "remaining", "expires_at", "expired", "created_at"},
		"ArenaPassSummary":      {"available_total", "checked_at", "lots"},
		"ArenaPassHistoryEntry": {"consumption_id", "arena_id", "origin", "reference", "consumed_at"},
		"ArenaPassHistory":      {"items", "next_cursor"},
	}
	forbiddenMarkers := []string{
		"email", "account_id", "password", "credential", "lot_id",
		"stripe", "customer", "billing", "payment", "fraud", "admin",
		"reason", "actor", "notes", "ip", "user_agent",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			for _, marker := range forbiddenMarkers {
				if strings.Contains(strings.ToLower(property), marker) {
					t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	for _, path := range []string{"/api/v1/me/passes", "/api/v1/me/passes/history"} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		if !strings.Contains(string(operations["get"]), `"SessionCookie"`) {
			t.Errorf("%s must require the SessionCookie scheme", path)
		}
	}
}

// TestContractWalletSchemasExposeOnlyAllowedFields is the contract-level
// proof of P06-T08: the wallet balance and statement documents declare
// exactly the allowed properties, no restricted administrative or antifraud
// data, and both routes require the session cookie.
func TestContractWalletSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"WalletBalance":        {"balance_free", "balance_purchased"},
		"WalletStatementEntry": {"transaction_id", "operation_id", "operation_type", "bucket", "amount", "reference", "created_at"},
		"WalletStatement":      {"items", "next_cursor"},
	}
	forbiddenMarkers := []string{
		"reason", "actor", "email", "account_id", "password", "credential",
		"stripe", "customer", "billing", "payment", "fraud", "admin",
		"notes", "ip", "user_agent",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			for _, marker := range forbiddenMarkers {
				if strings.Contains(strings.ToLower(property), marker) {
					t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	entryRaw := document.Components.Schemas["WalletStatementEntry"]
	if !strings.Contains(string(entryRaw), `"integer"`) {
		t.Error("WalletStatementEntry.amount must be an integer")
	}
	for _, path := range []string{"/api/v1/me/wallet", "/api/v1/me/wallet/transactions"} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		if !strings.Contains(string(operations["get"]), `"SessionCookie"`) {
			t.Errorf("%s must require the SessionCookie scheme", path)
		}
	}
}

// TestContractProfileSchemasExposeOnlyAllowedFields is the contract-level
// leak proof of P05-T04: the public and private profile documents declare
// exactly the allowed properties, and no schema declares forbidden markers
// (email, credentials, payment identifiers, antifraud flags or
// administrative notes).
func TestContractProfileSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"PublicProfile":  {"username", "interface_locale", "created_at"},
		"PrivateProfile": {"username", "interface_locale", "created_at", "updated_at"},
	}
	forbiddenMarkers := []string{
		"email", "account_id", "password", "credential", "hash",
		"stripe", "customer", "billing", "payment",
		"fraud", "admin", "notes", "ip", "user_agent",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			for _, marker := range forbiddenMarkers {
				if strings.Contains(strings.ToLower(property), marker) {
					t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	privatePath, ok := document.Paths["/api/v1/me/profile"]
	if !ok {
		t.Fatal("contract is missing /api/v1/me/profile")
	}
	if !strings.Contains(string(privatePath["get"]), `"SessionCookie"`) {
		t.Error("private profile operation must require the SessionCookie scheme")
	}
}

// TestContractArenaSchemasExposeOnlyAllowedFields is the contract-level proof
// of P08-T07: private and public arena documents declare exactly the allowed
// properties, no schema declares forbidden markers (email, credentials,
// payment identifiers, antifraud flags or administrative notes), the public
// statuses never include draft or removed, authenticated routes require the
// session cookie and the public reads document the ETag revalidation.
func TestContractArenaSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"ArenaDraftRequest":       {"statement", "context", "category", "language"},
		"ArenaDraftUpdateRequest": {"statement", "context", "category", "language", "expected_version"},
		"PrivateArena":            {"id", "slug", "statement", "context", "category", "language", "status", "version", "created_at", "published_at", "closes_at"},
		"PrivateArenaList":        {"items"},
		"PublicArenaSummary":      {"id", "slug", "statement", "category", "language", "status", "published_at", "closes_at"},
		"PublicArena":             {"id", "slug", "statement", "context", "category", "language", "status", "published_at", "closes_at"},
		"ArenaFeed":               {"items", "next_cursor"},
	}
	forbiddenMarkers := []string{
		"email", "account_id", "password", "credential", "creator",
		"stripe", "customer", "billing", "payment", "fraud", "admin",
		"reason", "actor", "notes", "ip", "user_agent",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			for _, marker := range forbiddenMarkers {
				if strings.Contains(strings.ToLower(property), marker) {
					t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	// The public status enums never include non-public states.
	for _, name := range []string{"PublicArenaSummary", "PublicArena"} {
		raw := document.Components.Schemas[name]
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		status, ok := schema.Properties["status"]
		if !ok {
			t.Fatalf("%s is missing the status property", name)
		}
		if strings.Join(status.Enum, ",") != "published,closed,restricted" {
			t.Errorf("%s.status enum = %v, want only published, closed and restricted", name, status.Enum)
		}
	}

	// Authenticated arena routes require the session cookie.
	for _, path := range []string{
		"/api/v1/me/arenas/drafts",
		"/api/v1/me/arenas/drafts/{id}",
		"/api/v1/me/arenas/drafts/{id}/publish",
		"/api/v1/me/arenas/{id}/close",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		for method, operation := range operations {
			if !strings.Contains(string(operation), `"SessionCookie"`) {
				t.Errorf("%s %s must require the SessionCookie scheme", strings.ToUpper(method), path)
			}
		}
	}

	// The public reads document the ETag revalidation and the public cache
	// policy; the private draft list stays no-store.
	for _, path := range []string{"/api/v1/arenas", "/api/v1/arenas/{slug}"} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations["get"])
		if strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("%s must stay public", path)
		}
		for _, marker := range []string{"ETag", "public, max-age=60", `"304"`, "If-None-Match"} {
			if !strings.Contains(operation, marker) {
				t.Errorf("%s operation must document %q", path, marker)
			}
		}
	}
}

// TestContractArenaDocumentRoute is the contract-level proof of P08-T08:
// the HTML document route is public, answers HTML, documents the ETag
// revalidation and distinguishes removed (410) from not found (404).
func TestContractArenaDocumentRoute(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	operations, ok := document.Paths["/d/{slug}"]
	if !ok {
		t.Fatal("contract is missing /d/{slug}")
	}
	operation := string(operations["get"])
	if strings.Contains(operation, `"SessionCookie"`) {
		t.Error("/d/{slug} must stay public")
	}
	for _, marker := range []string{
		"text/html", "ETag", "public, max-age=60", `"304"`, "If-None-Match", `"410"`, `"404"`,
	} {
		if !strings.Contains(operation, marker) {
			t.Errorf("/d/{slug} operation must document %q", marker)
		}
	}
}

// TestContractPositionSchemasExposeOnlyAllowedFields is the contract-level
// proof of P09-T06: private and public position documents declare exactly
// the allowed properties, no schema declares forbidden markers, the private
// routes require the session cookie, the public aggregate stays public with
// its ETag revalidation and the position vocabulary is closed.
func TestContractPositionSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"PositionRequest":       {"position"},
		"PrivatePosition":       {"arena_id", "initial_position", "current_position", "version", "created_at", "updated_at"},
		"PositionConfirmation":  {"position", "replayed"},
		"PositionChangeRecord":  {"change_id", "position"},
		"PositionChangeEntry":   {"change_id", "from_position", "to_position", "version", "changed_at"},
		"PositionChangeHistory": {"items"},
		"PositionDistribution":  {"agree", "disagree", "undecided"},
		"PositionAggregate":     {"participants_total", "suppressed", "initial", "current", "checked_at"},
	}
	forbiddenMarkers := []string{
		"account", "email", "password", "credential", "creator", "user",
		"stripe", "customer", "billing", "payment", "fraud", "admin",
		"reason", "actor", "notes", "ip", "user_agent",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			// Snake-case tokens compare exactly: "participants_total" must
			// not match the "ip" marker.
			for _, token := range strings.Split(strings.ToLower(property), "_") {
				for _, marker := range forbiddenMarkers {
					if token == marker {
						t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
					}
				}
			}
		}
	}

	// The position vocabulary is closed in every schema that carries it.
	for _, name := range []string{"PositionRequest", "PrivatePosition", "PositionChangeEntry"} {
		raw := string(document.Components.Schemas[name])
		if !strings.Contains(raw, `"agree"`) || !strings.Contains(raw, `"disagree"`) || !strings.Contains(raw, `"undecided"`) {
			t.Errorf("%s must declare the closed position vocabulary", name)
		}
		for _, forbidden := range []string{`"maybe"`, `"neutral"`, `"yes"`, `"no"`} {
			if strings.Contains(raw, forbidden) {
				t.Errorf("%s declares a position outside the vocabulary: %s", name, forbidden)
			}
		}
	}

	// Private routes require the session cookie; the aggregate stays public.
	for _, path := range []string{
		"/api/v1/me/arenas/{id}/position",
		"/api/v1/me/arenas/{id}/position/changes",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		for method, operation := range operations {
			if !strings.Contains(string(operation), `"SessionCookie"`) {
				t.Errorf("%s %s must require the SessionCookie scheme", strings.ToUpper(method), path)
			}
		}
	}

	operations, ok := document.Paths["/api/v1/arenas/{id}/positions"]
	if !ok {
		t.Fatal("contract is missing /api/v1/arenas/{id}/positions")
	}
	aggregateOperation := string(operations["get"])
	if strings.Contains(aggregateOperation, `"SessionCookie"`) {
		t.Error("the aggregate must stay public")
	}
	for _, marker := range []string{"ETag", "public, max-age=60", `"304"`, "If-None-Match", "suppressed"} {
		if !strings.Contains(aggregateOperation, marker) {
			t.Errorf("aggregate operation must document %q", marker)
		}
	}
}

// TestContractArgumentSchemasExposeOnlyAllowedFields is the contract-level
// proof of P10-T08: the argument documents declare exactly the allowed
// properties, no schema declares forbidden markers (account identifiers,
// credentials, payment data or moderation notes), the private routes
// require the session cookie and the idempotency header, and the public
// reads stay cacheable with ETag revalidation.
func TestContractArgumentSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"ArgumentPublishRequest": {"relation", "content", "sources"},
		"ArgumentSource":         {"url", "description"},
		"Argument":               {"id", "arena_id", "parent_id", "relation", "content", "status", "created_at"},
		"ArgumentListItem":       {"id", "arena_id", "parent_id", "relation", "content", "status", "created_at", "reply_count"},
		"ArgumentPage":           {"items", "next_cursor"},
		"ArgumentMutationResult": {"argument", "replayed"},
	}
	forbiddenMarkers := []string{
		"account", "author", "creator", "email", "user", "password", "credential",
		"stripe", "billing", "payment", "fraud", "admin", "reason", "actor",
		"notes", "ip", "user_agent",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			// Snake-case tokens compare exactly: "reply_count" must not
			// match a short marker by substring.
			for _, token := range strings.Split(strings.ToLower(property), "_") {
				for _, marker := range forbiddenMarkers {
					if token == marker {
						t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
					}
				}
			}
		}
	}

	// The relation vocabulary is closed in every schema that carries it.
	for _, name := range []string{"ArgumentPublishRequest", "Argument", "ArgumentListItem"} {
		raw := string(document.Components.Schemas[name])
		if !strings.Contains(raw, `"support"`) || !strings.Contains(raw, `"oppose"`) || !strings.Contains(raw, `"context"`) {
			t.Errorf("%s must declare the closed relation vocabulary", name)
		}
	}

	// Private routes require the session cookie; the two INK-spending posts
	// also require the idempotency header (withdrawal is idempotent by
	// state and needs no attempt key).
	privatePaths := map[string]bool{
		"/api/v1/me/arenas/{id}/arguments":                      true,
		"/api/v1/me/arenas/{id}/arguments/{argumentID}/replies": true,
		"/api/v1/me/arguments/{id}/withdraw":                    false,
	}
	for path, needsIdempotencyKey := range privatePaths {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations["post"])
		if !strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("POST %s must require the SessionCookie scheme", path)
		}
		if needsIdempotencyKey != strings.Contains(operation, `"Idempotency-Key"`) {
			t.Errorf("POST %s idempotency documentation = %v, want %v", path, !needsIdempotencyKey, needsIdempotencyKey)
		}
	}

	for _, path := range []string{
		"/api/v1/arenas/{id}/arguments",
		"/api/v1/arguments/{id}/replies",
		"/api/v1/arguments/{id}",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations["get"])
		if strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("%s must stay public", path)
		}
		for _, marker := range []string{"ETag", "public, max-age=60", `"304"`, "If-None-Match"} {
			if !strings.Contains(operation, marker) {
				t.Errorf("%s operation must document %q", path, marker)
			}
		}
	}
}

// TestContractPersuasionSchemasExposeOnlyAllowedFields is the contract-level
// proof of P11-T06: the attribution and reputation documents declare exactly
// the allowed properties, no schema declares forbidden markers (attributor or
// account identifiers, email, credentials, moderation or antifraud data), the
// private recording route requires the session cookie and the two public
// counts stay public with their ETag revalidation and no way to list people.
func TestContractPersuasionSchemasExposeOnlyAllowedFields(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"AttributionRecordRequest": {"argument_ids"},
		"AttributionRecordResult":  {"argument_ids", "replayed"},
		"ArgumentAttributionMetrics": {
			"valid_attributions", "distinct_people", "checked_at",
		},
		"ReputationArenaSlice": {
			"arena_id", "category", "language", "distinct_people", "valid_attributions",
		},
		"ReputationDimension": {
			"label", "distinct_people", "valid_attributions",
		},
		"ProfileReputation": {
			"username", "influenced_people", "valid_attributions",
			"arenas", "by_category", "by_language", "checked_at",
		},
	}
	forbiddenMarkers := []string{
		"account", "attributor", "email", "user", "password", "credential",
		"stripe", "billing", "payment", "fraud", "admin", "reason", "actor",
		"notes", "ip", "user_agent", "voter", "score", "rank",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			// Snake-case tokens compare exactly: "influenced_people" must not
			// match a short marker by substring.
			for _, token := range strings.Split(strings.ToLower(property), "_") {
				for _, marker := range forbiddenMarkers {
					if token == marker {
						t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
					}
				}
			}
		}
	}

	// No response document may carry the list of people behind a count: no
	// declared property names an attributor or account identifier, so the
	// counting surface stays counts, dimensions and public identifiers
	// (BR §5.1, REQ-PERS-05).
	for name, declared := range expected {
		for _, property := range declared {
			for _, marker := range []string{"attributor", "account", "email"} {
				if strings.Contains(property, marker) {
					t.Errorf("SECURITY VIOLATION: %s declares %q, which names %q", name, property, marker)
				}
			}
		}
	}

	// The recording route is the only private one and requires the session
	// cookie; a selection is at most three arguments.
	recordOperations, ok := document.Paths["/api/v1/me/position-changes/{id}/attributions"]
	if !ok {
		t.Fatal("contract is missing /api/v1/me/position-changes/{id}/attributions")
	}
	record := string(recordOperations["post"])
	if !strings.Contains(record, `"SessionCookie"`) {
		t.Error("POST /api/v1/me/position-changes/{id}/attributions must require the SessionCookie scheme")
	}
	if !strings.Contains(record, `"Idempotency-Replayed"`) {
		t.Error("the recording route must document the replay header")
	}
	requestRaw, ok := document.Components.Schemas["AttributionRecordRequest"]
	if !ok {
		t.Fatal("components.schemas.AttributionRecordRequest is missing")
	}
	if !strings.Contains(string(requestRaw), `"maxItems": 3`) {
		t.Error("AttributionRecordRequest.argument_ids must cap the selection at three arguments")
	}

	// The public counts stay public, cacheable and revalidatable.
	for _, path := range []string{
		"/api/v1/arguments/{id}/attributions",
		"/api/v1/profiles/{username}/reputation",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations["get"])
		if strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("%s must stay public", path)
		}
		for _, marker := range []string{"ETag", "public, max-age=60", `"304"`, "If-None-Match"} {
			if !strings.Contains(operation, marker) {
				t.Errorf("%s operation must document %q", path, marker)
			}
		}
	}
}

// TestContractAbuseSignalSurfaceStaysRestricted is the contract-level proof of
// P11-T07: the signal documents declare exactly the allowed properties, no
// property carries a score, severity, weight or rank, the restricted route
// requires the session cookie and the mandatory private cache policy, and no
// public persuasion document declares a signal or a score.
func TestContractAbuseSignalSurfaceStaysRestricted(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"AttributionSignal": {
			"kind", "counterpart_id", "mutual_events", "dominant_events",
			"share_basis_points", "changes", "reversals",
		},
		"AttributionSignals": {
			"author_id", "policy_version", "window_seconds", "checked_at", "signals",
		},
	}
	// Signals are advisory: nothing may carry a ranking or an automatic
	// consequence (MODERATION §5, §10).
	forbiddenMarkers := []string{
		"score", "severity", "weight", "rank", "penalty", "action",
		"block", "ban", "suspension", "probability", "confidence",
	}

	for name, expectedProperties := range expected {
		raw, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("components.schemas.%s is missing", name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("decode %s schema: %v", name, err)
		}
		if len(schema.Properties) != len(expectedProperties) {
			t.Fatalf("%s declares %d properties, want exactly %d", name, len(schema.Properties), len(expectedProperties))
		}
		for _, property := range expectedProperties {
			if _, ok := schema.Properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range schema.Properties {
			for _, token := range strings.Split(strings.ToLower(property), "_") {
				for _, marker := range forbiddenMarkers {
					if token == marker {
						t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
					}
				}
			}
		}
	}

	// The signal vocabulary is closed and carries the three documented kinds.
	kindRaw := string(document.Components.Schemas["AttributionSignal"])
	for _, kind := range []string{"reciprocity", "concentration", "rapid_alternation"} {
		if !strings.Contains(kindRaw, `"`+kind+`"`) {
			t.Errorf("AttributionSignal must declare the %q kind", kind)
		}
	}

	// The restricted route is authenticated and carries the private policy.
	operations, ok := document.Paths["/api/v1/moderation/attribution-signals/{authorID}"]
	if !ok {
		t.Fatal("contract is missing /api/v1/moderation/attribution-signals/{authorID}")
	}
	operation := string(operations["get"])
	if !strings.Contains(operation, `"SessionCookie"`) {
		t.Error("the signal route must require the SessionCookie scheme")
	}
	if !strings.Contains(operation, "private, no-store") {
		t.Error("the signal route must document the private cache policy")
	}
	if !strings.Contains(operation, `"403"`) {
		t.Error("the signal route must document the forbidden answer for unauthorized callers")
	}
	if strings.Contains(operation, "public, max-age") {
		t.Error("the signal route must never be publicly cacheable")
	}

	// No public persuasion document exposes a signal or a score.
	for name := range map[string]bool{
		"AttributionRecordRequest":   true,
		"AttributionRecordResult":    true,
		"ArgumentAttributionMetrics": true,
		"ReputationArenaSlice":       true,
		"ReputationDimension":        true,
		"ProfileReputation":          true,
	} {
		for property := range propertiesOf(t, document, name) {
			for _, marker := range []string{"signal", "score", "abuse", "weight", "rank", "severity"} {
				if strings.Contains(strings.ToLower(property), marker) {
					t.Errorf("SECURITY VIOLATION: public schema %s declares %q", name, property)
				}
			}
		}
	}
}

// propertiesOf decodes the declared properties of one schema.
func propertiesOf(t *testing.T, document *contract.Document, name string) map[string]json.RawMessage {
	t.Helper()
	raw, ok := document.Components.Schemas[name]
	if !ok {
		t.Fatalf("components.schemas.%s is missing", name)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode %s schema: %v", name, err)
	}
	return schema.Properties
}

func TestContractBillingPrivateAPIStaysPrivate(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"BillingCheckoutRequest": {"market", "product", "idempotency_key"},
		"BillingCheckout":        {"intent_id", "status", "redirect_url", "amount_minor", "currency", "market", "product", "replayed"},
		"BillingSubscription":    {"has_subscription", "status", "product", "market", "current_period_end", "cancel_at_period_end"},
		"BillingPortalRequest":   {"idempotency_key"},
		"BillingPortal":          {"portal_url"},
	}
	forbiddenMarkers := []string{
		"stripe", "customer", "session", "subscription_id", "payment",
		"price", "secret", "email", "account_id", "fraud", "admin",
	}

	for name, expectedProperties := range expected {
		properties := propertiesOf(t, document, name)
		for _, property := range expectedProperties {
			if _, ok := properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		for property := range properties {
			lowered := strings.ToLower(property)
			for _, marker := range forbiddenMarkers {
				if strings.Contains(lowered, marker) && property != "has_subscription" {
					t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	for path, method := range map[string]string{
		"/api/v1/me/billing/checkout":     "post",
		"/api/v1/me/billing/subscription": "get",
		"/api/v1/me/billing/portal":       "post",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations[method])
		if !strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("%s must require the SessionCookie scheme", path)
		}
		if !strings.Contains(operation, "private, no-store") {
			t.Errorf("%s must document the private cache policy", path)
		}
		if strings.Contains(operation, "public, max-age") {
			t.Errorf("%s must never be publicly cacheable", path)
		}
	}
}

func TestContractModerationAPIStaysPrivate(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	expected := map[string][]string{
		"ModerationReportRequest":   {"target_type", "target_id", "reason", "context"},
		"ModerationReport":          {"report_id", "replayed", "rate_limited", "reports_in_window"},
		"ModerationAppealRequest":   {"action_id", "context"},
		"ModerationAppeal":          {"appeal_id", "action_id", "replayed"},
		"ModerationCase":            {"case_id", "target_type", "target_id", "status", "priority", "created_at", "claimed_by"},
		"ModerationCasePage":        {"items", "next_cursor"},
		"ModerationClaim":           {"case_id", "status", "claimed_by"},
		"ModerationDecisionRequest": {"action", "rule", "justification", "expires_at"},
		"ModerationDecision":        {"action_id", "case_id", "action"},
	}
	// Responses (everything except the two request shapes) must never
	// carry restricted evidence: reporter context, justifications, appeal
	// contexts, reporter identities or emails.
	responseForbidden := []string{
		"context", "justification", "reporter", "reason", "email",
		"decision_reason", "rule_applied", "actor",
	}

	for name, expectedProperties := range expected {
		properties := propertiesOf(t, document, name)
		for _, property := range expectedProperties {
			if _, ok := properties[property]; !ok {
				t.Errorf("%s is missing allowed property %q", name, property)
			}
		}
		if strings.HasSuffix(name, "Request") {
			continue
		}
		for property := range properties {
			lowered := strings.ToLower(property)
			for _, marker := range responseForbidden {
				if strings.Contains(lowered, marker) {
					t.Errorf("SECURITY VIOLATION: response schema %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	for path, method := range map[string]string{
		"/api/v1/me/moderation/reports":           "post",
		"/api/v1/me/moderation/appeals":           "post",
		"/api/v1/moderation/cases":                "get",
		"/api/v1/moderation/cases/{id}/claim":     "post",
		"/api/v1/moderation/cases/{id}/decisions": "post",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations[method])
		if !strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("%s must require the SessionCookie scheme", path)
		}
		if !strings.Contains(operation, "private, no-store") {
			t.Errorf("%s must document the private cache policy", path)
		}
		if strings.Contains(operation, "public, max-age") {
			t.Errorf("%s must never be publicly cacheable", path)
		}
	}
}

func TestContractTransparencyStaysPublicAndPrivate(t *testing.T) {
	t.Parallel()

	document := loadContract(t)

	metrics := propertiesOf(t, document, "TransparencyMetricCounts")
	wantCounts := []string{
		"eligible_accounts",
		"arenas_published", "arenas_closed", "arenas_restricted", "arenas_removed",
		"arguments_published", "arguments_withdrawn",
		"position_changes",
		"attributions_valid", "attributions_invalidated", "influenced_authors",
		"ink_free_granted", "ink_free_expired", "ink_free_consumed",
		"ink_purchased_granted", "ink_purchased_consumed", "ink_refunded", "ink_admin_adjusted",
		"passes_purchase_granted", "passes_member_granted", "passes_consumed",
		"reports_filed", "actions_recorded", "appeals_filed", "appeals_reversed",
	}
	if len(metrics) != len(wantCounts) {
		t.Fatalf("TransparencyMetricCounts declares %d properties, want exactly %d", len(metrics), len(wantCounts))
	}
	for _, property := range wantCounts {
		if _, ok := metrics[property]; !ok {
			t.Errorf("TransparencyMetricCounts is missing %q", property)
		}
	}
	for _, name := range []string{"TransparencyMetrics", "TransparencyMetricCounts"} {
		for property := range propertiesOf(t, document, name) {
			lowered := strings.ToLower(property)
			// Aggregate family nouns (accounts, positions, authors) name
			// counts, never identities: the forbidden set targets
			// identifiers, secrets and per-account markers.
			for _, marker := range []string{"email", "stripe", "cus_", "ip", "attributor", "token", "password", "session"} {
				if strings.Contains(lowered, marker) {
					t.Errorf("SECURITY VIOLATION: %s declares forbidden property %q", name, property)
				}
			}
		}
	}

	for path, method := range map[string]string{
		"/api/v1/public/transparency": "get",
		"/transparency":               "get",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("contract is missing %s", path)
		}
		operation := string(operations[method])
		if strings.Contains(operation, `"SessionCookie"`) {
			t.Errorf("%s must stay public", path)
		}
		if !strings.Contains(operation, "public, max-age=3600") {
			t.Errorf("%s must document the public cache policy", path)
		}
		if !strings.Contains(operation, `"ETag"`) || !strings.Contains(operation, `"304"`) {
			t.Errorf("%s must document ETag revalidation", path)
		}
		if strings.Contains(operation, "no-store") {
			t.Errorf("%s must be cacheable, never no-store", path)
		}
	}
}

func TestCompareRoutesDetectsDriftBothWays(t *testing.T) {
	t.Parallel()

	a := []contract.Route{{Method: "GET", Path: "/health/live"}, {Method: "GET", Path: "/health/ready"}}
	b := []contract.Route{{Method: "GET", Path: "/health/live"}}

	err := contract.CompareRoutes(a, b)
	if err == nil {
		t.Fatal("expected drift when the contract declares an unregistered route")
	}
	if !strings.Contains(err.Error(), "declared in contract but not registered") {
		t.Errorf("error should name the missing registration, got: %v", err)
	}

	err = contract.CompareRoutes(b, a)
	if err == nil {
		t.Fatal("expected drift when a registered route is not declared")
	}
	if !strings.Contains(err.Error(), "registered but not declared in contract") {
		t.Errorf("error should name the undeclared registration, got: %v", err)
	}

	if err := contract.CompareRoutes(a, a); err != nil {
		t.Fatalf("identical routes must not drift: %v", err)
	}
}

func TestValidateRejectsBrokenDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "wrong dialect",
			payload: `{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{}}`,
			want:    `openapi = "3.0.3"`,
		},
		{
			name:    "missing info",
			payload: `{"openapi":"3.1.0","paths":{"/p":{"get":{}}}}`,
			want:    "info.title and info.version are required",
		},
		{
			name:    "no paths",
			payload: `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{}}`,
			want:    "defines no paths",
		},
		{
			name:    "path without slash",
			payload: `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"p":{"get":{}}}}`,
			want:    "must start with /",
		},
		{
			name:    "unknown operation field",
			payload: `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/p":{"fetch":{}}}}`,
			want:    "unsupported field",
		},
		{
			name:    "missing Problem schema",
			payload: `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/p":{"get":{}}},"components":{"securitySchemes":{"SessionCookie":{},"CsrfHeader":{}}}}`,
			want:    "schemas.Problem",
		},
		{
			name:    "missing security scheme",
			payload: `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/p":{"get":{}}},"components":{"schemas":{"Problem":{}},"securitySchemes":{"SessionCookie":{}}}}`,
			want:    "securitySchemes.CsrfHeader",
		},
		{
			name:    "missing conventions",
			payload: `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/p":{"get":{}}},"components":{"schemas":{"Problem":{}},"securitySchemes":{"SessionCookie":{},"CsrfHeader":{}}}}`,
			want:    "x-conventions block is required",
		},
	}

	for _, test := range tests {
		document, err := contract.Decode(strings.NewReader(test.payload))
		if err != nil {
			t.Errorf("%s: decode: %v", test.name, err)
			continue
		}
		err = document.Validate()
		if err == nil {
			t.Errorf("%s: expected validation failure containing %q, got nil", test.name, test.want)
			continue
		}
		if !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: error %q does not contain %q", test.name, err.Error(), test.want)
		}
	}
}

func TestDecodeRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	if _, err := contract.Decode(strings.NewReader("{not json")); err == nil {
		t.Fatal("expected decode error for invalid JSON")
	}
}
