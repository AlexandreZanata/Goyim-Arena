package buildinfo

import (
	"encoding/json"
	"testing"
	"time"
)

// setLinkTimeVars overrides the link-time variables and returns a restore
// function. The test binary is not built with -ldflags, so this simulates
// both the injected and the unset states.
func setLinkTimeVars(t *testing.T, versionValue, commitValue, dateValue string) func() {
	t.Helper()

	savedVersion, savedCommit, savedDate := version, commit, date
	version, commit, date = versionValue, commitValue, dateValue
	return func() {
		version, commit, date = savedVersion, savedCommit, savedDate
	}
}

func TestCurrentWithoutInjectionReturnsDevelopmentDefaults(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")

	restore := setLinkTimeVars(t, "", "", "")
	defer restore()

	info := Current()

	if info.Version != "dev" {
		t.Fatalf("version = %q, want %q", info.Version, "dev")
	}
	if info.Commit != "unknown" {
		t.Fatalf("commit = %q, want %q", info.Commit, "unknown")
	}
	if !info.Date.IsZero() {
		t.Fatalf("date = %v, want zero", info.Date.Time)
	}
}

func TestCurrentUsesInjectedValues(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")

	restore := setLinkTimeVars(t, "1.2.3", "abc1234", "1760000000")
	defer restore()

	info := Current()

	if info.Version != "1.2.3" {
		t.Fatalf("version = %q, want %q", info.Version, "1.2.3")
	}
	if info.Commit != "abc1234" {
		t.Fatalf("commit = %q, want %q", info.Commit, "abc1234")
	}
	want := time.Unix(1760000000, 0).UTC()
	if !info.Date.Equal(want) {
		t.Fatalf("date = %v, want %v", info.Date.Time, want)
	}
}

func TestCurrentPrefersInjectedDateOverSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	restore := setLinkTimeVars(t, "2.0.0", "def5678", "1760000000")
	defer restore()

	info := Current()

	want := time.Unix(1760000000, 0).UTC()
	if !info.Date.Equal(want) {
		t.Fatalf("date = %v, want injected value %v", info.Date.Time, want)
	}
}

func TestCurrentFallsBackToSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")

	restore := setLinkTimeVars(t, "", "", "")
	defer restore()

	info := Current()

	if info.Version != "dev" || info.Commit != "unknown" {
		t.Fatalf("version/commit = %q/%q, want dev/unknown", info.Version, info.Commit)
	}
	want := time.Unix(1700000000, 0).UTC()
	if !info.Date.Equal(want) {
		t.Fatalf("date = %v, want SOURCE_DATE_EPOCH value %v", info.Date.Time, want)
	}
}

func TestParseBuildDateAcceptsUnixAndRFC3339(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")

	unix := ParseBuildDate("1760000000")
	if !unix.Equal(time.Unix(1760000000, 0).UTC()) {
		t.Fatalf("unix date = %v, want %v", unix.Time, time.Unix(1760000000, 0).UTC())
	}

	rfc3339 := ParseBuildDate("2025-10-09T08:53:20Z")
	if !rfc3339.Equal(time.Unix(1760000000, 0).UTC()) {
		t.Fatalf("rfc3339 date = %v, want %v", rfc3339.Time, time.Unix(1760000000, 0).UTC())
	}

	if !ParseBuildDate("not-a-date").IsZero() {
		t.Fatalf("invalid date = %v, want zero", ParseBuildDate("not-a-date").Time)
	}
}

func TestBuildTimeJSONRoundTrip(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")

	want := time.Unix(1760000000, 0).UTC()

	encoded, err := json.Marshal(BuildTime{Time: want})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `"2025-10-09T08:53:20Z"` {
		t.Fatalf("encoded = %s, want RFC 3339 string", encoded)
	}

	var decoded BuildTime
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Equal(want) {
		t.Fatalf("decoded = %v, want %v", decoded.Time, want)
	}
}

func TestBuildTimeZeroMarshalsAsUnknown(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")

	encoded, err := json.Marshal(BuildTime{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `"unknown"` {
		t.Fatalf("encoded = %s, want %q", encoded, `"unknown"`)
	}

	var decoded BuildTime
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.IsZero() {
		t.Fatalf("decoded = %v, want zero", decoded.Time)
	}
}

func TestInfoJSONShape(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "")

	restore := setLinkTimeVars(t, "1.2.3", "abc1234", "1760000000")
	defer restore()

	encoded, err := json.Marshal(Current())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"version", "commit", "date"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("JSON missing key %q: %s", key, encoded)
		}
	}
	if payload["version"] != "1.2.3" || payload["commit"] != "abc1234" {
		t.Fatalf("unexpected JSON payload: %s", encoded)
	}
	if payload["date"] != "2025-10-09T08:53:20Z" {
		t.Fatalf("unexpected date in JSON: %s", encoded)
	}
}
