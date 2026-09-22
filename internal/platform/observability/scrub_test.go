package observability

import (
	"strings"
	"testing"
)

// TestAllowlistHasNoForbiddenProperty covers the structural rule: whatever a
// future event admits, it may not admit a property name that carries identity,
// credentials, message content or a provider payload. The check is here, not
// only at the call site, so the mistake is caught when the event is added.
func TestAllowlistHasNoForbiddenProperty(t *testing.T) {
	t.Parallel()

	for _, event := range AllowlistedEvents() {
		for _, property := range AllowedProperties(event) {
			if forbiddenPropertyNames[property] {
				t.Fatalf("event %q admits forbidden property %q", event, property)
			}
		}
	}
}

// TestAllowlistedEventsAreSortedAndNonEmpty covers the published set: the plan
// demands allowlisted events, and an empty allowlist would make every capture
// a programming error in silence.
func TestAllowlistedEventsAreSortedAndNonEmpty(t *testing.T) {
	t.Parallel()

	events := AllowlistedEvents()
	if len(events) == 0 {
		t.Fatal("the allowlist must not be empty")
	}
	for index := 1; index < len(events); index++ {
		if events[index-1] >= events[index] {
			t.Fatalf("allowlisted events are not strictly sorted: %q then %q", events[index-1], events[index])
		}
	}
	for _, event := range events {
		if !IsAllowlisted(event) {
			t.Fatalf("event %q is listed but not allowlisted", event)
		}
	}
}

// TestScrubbedRefusesUnknownEvent covers the programming-error half: an event
// name outside the allowlist is refused whole, in every mode, naming the file
// where the event belongs.
func TestScrubbedRefusesUnknownEvent(t *testing.T) {
	t.Parallel()

	_, err := Scrubbed(Event{Name: "account.made_up", AccountID: "acc-1"})
	if err == nil {
		t.Fatal("an event outside the allowlist must be refused")
	}
	if !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("error = %q, want it to name the allowlist", err)
	}
}

// TestScrubbedRefusesUnadmittedProperty covers the second programming error: a
// property the event does not admit is refused whole, so a call site cannot
// widen what is sent by passing a new key.
func TestScrubbedRefusesUnadmittedProperty(t *testing.T) {
	t.Parallel()

	_, err := Scrubbed(Event{
		Name:       EventAccountSignedIn,
		AccountID:  "acc-1",
		Properties: map[string]any{"plan": "premium"},
	})
	if err == nil {
		t.Fatal("an unadmitted property must be refused")
	}
}

// TestScrubbedRefusesForbiddenPropertyIsUnreachableByAllowlist covers the
// belt-and-braces refusal: even if an allowlist entry named a forbidden
// property, the runtime check would still refuse the event.
//
// This test temporarily edits the package allowlist, so it must not run in
// parallel with the tests that read it: Go runs the sequential tests of a
// package to completion before the parallel ones resume.
func TestScrubbedRefusesForbiddenPropertyIsUnreachableByAllowlist(t *testing.T) {
	original := allowlist[EventAccountSignedIn]
	allowlist[EventAccountSignedIn] = map[string]propertySpec{"email": localeSpec}
	defer func() { allowlist[EventAccountSignedIn] = original }()

	_, err := Scrubbed(Event{
		Name:       EventAccountSignedIn,
		AccountID:  "acc-1",
		Properties: map[string]any{"email": "person@example.invalid"},
	})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %v, want a forbidden-property refusal", err)
	}
}

// TestScrubbedRefusesEventWithoutAttribution covers the attribution rule: an
// event with no account has no distinct identifier to aggregate under, so it
// is refused rather than recorded under a lie.
func TestScrubbedRefusesEventWithoutAttribution(t *testing.T) {
	t.Parallel()

	if _, err := Scrubbed(Event{Name: EventAccountSignedIn}); err == nil {
		t.Fatal("an event without an account must be refused")
	}
}

// TestScrubbedDropsValueOutsideItsDomain covers the best-effort half: a value
// that does not fit its admitted domain is dropped from the document and the
// event still travels, because the value came from request-adjacent data.
func TestScrubbedDropsValueOutsideItsDomain(t *testing.T) {
	t.Parallel()

	properties, err := Scrubbed(Event{
		Name:      EventArenaInfluenceAssigned,
		AccountID: "acc-1",
		Properties: map[string]any{
			"locale":           "not-a-locale",
			"attributed_count": 1_000_000,
		},
	})
	if err != nil {
		t.Fatalf("a value outside its domain must not refuse the event: %v", err)
	}
	if len(properties) != 0 {
		t.Fatalf("properties = %v, want both dropped", properties)
	}
}

// TestScrubbedAdmitsTheAdmittedValues covers the positive path: a supported
// locale and an in-range integer travel unchanged.
func TestScrubbedAdmitsTheAdmittedValues(t *testing.T) {
	t.Parallel()

	properties, err := Scrubbed(Event{
		Name:      EventArenaInfluenceAssigned,
		AccountID: "acc-1",
		Properties: map[string]any{
			"locale":           "pt-BR",
			"attributed_count": int64(3),
		},
	})
	if err != nil {
		t.Fatalf("scrub: %v", err)
	}
	if properties["locale"] != "pt-BR" {
		t.Fatalf("locale = %v, want pt-BR", properties["locale"])
	}
	if properties["attributed_count"] != int64(3) {
		t.Fatalf("attributed_count = %v, want 3", properties["attributed_count"])
	}
}

// TestScrubbedNeverCarriesRequestDerivedStrings covers the redaction promise
// from the other direction: the only string kind the allowlist admits is a
// locale, so an email, a message body or a provider payload has no admitted
// shape to travel in — the values are dropped, and the event still travels.
func TestScrubbedNeverCarriesRequestDerivedStrings(t *testing.T) {
	t.Parallel()

	properties, err := Scrubbed(Event{
		Name:      EventAccountSignedIn,
		AccountID: "acc-1",
		Properties: map[string]any{
			"locale": "person@example.invalid",
		},
	})
	if err != nil {
		t.Fatalf("scrub: %v", err)
	}
	if _, present := properties["locale"]; present {
		t.Fatalf("an email must not survive as a locale: %v", properties)
	}
}

// TestSanitizedTagsDropsAnythingNotAnOperationToken covers the error-report
// tag rule: a tag is where an unexpected error would echo its input, so what
// does not look like a short printable token is dropped.
func TestSanitizedTagsDropsAnythingNotAnOperationToken(t *testing.T) {
	t.Parallel()

	tags := SanitizedTags(ErrorReport{
		Kind:      "handler",
		Operation: "email_delivery",
		RequestID: "line one\nline two",
	})
	if tags["kind"] != "handler" || tags["operation"] != "email_delivery" {
		t.Fatalf("tags = %v, want the two printable tokens", tags)
	}
	if _, present := tags["request_id"]; present {
		t.Fatalf("a tag with a newline must be dropped: %v", tags)
	}

	empty := SanitizedTags(ErrorReport{})
	if len(empty) != 0 {
		t.Fatalf("empty fields must not become tags: %v", empty)
	}
}
