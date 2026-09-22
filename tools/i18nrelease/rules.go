// The rules of the internationalization audit (P20-T09).
//
// Each rule is one property that has to hold for the register to be an audit
// rather than a summary, and each names the failure it prevents. Two families
// live together here:
//
//   - properties of the tree that the audit measures itself and refuses
//     outright — a divergent placeholder, an empty message, a document varied by
//     the raw `Accept-Language` header, a value left to `toLocaleString`;
//   - properties of the register that the audit can only check against the
//     measurement — a claim that disagrees with the tree, an area the phase
//     names that the register does not execute, a finding with no owner, a
//     human review recorded as done with nobody's name on it.
//
// The second family is why the block exists. A document that narrates what the
// audit did is indistinguishable from one that narrates what the audit was
// supposed to do, until something compares the two.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Rule names, so a failure says which property broke.
const (
	RuleVersion        = "document-version"
	RuleDate           = "document-date"
	RuleSections       = "section-missing"
	RuleAreaSet        = "area-set"
	RuleAreaCommand    = "area-command"
	RuleAreaEvidence   = "area-evidence"
	RuleAreaStatus     = "area-status"
	RuleClaimCatalog   = "claim-catalog"
	RuleClaimZero      = "claim-zero"
	RuleClaimOrphans   = "claim-orphans"
	RuleClaimGoldens   = "claim-email-goldens"
	RuleClaimCodes     = "claim-error-codes"
	RuleClaimPlural    = "claim-plural"
	RuleClaimHreflang  = "claim-hreflang"
	RuleClaimCache     = "claim-cache"
	RulePseudoMechanic = "pseudo-mechanism"
	RuleJourneyLocales = "journey-locales"
	RuleJourneyContent = "journey-content-language"
	RuleJourneyGate    = "journey-gate"
	RuleFindingVocab   = "finding-vocabulary"
	RuleFindingBlocker = "finding-blocker"
	RuleReviewState    = "review-state"
	RuleLimits         = "limits"
	RulePII            = "document-pii"
)

// Violation is one refusal, with the rule that produced it.
type Violation struct {
	Rule   string
	Detail string
}

// String renders a violation as one reviewable line.
func (violation Violation) String() string {
	return violation.Rule + ": " + violation.Detail
}

// areaCatalogue is the phase's list of areas, read from
// `.local/phases/20-release-readiness.md` and turned into the command that
// executes each one. A register that executes a different list is refused: the
// point of the audit is the properties named, not the properties found.
//
// The commands are the delivered gates, not private ones: an audit that
// executed its own copies of the suites would prove that the audit passes, and
// would go on passing after the gates that ship stopped existing.
var areaCatalogue = []Area{
	{ID: "catalog-coverage", Command: "make generate-check", Evidence: "locales/pt-BR/arenas.json"},
	{ID: "pseudo-locale", Command: "go test -tags pseudolocale ./internal/i18n/...", Evidence: "internal/i18n/pseudo_enabled.go"},
	{ID: "catalog-snapshots", Command: "go test ./internal/notifications/adapters/renderer/...", Evidence: "internal/notifications/adapters/renderer/snapshot_test.go"},
	{ID: "emails", Command: "go test ./internal/notifications/...", Evidence: "locales/pt-BR/email.json"},
	{ID: "problem-details", Command: "go test ./internal/platform/httperror/...", Evidence: "locales/pt-BR/errors.json"},
	{ID: "seo", Command: "go test ./internal/arenas/adapters/html/...", Evidence: "internal/arenas/adapters/html/templates.go"},
	{ID: "money", Command: "make test-web", Evidence: "web/src/i18n/formats.ts"},
	{ID: "plural", Command: "make test-web", Evidence: "web/src/i18n/translator.ts"},
	{ID: "timezone", Command: "make test-web", Evidence: "web/tests/i18n/formats.test.ts"},
	{ID: "cache", Command: "go test ./internal/arenas/adapters/html/...", Evidence: "internal/platform/httpcache/policy.go"},
	{ID: "hardcoded-text", Command: "make audit-i18n", Evidence: "tools/i18naudit/audit.go"},
	{ID: "browser-journeys", Command: "make test-e2e", Evidence: "tools/e2e/support/locales.js"},
}

// minimumLimits is how many limitations the register has to declare. A register
// that declares none is a register claiming the audit proved everything it
// touched, and no audit does.
const minimumLimits = 5

// reservedZones are the domain suffixes a document may carry: reserved by RFC
// 2606 for documentation, and reserved by the project for synthetic identities.
var reservedZones = []string{
	"example.com", "example.net", "example.org", "example.test", "example.invalid",
}

// emailPattern finds anything shaped like an address.
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// Judge compares a register with a measurement and a root, and returns every
// property that does not hold.
func Judge(facts Facts, document register, text, root string) []Violation {
	var violations []Violation
	refuse := func(rule, format string, arguments ...any) {
		violations = append(violations, Violation{Rule: rule, Detail: fmt.Sprintf(format, arguments...)})
	}

	if document.Version != version {
		refuse(RuleVersion, "the block declares version %q and this audit reads %q", document.Version, version)
	}
	if _, err := time.Parse("2006-01-02", document.Date); err != nil {
		refuse(RuleDate, "the audit date %q is not an ISO 8601 day", document.Date)
	}
	for _, section := range requiredSections {
		if !strings.Contains(text, section) {
			refuse(RuleSections, "the document has no %q section: a register nobody can read is not an audit", section)
		}
	}

	violations = append(violations, judgeAreas(document, root)...)

	// The catalog, claim by claim.
	if !equalStrings(document.Claims.Locales, facts.Catalog.Locales) {
		refuse(RuleClaimCatalog, "the register claims locales %v and the tree holds %v", document.Claims.Locales, facts.Catalog.Locales)
	}
	if !equalStrings(document.Claims.Namespaces, facts.Catalog.Namespaces) {
		refuse(RuleClaimCatalog, "the register claims namespaces %v and the tree holds %v", document.Claims.Namespaces, facts.Catalog.Namespaces)
	}
	for _, locale := range facts.Catalog.Locales {
		claimed, ok := document.Claims.KeysPerLocale[locale]
		if !ok {
			refuse(RuleClaimCatalog, "the register claims no message count for %s, which the tree holds (%d)", locale, facts.Catalog.Keys[locale])
			continue
		}
		if claimed != facts.Catalog.Keys[locale] {
			refuse(RuleClaimCatalog, "the register claims %d messages in %s and the tree holds %d", claimed, locale, facts.Catalog.Keys[locale])
		}
	}
	for locale := range document.Claims.KeysPerLocale {
		if !contains(facts.Catalog.Locales, locale) {
			refuse(RuleClaimCatalog, "the register claims a message count for %q, which is not a locale of the catalog", locale)
		}
	}

	for _, measured := range []struct {
		rule    string
		claim   int
		actual  int
		subject string
	}{
		{RuleClaimZero, document.Claims.PlaceholderDivergence, len(facts.Catalog.PlaceholderDivergence), "keys with divergent placeholders"},
		{RuleClaimZero, document.Claims.EmptyMessages, len(facts.Catalog.Empty), "empty messages"},
		{RuleClaimZero, document.Claims.ErrorCodeDrift, len(facts.Codes.Drift), "error codes missing from a locale"},
		{RuleClaimZero, document.Claims.UnknownReferences, len(facts.Catalog.Unknown), "references to keys the catalog does not declare"},
	} {
		if measured.claim != measured.actual {
			refuse(measured.rule, "the register claims %d %s and the tree holds %d", measured.claim, measured.subject, measured.actual)
		}
	}

	// A divergent placeholder is refused outright, whatever the register says:
	// a sentence that renders with a hole in it is a broken page in the locale
	// that lost the placeholder.
	if len(facts.Catalog.PlaceholderDivergence) > 0 {
		refuse(RuleClaimZero, "placeholder divergence: %v", facts.Catalog.PlaceholderDivergence)
	}
	if len(facts.Catalog.Empty) > 0 {
		refuse(RuleClaimZero, "empty messages: %v", facts.Catalog.Empty)
	}
	if len(facts.Codes.Drift) > 0 {
		refuse(RuleClaimCodes, "error codes declared in some locales and not all: %v", facts.Codes.Drift)
	}

	// Orphans and unknown references may exist; what may not exist is one the
	// register does not name. An orphan key is a sentence nobody can render,
	// and leaving it unnamed is how it survives the next audit too.
	if document.Claims.OrphanKeys != len(facts.Catalog.Orphans) {
		refuse(RuleClaimOrphans, "the register claims %d orphan keys and the tree holds %d", document.Claims.OrphanKeys, len(facts.Catalog.Orphans))
	}
	for _, orphan := range facts.Catalog.Orphans {
		if !documentNames(document, orphan) {
			refuse(RuleClaimOrphans, "the key %q is reachable from no delivered file and the register does not name it", orphan)
		}
	}
	for _, unknown := range facts.Catalog.Unknown {
		literal := unknown[strings.Index(unknown, ": ")+2:]
		if !documentNames(document, literal) {
			refuse(RuleClaimOrphans, "the literal %q is shaped like a catalog key and no locale declares it; the register does not name it", literal)
		}
	}

	if document.Claims.EmailGoldens != facts.Emails.Present {
		refuse(RuleClaimGoldens, "the register claims %d committed email snapshots and the tree holds %d", document.Claims.EmailGoldens, facts.Emails.Present)
	}
	if len(facts.Emails.Missing) > 0 {
		refuse(RuleClaimGoldens, "templates without a committed snapshot: %v", facts.Emails.Missing)
	}
	if document.Claims.PluralVariants != facts.Plural.Variants {
		refuse(RuleClaimPlural, "the register claims %d plural variants and the catalog declares %d", document.Claims.PluralVariants, facts.Plural.Variants)
	}
	if document.Claims.Hreflang != len(facts.Search.Hreflang) {
		refuse(RuleClaimHreflang, "the register claims %d hreflang occurrences and the tree holds %d", document.Claims.Hreflang, len(facts.Search.Hreflang))
	}
	if document.Claims.ToLocaleString != len(facts.Search.ToLocaleString) {
		refuse(RuleClaimHreflang, "the register claims %d uses of toLocaleString and the tree holds %d", document.Claims.ToLocaleString, len(facts.Search.ToLocaleString))
	}
	// §6 forbids the browser's own locale for formatting; there is no waiver
	// for it, so it is refused rather than reported.
	if len(facts.Search.ToLocaleString) > 0 {
		refuse(RuleClaimHreflang, "a value is formatted with the browser's locale instead of an explicit one: %v", facts.Search.ToLocaleString)
	}
	if document.Claims.VaryByRawAcceptLanguage != len(facts.Cache.RawAcceptLanguage) {
		refuse(RuleClaimCache, "the register claims %d places varying by the raw Accept-Language header and the tree holds %d", document.Claims.VaryByRawAcceptLanguage, len(facts.Cache.RawAcceptLanguage))
	}
	// A public response varied by the raw `Accept-Language` header is a defect
	// §4 names, and the audit does not decide the remedy: serving the localized
	// document privately and resolving the public one from an explicit locale
	// are both compliant and are a product decision. What the register may not
	// do is leave it unnamed — an unrecorded violation is the one that survives
	// the next audit.
	for _, place := range facts.Cache.RawAcceptLanguage {
		file := place[:strings.Index(place, ":")]
		if !documentNames(document, file) {
			refuse(RuleClaimCache, "%s varies a response by the raw Accept-Language header (§4 forbids it) and the register does not name it", place)
		}
	}
	if document.Claims.PublicCacheableSurfaces != facts.Cache.PublicSurfaces {
		refuse(RuleClaimCache, "the register claims %d publicly cacheable call sites and the tree holds %d", document.Claims.PublicCacheableSurfaces, facts.Cache.PublicSurfaces)
	}

	// The pseudo-locale has to be a mechanism, not a claim.
	if facts.Pseudo.Enabled == "" || facts.Pseudo.Disabled == "" {
		refuse(RulePseudoMechanic, "the build-tagged pseudo-locale files are not both present")
	}
	if facts.Pseudo.InProductionCatalog {
		refuse(RulePseudoMechanic, "the pseudo-locale leaked into the delivered catalog: it would be negotiable in production")
	}

	// The journeys, in every supported locale, on the seeded content language.
	if !equalStrings(document.Claims.JourneyLocales, facts.Journeys.Locales) {
		refuse(RuleJourneyLocales, "the register claims journeys in %v and the suite iterates over %v", document.Claims.JourneyLocales, facts.Journeys.Locales)
	}
	if !equalStrings(facts.Journeys.Locales, facts.Catalog.Locales) {
		refuse(RuleJourneyLocales, "the journeys cover %v and the product ships %v", facts.Journeys.Locales, facts.Catalog.Locales)
	}
	if facts.Journeys.ContentLanguageHarness == "" || facts.Journeys.ContentLanguageSeed == "" {
		refuse(RuleJourneyContent, "the content language is declared by the harness (%q) or the seed (%q) but not both",
			facts.Journeys.ContentLanguageHarness, facts.Journeys.ContentLanguageSeed)
	} else if facts.Journeys.ContentLanguageHarness != facts.Journeys.ContentLanguageSeed {
		refuse(RuleJourneyContent, "the harness exports %q and the seed declares %q: the journeys would compare a document against a copy of the fact that drifted",
			facts.Journeys.ContentLanguageHarness, facts.Journeys.ContentLanguageSeed)
	}
	violations = append(violations, judgeJourneyGate(facts.Journeys, root)...)

	violations = append(violations, judgeFindings(document)...)
	violations = append(violations, judgeReview(document, root)...)

	if len(document.Limits) < minimumLimits {
		refuse(RuleLimits, "the register declares %d limitations and at least %d are required: an audit that declares none claims more than it proves",
			len(document.Limits), minimumLimits)
	}
	for index, limit := range document.Limits {
		if strings.TrimSpace(limit) == "" {
			refuse(RuleLimits, "limitation %d is empty", index+1)
		}
	}

	// The document is read by people and by the repository: an address in it is
	// a real address in a tracked file.
	for _, address := range emailPattern.FindAllString(text, -1) {
		if !inReservedZone(address) {
			refuse(RulePII, "the document carries %q, which is not in a reserved domain", address)
		}
	}
	return violations
}

// judgeAreas compares the register's list with the catalogue, and checks each
// area's command, evidence and status.
func judgeAreas(document register, root string) []Violation {
	var violations []Violation
	refuse := func(format string, arguments ...any) {
		violations = append(violations, Violation{Rule: RuleAreaSet, Detail: fmt.Sprintf(format, arguments...)})
	}

	if len(document.Areas) != len(areaCatalogue) {
		refuse("the register executes %d areas and the phase names %d", len(document.Areas), len(areaCatalogue))
	}
	seen := map[string]bool{}
	targets := makefileTargets(root)
	for _, area := range document.Areas {
		if seen[area.ID] {
			refuse("area %q appears twice", area.ID)
		}
		seen[area.ID] = true

		want, ok := catalogueArea(area.ID)
		if !ok {
			refuse("%q is not an area the phase names", area.ID)
			continue
		}
		if area.Command != want.Command {
			refuse("area %q claims the command %q and the audit executes %q", area.ID, area.Command, want.Command)
		}
		if area.Evidence != want.Evidence {
			refuse("area %q cites %q and the evidence of the audit is %q", area.ID, area.Evidence, want.Evidence)
		}
		if path := strings.TrimPrefix(area.Command, "make "); path != area.Command {
			if !targets[path] {
				violations = append(violations, Violation{Rule: RuleAreaCommand,
					Detail: fmt.Sprintf("area %q runs `make %s` and the Makefile declares no such target", area.ID, path)})
			}
		}
		if _, err := os.Stat(filepath.Join(root, area.Evidence)); err != nil {
			violations = append(violations, Violation{Rule: RuleAreaEvidence,
				Detail: fmt.Sprintf("area %q cites %q, which is not in the tree", area.ID, area.Evidence)})
		}
		if area.Status != StatusPass && area.Status != StatusFinding {
			violations = append(violations, Violation{Rule: RuleAreaStatus,
				Detail: fmt.Sprintf("area %q records status %q, which is not %q or %q", area.ID, area.Status, StatusPass, StatusFinding)})
		}
	}
	for _, want := range areaCatalogue {
		if !seen[want.ID] {
			refuse("the phase names %q and the register does not execute it", want.ID)
		}
	}
	return violations
}

// judgeJourneyGate checks that the journeys are run by a gate that exists.
func judgeJourneyGate(facts journeyFacts, root string) []Violation {
	var violations []Violation
	if len(facts.Specs) == 0 {
		violations = append(violations, Violation{Rule: RuleJourneyGate, Detail: "the suite holds no journey file"})
	}
	for _, spec := range facts.Specs {
		if _, err := os.Stat(filepath.Join(root, "tools", "e2e", "specs", spec)); err != nil {
			violations = append(violations, Violation{Rule: RuleJourneyGate,
				Detail: fmt.Sprintf("the journey %q is not in the tree", spec)})
		}
	}
	if !makefileTargets(root)["test-e2e"] {
		violations = append(violations, Violation{Rule: RuleJourneyGate,
			Detail: "the Makefile declares no test-e2e target: the journeys would be recorded as executed and run by nothing"})
	}
	return violations
}

// judgeFindings checks the vocabulary and what an open or accepted finding
// requires.
func judgeFindings(document register) []Violation {
	var violations []Violation
	seen := map[string]bool{}
	for _, finding := range document.Findings {
		refuse := func(format string, arguments ...any) {
			violations = append(violations, Violation{Rule: RuleFindingVocab, Detail: fmt.Sprintf(format, arguments...)})
		}
		if strings.TrimSpace(finding.ID) == "" {
			refuse("a finding carries no identifier")
			continue
		}
		if seen[finding.ID] {
			refuse("the finding %q appears twice", finding.ID)
		}
		seen[finding.ID] = true

		if !contains(severities, finding.Severity) {
			refuse("finding %s declares severity %q, which is not one of %v", finding.ID, finding.Severity, severities)
		}
		if !contains(findingStatuses, finding.Status) {
			refuse("finding %s declares status %q, which is not one of %v", finding.ID, finding.Status, findingStatuses)
		}
		if strings.TrimSpace(finding.Evidence) == "" {
			refuse("finding %s cites no evidence", finding.ID)
		}
		if _, ok := catalogueArea(finding.Area); !ok {
			refuse("finding %s belongs to area %q, which the phase does not name", finding.ID, finding.Area)
		}
		if finding.Severity == "critical" || finding.Severity == "high" {
			if finding.Status == "open" {
				violations = append(violations, Violation{Rule: RuleFindingBlocker,
					Detail: fmt.Sprintf("finding %s is %s and open: no release carries an open critical or high finding", finding.ID, finding.Severity)})
			}
		}
		if finding.Status == "accepted" {
			for name, value := range map[string]string{
				"an owner": finding.Owner, "the work it implies": finding.Next, "an evidence": finding.Evidence,
			} {
				if strings.TrimSpace(value) == "" {
					violations = append(violations, Violation{Rule: RuleFindingBlocker,
						Detail: fmt.Sprintf("finding %s is accepted and names %s nowhere", finding.ID, name)})
				}
			}
		}
	}
	return violations
}

// judgeReview checks the mandatory human review: what a completed review must
// name, and what a pending one must not omit.
func judgeReview(document register, root string) []Violation {
	var violations []Violation
	review := document.Review
	refuse := func(format string, arguments ...any) {
		violations = append(violations, Violation{Rule: RuleReviewState, Detail: fmt.Sprintf(format, arguments...)})
	}
	if !contains(reviewStates, review.State) {
		refuse("the review of %q declares state %q, which is not one of %v", review.Locale, review.State, reviewStates)
		return violations
	}
	if strings.TrimSpace(review.Locale) == "" {
		refuse("the review names no locale")
	}
	switch review.State {
	case "reviewed":
		if strings.TrimSpace(review.Reviewer) == "" || strings.TrimSpace(review.Date) == "" {
			refuse("the review is recorded as done with no reviewer or no date: a review nobody signed is a review nobody did")
		}
		if _, err := time.Parse("2006-01-02", review.Date); err != nil {
			refuse("the review date %q is not an ISO 8601 day", review.Date)
		}
		if len(review.Scope) == 0 {
			refuse("the review is recorded as done and names no scope: what was read is what was reviewed")
		}
		for _, path := range review.Scope {
			if _, err := os.Stat(filepath.Join(root, path)); err != nil {
				refuse("the review claims to have read %q, which is not in the tree", path)
			}
		}
	case "pending":
		if strings.TrimSpace(review.Decider) == "" {
			refuse("the review of %q is pending and names nobody who decides", review.Locale)
		}
		if len(review.Missing) == 0 {
			refuse("the review of %q is pending and does not say what is missing", review.Locale)
		}
	}
	return violations
}

// catalogueArea returns the area of the catalogue with one identifier.
func catalogueArea(id string) (Area, bool) {
	for _, area := range areaCatalogue {
		if area.ID == id {
			return area, true
		}
	}
	return Area{}, false
}

// documentNames reports whether the register names one key anywhere — as a
// finding's evidence, its detail, or a limitation. Naming it is what makes it
// reviewable in the next audit.
func documentNames(document register, key string) bool {
	for _, finding := range document.Findings {
		if strings.Contains(finding.Evidence, key) || strings.Contains(finding.Detail, key) {
			return true
		}
	}
	for _, limit := range document.Limits {
		if strings.Contains(limit, key) {
			return true
		}
	}
	return false
}

// inReservedZone reports whether an address is in a domain reserved for
// documentation or for synthetic identities.
func inReservedZone(address string) bool {
	at := strings.LastIndex(address, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(address[at+1:])
	for _, zone := range reservedZones {
		if domain == zone || strings.HasSuffix(domain, "."+zone) {
			return true
		}
	}
	return false
}

// makefileTargets returns the targets the Makefile declares, so a command the
// register claims to have run has to exist.
func makefileTargets(root string) map[string]bool {
	targets := map[string]bool{}
	raw, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return targets
	}
	for _, line := range strings.Split(string(raw), "\n") {
		index := strings.Index(line, ":")
		if index <= 0 || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		name := strings.TrimSpace(line[:index])
		if name == "" || strings.ContainsAny(name, " \t=$") {
			continue
		}
		targets[name] = true
	}
	return targets
}

// sortedKeys is a test helper for deterministic iteration over a map.
func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
