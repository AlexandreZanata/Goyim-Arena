package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// root is the repository root, as the test binary sees it.
const root = "../.."

// document is the register under test, and text its prose.
func loadRegister(t *testing.T) (register, string) {
	t.Helper()
	registered, text, err := parseDocument(filepath.Join(root, "docs", "I18N_AUDIT.md"))
	if err != nil {
		t.Fatalf("parse the delivered register: %v", err)
	}
	return registered, text
}

// TestTheDeliveredRegisterIsAccepted is the control: the document that ships and
// the tree it describes agree, so every mutation below is a change of something
// that held.
func TestTheDeliveredRegisterIsAccepted(t *testing.T) {
	facts, err := Measure(root)
	if err != nil {
		t.Fatalf("Measure() error = %v", err)
	}
	registered, text := loadRegister(t)
	if violations := Judge(facts, registered, text, root); len(violations) > 0 {
		for _, violation := range violations {
			t.Errorf("delivered register: %s", violation)
		}
	}
}

// TestAMissingBlockIsRefused covers the shape failures: a document without a
// machine block, and one whose block is not JSON.
func TestAMissingBlockIsRefused(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "register.md")
	if err := os.WriteFile(path, []byte("## Áreas executadas\nsó prosa\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseDocument(path); err == nil {
		t.Fatal("a document without a machine block was accepted")
	}
	if err := os.WriteFile(path, []byte(blockStart+"\n```json\n{not json\n```\n"+blockEnd+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseDocument(path); err == nil {
		t.Fatal("a block that is not JSON was accepted")
	}
}

// TestTheRegisterIsJudgedAgainstItsOwnTree mutates one property at a time. Each
// case names the rule it expects: a mutation that fails for another reason is a
// test that would pass while the rule it names never ran.
func TestTheRegisterIsJudgedAgainstItsOwnTree(t *testing.T) {
	facts, err := Measure(root)
	if err != nil {
		t.Fatalf("Measure() error = %v", err)
	}
	delivered, text := loadRegister(t)

	cases := []struct {
		name    string
		rule    string
		mutate  func(document *register, text *string, facts *Facts)
		skipped string
	}{
		{
			name: "an unknown block version",
			rule: RuleVersion,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Version = "i18n-release/99"
			},
		},
		{
			name: "a date that is not a day",
			rule: RuleDate,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Date = "22/09/2026"
			},
		},
		{
			name: "a document without its prose",
			rule: RuleSections,
			mutate: func(_ *register, text *string, _ *Facts) {
				*text = strings.ReplaceAll(*text, "## Limitações", "## Nada")
			},
		},
		{
			name: "an area the phase does not name",
			rule: RuleAreaSet,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Areas[0].ID = "cobertura-de-catalogo"
			},
		},
		{
			name: "an area whose command is not the one executed",
			rule: RuleAreaSet,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Areas[0].Command = "make test-unit"
			},
		},
		{
			name: "an area citing another evidence",
			rule: RuleAreaSet,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Areas[0].Evidence = "locales/pt-BR/auth.json"
			},
		},
		{
			name: "an area running a target that does not exist",
			rule: RuleAreaCommand,
			mutate: func(document *register, _ *string, _ *Facts) {
				for index := range document.Areas {
					if document.Areas[index].ID == "hardcoded-text" {
						document.Areas[index].Command = "make audit-i18nn"
					}
				}
			},
		},
		{
			name: "an area citing a file that is not in the tree",
			rule: RuleAreaEvidence,
			mutate: func(document *register, _ *string, _ *Facts) {
				for index := range document.Areas {
					if document.Areas[index].ID == "hardcoded-text" {
						document.Areas[index].Evidence = "tools/i18naudit/missing.go"
					}
				}
			},
		},
		{
			name: "an area whose status is not a vocabulary",
			rule: RuleAreaStatus,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Areas[0].Status = "green"
			},
		},
		{
			name: "an area missing from the register",
			rule: RuleAreaSet,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Areas = document.Areas[:len(document.Areas)-1]
			},
		},
		{
			name: "a locale list that disagrees with the catalog",
			rule: RuleClaimCatalog,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Claims.Locales = []string{"pt-BR", "en-US"}
			},
		},
		{
			name: "a namespace list that disagrees with the catalog",
			rule: RuleClaimCatalog,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Claims.Namespaces = []string{"arenas"}
			},
		},
		{
			name: "a message count that disagrees with the tree",
			rule: RuleClaimCatalog,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Claims.KeysPerLocale["en-US"] = 199
			},
		},
		{
			name: "a message count for a locale that does not exist",
			rule: RuleClaimCatalog,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Claims.KeysPerLocale["es-ES"] = 200
			},
		},
		{
			name: "a divergence claim that hides a divergence",
			rule: RuleClaimZero,
			mutate: func(document *register, _ *string, facts *Facts) {
				facts.Catalog.PlaceholderDivergence = []string{"arenas.document.page_title"}
			},
		},
		{
			name: "an empty message the register does not count",
			rule: RuleClaimZero,
			mutate: func(document *register, _ *string, facts *Facts) {
				facts.Catalog.Empty = []string{"pt-BR arenas.participation.brand"}
			},
		},
		{
			name: "an orphan key the register does not name",
			rule: RuleClaimOrphans,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Catalog.Orphans = []string{"arenas.document.ghost_heading"}
			},
		},
		{
			name: "a literal with the shape of a key that the register does not name",
			rule: RuleClaimOrphans,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Catalog.Unknown = []string{"internal/platform/ratelimit/policy.go: auth.unknown_action"}
			},
		},
		{
			name: "an email snapshot that is not committed",
			rule: RuleClaimGoldens,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Emails.Present = 5
				facts.Emails.Missing = []string{"verification.en-US.golden"}
			},
		},
		{
			name: "an error code missing from a locale",
			rule: RuleClaimCodes,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Codes.Drift = []string{"en-US validation"}
			},
		},
		{
			name: "a plural variant count that disagrees",
			rule: RuleClaimPlural,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Claims.PluralVariants = 2
			},
		},
		{
			name: "an hreflang that appeared without an equivalent page",
			rule: RuleClaimHreflang,
			mutate: func(document *register, _ *string, facts *Facts) {
				facts.Search.Hreflang = []string{"internal/arenas/adapters/html/templates.go:12"}
			},
		},
		{
			name: "a value formatted with the browser's own locale",
			rule: RuleClaimHreflang,
			mutate: func(document *register, _ *string, facts *Facts) {
				facts.Search.ToLocaleString = []string{"web/src/pages/participation.ts:40"}
			},
		},
		{
			name: "a cacheable surface varied by the raw header, unnamed",
			rule: RuleClaimCache,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Cache.RawAcceptLanguage = []string{"internal/other/adapters/http/handler.go:9: w.Header().Set(\"Vary\", \"Accept-Language\")"}
			},
		},
		{
			name: "a public cacheable surface count that disagrees",
			rule: RuleClaimCache,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Claims.PublicCacheableSurfaces = 7
			},
		},
		{
			name: "a pseudo-locale that leaked into production",
			rule: RulePseudoMechanic,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Pseudo.InProductionCatalog = true
			},
		},
		{
			name: "journeys covering fewer locales than the product",
			rule: RuleJourneyLocales,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Journeys.Locales = []string{"pt-BR"}
			},
		},
		{
			name: "a content language that drifts between harness and seed",
			rule: RuleJourneyContent,
			mutate: func(_ *register, _ *string, facts *Facts) {
				facts.Journeys.ContentLanguageSeed = "en-US"
			},
		},
		{
			name: "a finding with a severity that is not a level",
			rule: RuleFindingVocab,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Findings[0].Severity = "grave"
			},
		},
		{
			name: "a finding that belongs to no area of the phase",
			rule: RuleFindingVocab,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Findings[0].Area = "whatever"
			},
		},
		{
			name: "a finding with no evidence",
			rule: RuleFindingVocab,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Findings[0].Evidence = ""
			},
		},
		{
			name: "an open critical finding",
			rule: RuleFindingBlocker,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Findings[0].Severity = "critical"
				document.Findings[0].Status = "open"
			},
		},
		{
			name: "an accepted finding with nobody responsible",
			rule: RuleFindingBlocker,
			mutate: func(document *register, _ *string, _ *Facts) {
				for index := range document.Findings {
					if document.Findings[index].Status == "accepted" {
						document.Findings[index].Owner = ""
					}
				}
			},
		},
		{
			name: "a review recorded as done with no reviewer",
			rule: RuleReviewState,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Review.State = "reviewed"
			},
		},
		{
			name: "a review recorded as done over a file that does not exist",
			rule: RuleReviewState,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Review.State = "reviewed"
				document.Review.Reviewer = "revisor"
				document.Review.Date = "2026-09-22"
				document.Review.Scope = []string{"locales/en-US/ghost.json"}
			},
		},
		{
			name: "a pending review that names nobody who decides",
			rule: RuleReviewState,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Review.Decider = ""
			},
		},
		{
			name: "a pending review that does not say what is missing",
			rule: RuleReviewState,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Review.Missing = nil
			},
		},
		{
			name: "fewer limitations than the audit admits",
			rule: RuleLimits,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Limits = document.Limits[:minimumLimits-1]
			},
		},
		{
			name: "a limitation that says nothing",
			rule: RuleLimits,
			mutate: func(document *register, _ *string, _ *Facts) {
				document.Limits[0] = "   "
			},
		},
		{
			name: "a real address inside the document",
			rule: RulePII,
			mutate: func(_ *register, text *string, _ *Facts) {
				*text += "\nquem revisou: ana.silva@corp.example-business.com\n"
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document := cloneRegister(delivered)
			mutated := text
			mutatedFacts := cloneFacts(facts)
			testCase.mutate(&document, &mutated, &mutatedFacts)

			violations := Judge(mutatedFacts, document, mutated, root)
			for _, violation := range violations {
				if violation.Rule == testCase.rule {
					return
				}
			}
			t.Fatalf("want a %s violation, got %v", testCase.rule, violations)
		})
	}
}

// TestTheJourneyGateNeedsItsTarget covers the one rule that reads the Makefile
// rather than the tree: a suite nobody runs is a suite that stops passing
// silently.
func TestTheJourneyGateNeedsItsTarget(t *testing.T) {
	facts, err := Measure(root)
	if err != nil {
		t.Fatalf("Measure() error = %v", err)
	}
	registered, text := loadRegister(t)

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "Makefile"), []byte("verify:\n\ttrue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	violations := Judge(facts, registered, text, directory)
	for _, violation := range violations {
		if violation.Rule == RuleJourneyGate {
			return
		}
	}
	t.Fatalf("want a %s violation for a Makefile without test-e2e, got %v", RuleJourneyGate, violations)
}

// TestTheMeasurementSeesWhatItClaims is the antivacuity guard of the
// measurement itself: a fixture tree with a divergent placeholder, an empty
// message, an orphan key and a literal that names no message has to produce all
// four, or the numbers in the register would be the tool agreeing with itself.
func TestTheMeasurementSeesWhatItClaims(t *testing.T) {
	directory := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(directory, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("locales/pt-BR/arenas.json", `{"arenas":{"brand":"Arena {name}","orphan":"ninguém pede","empty":"  "}}`)
	write("locales/en-US/arenas.json", `{"arenas":{"brand":"Arena","orphan":"nobody asks","empty":"  "}}`)
	write("internal/arenas/adapters/html/handler.go", `package html

func handler() string {
	return "arenas.brand" + "arenas.ghost_heading"
}
`)

	catalog, err := loadCatalog(directory)
	if err != nil {
		t.Fatalf("loadCatalog() error = %v", err)
	}
	references, err := scanDeliveredSource(directory, catalog)
	if err != nil {
		t.Fatalf("scanDeliveredSource() error = %v", err)
	}
	facts := measureCatalog(catalog, references)
	if got := facts.Keys["pt-BR"]; got != 3 {
		t.Fatalf("keys in pt-BR = %d, want 3", got)
	}
	if len(facts.PlaceholderDivergence) != 1 {
		t.Fatalf("placeholder divergence = %v, want the one key whose placeholders differ", facts.PlaceholderDivergence)
	}
	if len(facts.Empty) != 2 {
		t.Fatalf("empty messages = %v, want one per locale", facts.Empty)
	}
	// Both keys nobody reaches are reported: the one with a plausible name and
	// the one the fixture left blank are two different defects, and a measurement
	// that found only the first would be reading the name instead of the tree.
	if !equalStrings(facts.Orphans, []string{"arenas.empty", "arenas.orphan"}) {
		t.Fatalf("orphans = %v, want the two keys no file reaches", facts.Orphans)
	}
	if facts.Referenced != 1 {
		t.Fatalf("referenced keys = %d, want the one the fixture spells out", facts.Referenced)
	}
	if len(facts.Unknown) != 1 || !strings.Contains(facts.Unknown[0], "arenas.ghost_heading") {
		t.Fatalf("unknown references = %v, want the literal no locale declares", facts.Unknown)
	}
	if got := catalog.placeholders["arenas.brand"]; len(got) != 1 || got[0] != "name" {
		t.Fatalf("placeholders of arenas.brand = %v, want [name]", got)
	}
}

// TestTheComposedStemIsReachable covers the rule that keeps the coverage number
// honest: a key completed at runtime from a stem ending in a dot is reached.
func TestTheComposedStemIsReachable(t *testing.T) {
	references := sourceReferences{
		literals: map[string]string{},
		prefixes: map[string]string{"arenas.document.status.": "handler.go"},
	}
	if !reached("arenas.document.status.published", references) {
		t.Fatal("a key composed from a stem was reported as unreachable")
	}
	if reached("arenas.document.gone.title", references) {
		t.Fatal("a key with no stem and no literal was reported as reachable")
	}
}

// cloneRegister deep-copies a register, so a mutation cannot leak into the next
// case.
func cloneRegister(source register) register {
	raw, err := json.Marshal(source)
	if err != nil {
		panic(err)
	}
	var copy register
	if err := json.Unmarshal(raw, &copy); err != nil {
		panic(err)
	}
	return copy
}

// cloneFacts deep-copies a measurement, for the same reason.
func cloneFacts(source Facts) Facts {
	raw, err := json.Marshal(source)
	if err != nil {
		panic(err)
	}
	var copy Facts
	if err := json.Unmarshal(raw, &copy); err != nil {
		panic(err)
	}
	return copy
}
