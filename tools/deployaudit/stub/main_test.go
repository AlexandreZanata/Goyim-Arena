package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerMapsModeAndPathToStatus is the contract the drill relies on, and
// it is asserted here so that a mode that silently starts claiming readiness
// fails a unit test in `make verify` instead of turning a long Docker exercise
// green for the wrong reason.
func TestHandlerMapsModeAndPathToStatus(t *testing.T) {
	cases := []struct {
		mode string
		path string
		want int
	}{
		// A release that never becomes ready fails the readiness probe. This
		// is the incoming release of the health scenario.
		{modeNotReady, "/health/live", http.StatusServiceUnavailable},
		{modeNotReady, "/health/ready", http.StatusServiceUnavailable},
		{modeNotReady, "/login", http.StatusServiceUnavailable},

		// A release that is ready serves both probes and its pages. This is
		// the incoming release of the promote and rollback scenarios.
		{modeReady, "/health/live", http.StatusOK},
		{modeReady, "/health/ready", http.StatusOK},
		{modeReady, "/", http.StatusOK},
		{modeReady, "/login", http.StatusOK},

		// A release that is ready and has lost a page fails the smoke probe
		// and nothing else: readiness is not the claim that pages answer.
		{modePageMissing, "/health/live", http.StatusOK},
		{modePageMissing, "/health/ready", http.StatusOK},
		{modePageMissing, "/login", http.StatusNotFound},
	}

	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		newHandler(tc.mode).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != tc.want {
			t.Errorf("mode %q, path %q: status %d, want %d", tc.mode, tc.path, recorder.Code, tc.want)
		}
	}
}

// TestServingPageIsItsOwn checks the marker the drill uses: after a rollback it
// asserts the application's page came back, so the stand-in's page has to be
// recognisable and has to name itself.
func TestServingPageIsItsOwn(t *testing.T) {
	recorder := httptest.NewRecorder()
	newHandler(modeReady).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	if !strings.Contains(string(body), "stand-in") {
		t.Errorf("the serving mode returned %q, which does not name itself as a stand-in", body)
	}
}

// TestUnknownModeDoesNotClaimReadiness pins the defensive default: a mode that
// is not one of the three built ones is treated as "not ready", never as
// "ready". A misspelled mode must not be able to make a broken release look
// healthy.
func TestUnknownModeDoesNotClaimReadiness(t *testing.T) {
	for _, mode := range []string{"", "READY", "ready ", "readyness", "notready"} {
		recorder := httptest.NewRecorder()
		newHandler(normalizeMode(mode)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
		if recorder.Code == http.StatusOK {
			t.Errorf("mode %q normalised to a ready release: status %d", mode, recorder.Code)
		}
	}
}

// TestKnownModesSurviveNormalisation is the other half: the three modes the
// drill bakes are the three modes that come back unchanged.
func TestKnownModesSurviveNormalisation(t *testing.T) {
	for _, mode := range []string{modeReady, modePageMissing, modeNotReady} {
		if got := normalizeMode(mode); got != mode {
			t.Errorf("normalizeMode(%q) = %q, want it unchanged", mode, got)
		}
	}
}
