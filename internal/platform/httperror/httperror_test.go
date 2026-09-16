package httperror

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/apperr"
)

// kindCase fixes the expected public shape of one kind.
type kindCase struct {
	status int
	typ    string
	title  string
}

// kindCases covers the validation minimum: one case per stable kind with
// the exact public status mapping.
func kindCases() map[apperr.Kind]kindCase {
	return map[apperr.Kind]kindCase{
		apperr.KindValidation:   {http.StatusBadRequest, "validation", "the request is invalid"},
		apperr.KindUnauthorized: {http.StatusUnauthorized, "unauthorized", "authentication is required"},
		apperr.KindForbidden:    {http.StatusForbidden, "forbidden", "you are not allowed to do this"},
		apperr.KindNotFound:     {http.StatusNotFound, "not_found", "resource not found"},
		apperr.KindConflict:     {http.StatusConflict, "conflict", "the request conflicts with the current state"},
		apperr.KindRateLimited:  {http.StatusTooManyRequests, "rate_limited", "too many requests"},
		apperr.KindInternal:     {http.StatusInternalServerError, "internal", "internal error"},
	}
}

func TestKindMappingTable(t *testing.T) {
	for kind, want := range kindCases() {
		err := apperr.New(kind, "ARENA-TEST", "public detail")
		problem := ProblemFor("req-1", err)

		if problem.Status != want.status {
			t.Errorf("kind %s: status = %d, want %d", kind, problem.Status, want.status)
		}
		if problem.Type != "https://goyim-arena.dev/problems/"+want.typ {
			t.Errorf("kind %s: type = %q", kind, problem.Type)
		}
		if problem.Title != want.title {
			t.Errorf("kind %s: title = %q, want %q", kind, problem.Title, want.title)
		}
		if problem.Code != "ARENA-TEST" {
			t.Errorf("kind %s: code = %q, want stable code", kind, problem.Code)
		}
		if problem.RequestID != "req-1" {
			t.Errorf("kind %s: request id = %q", kind, problem.RequestID)
		}
	}
}

func TestWriteProblemRendersProblemDetails(t *testing.T) {
	for kind, want := range kindCases() {
		recorder := httptest.NewRecorder()
		status := WriteProblem(recorder, "req-42", apperr.New(kind, "ARENA-TEST", "public detail"))

		if status != want.status {
			t.Errorf("kind %s: rendered status = %d, want %d", kind, status, want.status)
		}
		if recorder.Code != want.status {
			t.Errorf("kind %s: response code = %d, want %d", kind, recorder.Code, want.status)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Errorf("kind %s: content type = %q, want application/problem+json", kind, got)
		}

		var problem Problem
		if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
			t.Fatalf("kind %s: body is not valid JSON: %v\n%s", kind, err, recorder.Body.String())
		}
		if problem.Status != want.status || problem.Code != "ARENA-TEST" || problem.RequestID != "req-42" {
			t.Errorf("kind %s: unexpected problem: %+v", kind, problem)
		}
	}
}

// TestInternalCauseNeverSerializes is the security invariant: the wrapped
// cause (and the internal detail) must never reach the public body, while
// staying available for logs via errors.Is/As.
func TestInternalCauseNeverSerializes(t *testing.T) {
	sensitive := fmt.Errorf("pq: password authentication failed for user %q dsn=postgres://arena:hunter2@db", "admin")
	internal := apperr.New(apperr.KindInternal, "ARENA-DB-FAIL", "database unavailable").
		WithCause(sensitive)

	recorder := httptest.NewRecorder()
	status := WriteProblem(recorder, "req-7", internal)

	body := recorder.Body.String()
	for _, leaked := range []string{"hunter2", "admin", "pq:", "database unavailable"} {
		if strings.Contains(body, leaked) {
			t.Errorf("internal body leaked %q: %s", leaked, body)
		}
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("internal status = %d", status)
	}

	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if problem.Detail != "" {
		t.Errorf("internal problem must have empty detail, got %q", problem.Detail)
	}
	if problem.Title != "internal error" {
		t.Errorf("internal problem must keep the generic title, got %q", problem.Title)
	}
	if problem.Code != "ARENA-DB-FAIL" {
		t.Errorf("internal problem keeps the stable vocabulary code, got %q", problem.Code)
	}

	// The cause stays available for logs through unwrapping.
	if !errors.Is(internal, sensitive) {
		t.Error("internal cause must remain unwrappable for logging")
	}
	var appError *apperr.Error
	if !errors.As(internal, &appError) || appError.Code() != "ARENA-DB-FAIL" {
		t.Error("internal code must remain available for logs")
	}
}

func TestForeignErrorCollapsesToGenericInternalProblem(t *testing.T) {
	foreign := fmt.Errorf("explosive detail: secret-value-123")

	recorder := httptest.NewRecorder()
	status := WriteProblem(recorder, "req-8", foreign)

	if status != http.StatusInternalServerError {
		t.Fatalf("foreign error status = %d", status)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "secret-value-123") || strings.Contains(body, "explosive") {
		t.Errorf("foreign error text leaked into the body: %s", body)
	}

	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if problem.Code != "ARENA-internal" || problem.Detail != "" {
		t.Errorf("foreign error must render the generic problem: %+v", problem)
	}
}

func TestPublicDetailExposedForSafeKindsOnly(t *testing.T) {
	safe := []apperr.Kind{
		apperr.KindValidation, apperr.KindUnauthorized, apperr.KindForbidden,
		apperr.KindNotFound, apperr.KindConflict, apperr.KindRateLimited,
	}
	for _, kind := range safe {
		problem := ProblemFor("req", apperr.New(kind, "ARENA-X", "username already taken"))
		if problem.Detail != "username already taken" {
			t.Errorf("kind %s: safe detail hidden: %q", kind, problem.Detail)
		}
	}

	internal := ProblemFor("req", apperr.New(apperr.KindInternal, "ARENA-X", "row lock timeout"))
	if internal.Detail != "" {
		t.Errorf("internal detail must be hidden, got %q", internal.Detail)
	}
}

func TestRateLimitedProblemShape(t *testing.T) {
	problem := ProblemFor("req-9", apperr.New(apperr.KindRateLimited, "ARENA-RL-1", "try again in 30 seconds"))
	if problem.Status != http.StatusTooManyRequests {
		t.Fatalf("rate limit status = %d", problem.Status)
	}
	if problem.Detail != "try again in 30 seconds" {
		t.Fatalf("rate limit detail = %q", problem.Detail)
	}
}
