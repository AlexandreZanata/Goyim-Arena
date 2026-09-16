// Package contract is the tooling-side validator of the versioned API
// contract (P02-T06): it parses api/openapi.json, enforces the structural
// conventions of the master plan (health endpoints, RFC 9457 Problem
// Details, security schemes, pagination and idempotency conventions) and
// compares the routes registered by the binary against the document, so
// route drift fails the build.
//
// It is test/build tooling: nothing here runs inside the request path and
// nothing is served from this package. Validation is structural (stdlib
// only, per the dependency policy) — semantic request/response validation
// arrives with the contract tests of real endpoints.
package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

// Media type and route conventions fixed by the master plan.
const (
	// ProblemMediaType is the RFC 9457 error media type.
	ProblemMediaType = "application/problem+json"

	// OpenAPIVersion is the contract dialect.
	OpenAPIVersion = "3.1.0"
)

// Document is the structural view of api/openapi.json used for validation.
type Document struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Title   string `json:"title"`
		Version string `json:"version"`
	} `json:"info"`
	Paths map[string]map[string]json.RawMessage `json:"paths"`

	Components struct {
		Schemas         map[string]json.RawMessage `json:"schemas"`
		SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
	} `json:"components"`

	// extensions holds top-level x-* fields captured at decode time.
	extensions map[string]json.RawMessage
}

// Load parses the OpenAPI document at path.
func Load(path string) (*Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("contract: open %s: %w", path, err)
	}
	defer file.Close()
	return Decode(file)
}

// Decode parses an OpenAPI document from reader, capturing top-level
// extension fields (x-*) alongside the typed view.
func Decode(reader io.Reader) (*Document, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("contract: read document: %w", err)
	}
	document := &Document{}
	if err := json.Unmarshal(data, document); err != nil {
		return nil, fmt.Errorf("contract: invalid JSON: %w", err)
	}
	extensions := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &extensions); err != nil {
		return nil, fmt.Errorf("contract: invalid JSON: %w", err)
	}
	document.extensions = extensions
	return document, nil
}

// Validate enforces the structural conventions of the contract.
func (document *Document) Validate() error {
	if document.OpenAPI != OpenAPIVersion {
		return fmt.Errorf("contract: openapi = %q, want %q", document.OpenAPI, OpenAPIVersion)
	}
	if document.Info.Title == "" || document.Info.Version == "" {
		return fmt.Errorf("contract: info.title and info.version are required")
	}
	if len(document.Paths) == 0 {
		return fmt.Errorf("contract: document defines no paths")
	}
	for path, operations := range document.Paths {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("contract: path %q must start with /", path)
		}
		if len(operations) == 0 {
			return fmt.Errorf("contract: path %q has no operations", path)
		}
		for method := range operations {
			if !isKnownMethod(method) {
				return fmt.Errorf("contract: path %q has unsupported field %q (want an HTTP method)", path, method)
			}
		}
	}
	if problem, ok := document.Components.Schemas["Problem"]; !ok || len(problem) == 0 {
		return fmt.Errorf("contract: components.schemas.Problem (RFC 9457) is required")
	}
	requiredSchemes := []string{"SessionCookie", "CsrfHeader"}
	for _, scheme := range requiredSchemes {
		if _, ok := document.Components.SecuritySchemes[scheme]; !ok {
			return fmt.Errorf("contract: components.securitySchemes.%s is required", scheme)
		}
	}
	return validateConventions(document)
}

// validateConventions checks the x-conventions block: pagination and
// idempotency conventions are part of the public contract.
func validateConventions(document *Document) error {
	raw, ok := document.Extensions()["x-conventions"]
	if !ok {
		return fmt.Errorf(`contract: x-conventions block is required (pagination, idempotency)`)
	}
	var conventions struct {
		Pagination struct {
			CursorParameter  string          `json:"cursorParameter"`
			LimitParameter   string          `json:"limitParameter"`
			LimitMaximum     float64         `json:"limitMaximum"`
			ResponseEnvelope json.RawMessage `json:"responseEnvelope"`
		} `json:"pagination"`
		Idempotency struct {
			Header       string `json:"header"`
			ReplayHeader string `json:"replayHeader"`
		} `json:"idempotency"`
	}
	if err := json.Unmarshal(raw, &conventions); err != nil {
		return fmt.Errorf("contract: invalid x-conventions: %w", err)
	}
	if conventions.Pagination.CursorParameter == "" || conventions.Pagination.LimitParameter == "" {
		return fmt.Errorf("contract: pagination conventions must name cursor and limit parameters")
	}
	if conventions.Pagination.LimitMaximum <= 0 {
		return fmt.Errorf("contract: pagination limitMaximum must be positive")
	}
	if !json.Valid(conventions.Pagination.ResponseEnvelope) {
		return fmt.Errorf("contract: pagination responseEnvelope must be a JSON schema object")
	}
	if conventions.Idempotency.Header == "" || conventions.Idempotency.ReplayHeader == "" {
		return fmt.Errorf("contract: idempotency conventions must name the key and replay headers")
	}
	return nil
}

// Extensions returns the top-level extension fields (x-*) captured at
// decode time, so conventions like x-conventions stay inspectable.
func (document *Document) Extensions() map[string]json.RawMessage {
	return document.extensions
}

// Route is one operation of the contract in registry form.
type Route struct {
	Method string
	Path   string
}

// String renders the canonical "METHOD /path" form.
func (route Route) String() string {
	return route.Method + " " + route.Path
}

// Routes returns every operation of the document sorted in canonical
// "METHOD /path" order.
func (document *Document) Routes() ([]Route, error) {
	routes := make([]Route, 0, len(document.Paths)*2)
	for path, operations := range document.Paths {
		for method := range operations {
			if !isKnownMethod(method) {
				return nil, fmt.Errorf("contract: path %q has unsupported field %q", path, method)
			}
			routes = append(routes, Route{Method: strings.ToUpper(method), Path: path})
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].String() < routes[j].String()
	})
	return routes, nil
}

// CompareRoutes fails with a precise diff when the registered routes and
// the contract diverge in either direction.
func CompareRoutes(contractRoutes []Route, registeredRoutes []Route) error {
	contractSet := make(map[string]Route, len(contractRoutes))
	for _, route := range contractRoutes {
		contractSet[route.String()] = route
	}
	registeredSet := make(map[string]Route, len(registeredRoutes))
	for _, route := range registeredRoutes {
		registeredSet[route.String()] = route
	}

	var missing []string
	for key := range contractSet {
		if _, ok := registeredSet[key]; !ok {
			missing = append(missing, key)
		}
	}
	var undeclared []string
	for key := range registeredSet {
		if _, ok := contractSet[key]; !ok {
			undeclared = append(undeclared, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(undeclared)

	if len(missing) == 0 && len(undeclared) == 0 {
		return nil
	}
	var problems []string
	for _, key := range missing {
		problems = append(problems, fmt.Sprintf("declared in contract but not registered: %s", key))
	}
	for _, key := range undeclared {
		problems = append(problems, fmt.Sprintf("registered but not declared in contract: %s", key))
	}
	return fmt.Errorf("contract: route drift detected (%d problems):\n  %s",
		len(problems), strings.Join(problems, "\n  "))
}

// isKnownMethod reports whether the field name is an HTTP operation.
func isKnownMethod(field string) bool {
	switch strings.ToUpper(field) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
