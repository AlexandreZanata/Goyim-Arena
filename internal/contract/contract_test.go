// Tests of internal/contract (P02-T06): the api/openapi.json document must
// parse, satisfy the structural conventions of the master plan and match
// the routes the arena binary actually registers. Drift in either
// direction fails the build.
package contract_test

import (
	"strings"
	"testing"

	"github.com/AlexandreZanata/Goyim-Arena/internal/contract"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
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
		t.Errorf("contract declares %s but only health endpoints are implemented in this stage", route.String())
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
