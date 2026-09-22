package observability

import (
	"sort"
	"strings"
	"testing"
)

// TestTheAllowlistAdmitsOnlyKnownShapes is the review gate of the event
// vocabulary: every admitted property is one of the two value domains, none
// of them is a forbidden name, and the integer bounds are usable.
func TestTheAllowlistAdmitsOnlyKnownShapes(t *testing.T) {
	t.Parallel()

	for name, spec := range allowlist {
		if name == "" || strings.TrimSpace(name) != name {
			t.Errorf("event name %q must be a non-empty constant", name)
		}
		if len(spec) == 0 {
			t.Errorf("event %q admits no properties; either give it a fact or remove it", name)
		}
		for property, definition := range spec {
			if forbiddenPropertyNames[property] {
				t.Errorf("event %q admits forbidden property %q", name, property)
			}
			switch definition.kind {
			case kindLocale:
				if definition.min != 0 || definition.max != 0 {
					t.Errorf("event %q property %q carries integer bounds for a locale", name, property)
				}
			case kindInteger:
				if definition.max <= definition.min || definition.min < 0 {
					t.Errorf("event %q property %q has unusable bounds [%d, %d]", name, property, definition.min, definition.max)
				}
			default:
				t.Errorf("event %q property %q has unknown kind %d", name, property, definition.kind)
			}
		}
	}
}

// TestEveryEventConstantIsAllowlisted keeps the constants and the map from
// drifting apart: a constant that is not admitted is a call site that would
// be refused in production.
func TestEveryEventConstantIsAllowlisted(t *testing.T) {
	t.Parallel()

	constants := []string{
		EventAccountRegistrationSubmitted,
		EventAccountSignedIn,
		EventArenaPositionConfirmed,
		EventArenaPositionChanged,
		EventArenaArgumentPublished,
		EventArenaInfluenceAssigned,
	}
	sort.Strings(constants)
	if admitted := AllowlistedEvents(); !equalSlices(admitted, constants) {
		t.Fatalf("allowlisted events = %v, constants = %v", admitted, constants)
	}
}

// TestScrubbedRefusesProgrammingErrors covers the mistakes that must refuse
// the whole event: an unknown name, a property the event does not admit, and
// an event with no account attribution.
func TestScrubbedRefusesProgrammingErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name:  "unknown event",
			event: Event{Name: "account.unknown", AccountID: "acc-1"},
			want:  "not allowlisted",
		},
		{
			name: "unadmitted property",
			event: Event{
				Name:       EventAccountSignedIn,
				AccountID:  "acc-1",
				Properties: map[string]any{"checkout_session": "cs_test_123"},
			},
			want: "does not admit property",
		},
		{
			name:  "no attribution",
			event: Event{Name: EventAccountSignedIn},
			want:  "no account attribution",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Scrubbed(testCase.event)
			if err == nil {
				t.Fatal("the event must be refused")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestScrubbedDropsValuesOutsideTheirDomain is the redaction gate: an
// admitted property whose value does not fit its domain is dropped while the
// event still travels, so no email, argument or provider payload reaches a
// sink through an allowlisted name.
func TestScrubbedDropsValuesOutsideTheirDomain(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		properties map[string]any
		want       map[string]any
	}{
		{
			name:       "an email is not a locale",
			properties: map[string]any{"locale": "user@example.invalid"},
			want:       map[string]any{},
		},
		{
			name:       "an unknown tag is not a locale",
			properties: map[string]any{"locale": "xx-YY"},
			want:       map[string]any{},
		},
		{
			name:       "a payload is not a locale",
			properties: map[string]any{"locale": map[string]any{"amount": 100}},
			want:       map[string]any{},
		},
		{
			name:       "a supported tag travels",
			properties: map[string]any{"locale": "pt-BR"},
			want:       map[string]any{"locale": "pt-BR"},
		},
		{
			name:       "an empty locale is dropped",
			properties: map[string]any{"locale": ""},
			want:       map[string]any{},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			scrubbed, err := Scrubbed(Event{
				Name:       EventAccountSignedIn,
				AccountID:  "acc-1",
				Properties: testCase.properties,
			})
			if err != nil {
				t.Fatalf("Scrubbed() error = %v", err)
			}
			if len(scrubbed) != len(testCase.want) {
				t.Fatalf("scrubbed = %v, want %v", scrubbed, testCase.want)
			}
			for name, want := range testCase.want {
				if scrubbed[name] != want {
					t.Fatalf("scrubbed[%q] = %v, want %v", name, scrubbed[name], want)
				}
			}
		})
	}
}

// TestScrubbedBoundsTheAttributedCount covers the integer domain: the count
// that leaves the process is an integer within the admitted bounds, never a
// float, a string or an out-of-range number.
func TestScrubbedBoundsTheAttributedCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  any
	}{
		{name: "in range int", value: 2, want: int64(2)},
		{name: "in range int64", value: int64(3), want: int64(3)},
		{name: "zero", value: 0, want: nil},
		{name: "above the bound", value: 101, want: nil},
		{name: "a float", value: 2.5, want: nil},
		{name: "a string", value: "2", want: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			scrubbed, err := Scrubbed(Event{
				Name:      EventArenaInfluenceAssigned,
				AccountID: "acc-1",
				Properties: map[string]any{
					"locale":           "en-US",
					"attributed_count": testCase.value,
				},
			})
			if err != nil {
				t.Fatalf("Scrubbed() error = %v", err)
			}
			got, exists := scrubbed["attributed_count"]
			if testCase.want == nil {
				if exists {
					t.Fatalf("attributed_count = %v, want it dropped", got)
				}
				return
			}
			if got != testCase.want {
				t.Fatalf("attributed_count = %v, want %v", got, testCase.want)
			}
		})
	}
}

// equalSlices compares two sorted string slices.
func equalSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
