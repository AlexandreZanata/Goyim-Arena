package observability

import (
	"sort"
)

// The allowlisted analytics events of the product. The plan demands
// "eventos analytics allowlisted": a new event starts here, in review, with
// its emitter and its admitted properties — never at a call site.
//
// The vocabulary is the actions the composed surfaces can attribute to an
// account. A flow whose use case answers an error only (email confirmation
// and password recovery, today) has no account to attribute the event to and
// therefore no event here: telemetry is added together with the fact, not
// ahead of it.
const (
	// EventAccountRegistrationSubmitted fires when the registration form is
	// accepted. The flow answers uniformly for a fresh address and for one
	// that already had an account, so the event records the accepted
	// submission and deliberately does not claim a new account.
	EventAccountRegistrationSubmitted = "account.registration_submitted"
	// EventAccountSignedIn fires when credentials are accepted and a session
	// is opened.
	EventAccountSignedIn = "account.signed_in"
	// EventArenaPositionConfirmed fires when the first position of an account
	// in an Arena is recorded.
	EventArenaPositionConfirmed = "arena.position_confirmed"
	// EventArenaPositionChanged fires when an accepted position change is
	// recorded.
	EventArenaPositionChanged = "arena.position_changed"
	// EventArenaArgumentPublished fires when an argument is published and its
	// INK is charged.
	EventArenaArgumentPublished = "arena.argument_published"
	// EventArenaInfluenceAssigned fires when influence is attributed to a
	// position change.
	EventArenaInfluenceAssigned = "arena.influence_assigned"
)

// propertyKind is the admitted value domain of one property. There is no
// free-form string kind on purpose: the only string that may cross this
// boundary is a locale tag validated against the catalog allowlist, so an
// email, a message body or a provider payload has no admitted shape to
// travel in.
type propertyKind uint8

const (
	kindLocale propertyKind = iota + 1
	kindInteger
)

// propertySpec is the admitted domain of one property name.
type propertySpec struct {
	kind propertyKind
	// min and max bound kindInteger values. They are ignored by kindLocale.
	min int64
	max int64
}

var (
	// localeSpec admits an interface locale tag in the catalog allowlist.
	localeSpec = propertySpec{kind: kindLocale}
	// attributedCountSpec admits the number of arguments one attribution
	// credited, bounded well above any policy limit.
	attributedCountSpec = propertySpec{kind: kindInteger, min: 1, max: 100}
)

// allowlist is the complete set of analytics events the product may send,
// with the property names each one admits.
var allowlist = map[string]map[string]propertySpec{
	EventAccountRegistrationSubmitted: {"locale": localeSpec},
	EventAccountSignedIn:              {"locale": localeSpec},
	EventArenaPositionConfirmed:       {"locale": localeSpec},
	EventArenaPositionChanged:         {"locale": localeSpec},
	EventArenaArgumentPublished:       {"locale": localeSpec},
	EventArenaInfluenceAssigned: {
		"locale":           localeSpec,
		"attributed_count": attributedCountSpec,
	},
}

// IsAllowlisted reports whether the event name may be sent at all.
func IsAllowlisted(name string) bool {
	_, exists := allowlist[name]
	return exists
}

// AllowedProperties returns the property names the event admits, sorted. An
// unknown event returns nil.
func AllowedProperties(name string) []string {
	spec, exists := allowlist[name]
	if !exists {
		return nil
	}
	names := make([]string, 0, len(spec))
	for property := range spec {
		names = append(names, property)
	}
	sort.Strings(names)
	return names
}

// AllowlistedEvents returns every event name, sorted, so tests and
// documentation can compare the shipped set against the contract.
func AllowlistedEvents() []string {
	names := make([]string, 0, len(allowlist))
	for name := range allowlist {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
