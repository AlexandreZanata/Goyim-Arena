// The register of the internationalization audit (P20-T09).
//
// The document is two things at once, deliberately: prose a person reads, and
// one machine block the audit judges. The prose carries the reasoning and the
// findings; the block carries the claims, so the audit can compare each claim
// with the tree instead of reading a sentence and hoping.
//
// The block is written by the harness that runs the areas, never by hand: an
// execution record a person can type is an execution record that can be typed
// without running anything.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// register is the machine block of the document.
type register struct {
	// Version names the shape of the block, so a reader knows what it is
	// looking at when the audit changes.
	Version string `json:"version"`
	// Date is the day the audit was executed, in ISO 8601.
	Date string `json:"date"`
	// Areas are the properties the phase names, with the execution recorded by
	// the harness.
	Areas []Area `json:"areas"`
	// Claims are the numbers the audit asserts about the catalogs. They are
	// compared with the measurement one by one.
	Claims claims `json:"claims"`
	// Findings are the defects the audit found, each with an owner and the work
	// it implies.
	Findings []finding `json:"findings"`
	// Review is the mandatory human review of the translation, which the audit
	// cannot perform and therefore must be able to see unresolved.
	Review review `json:"review"`
	// Limits are what this audit does not prove.
	Limits []string `json:"limits"`
}

// claims is what the register says the tree measures.
type claims struct {
	Locales                 []string       `json:"locales"`
	Namespaces              []string       `json:"namespaces"`
	KeysPerLocale           map[string]int `json:"keys_per_locale"`
	PlaceholderDivergence   int            `json:"placeholder_divergence"`
	EmptyMessages           int            `json:"empty_messages"`
	OrphanKeys              int            `json:"orphan_keys"`
	UnknownReferences       int            `json:"unknown_references"`
	EmailGoldens            int            `json:"email_goldens"`
	ErrorCodeDrift          int            `json:"error_code_drift"`
	PluralVariants          int            `json:"plural_variants"`
	Hreflang                int            `json:"hreflang"`
	ToLocaleString          int            `json:"to_locale_string"`
	VaryByRawAcceptLanguage int            `json:"vary_by_raw_accept_language"`
	PublicCacheableSurfaces int            `json:"public_cacheable_surfaces"`
	JourneyLocales          []string       `json:"journey_locales"`
}

// finding is one defect, in the vocabulary the phase requires.
type finding struct {
	// ID is stable and unique: a finding a later audit can cite.
	ID string `json:"id"`
	// Severity is one of the levels the register defines.
	Severity string `json:"severity"`
	// Status is `open`, `fixed` or `accepted`.
	Status string `json:"status"`
	// Area is the area of the audit the finding belongs to.
	Area string `json:"area"`
	// Owner is who decides about it, and Next is the work it implies.
	Owner string `json:"owner"`
	Next  string `json:"next"`
	// Evidence is the file, test or command that shows the defect.
	Evidence string `json:"evidence"`
	// Detail states the defect in one sentence.
	Detail string `json:"detail"`
}

// review is the human review of the translation the release requires.
type review struct {
	// Locale is the catalog under review; today only the translated one.
	Locale string `json:"locale"`
	// State is `reviewed` or `pending`. A pending review is not a failure of
	// the audit — it is a statement that the release is not authorized, and it
	// has to name the person who decides and what is missing.
	State    string   `json:"state"`
	Reviewer string   `json:"reviewer"`
	Date     string   `json:"date"`
	Scope    []string `json:"scope"`
	Decider  string   `json:"decider"`
	Missing  []string `json:"missing"`
}

// Severities and statuses the register may use.
var (
	// severities are the levels, in the order the register reports them.
	severities = []string{"critical", "high", "medium", "low"}
	// findingStatuses are the states a finding may be in.
	findingStatuses = []string{"open", "fixed", "accepted"}
	// reviewStates are the states the human review may be in.
	reviewStates = []string{"reviewed", "pending"}
)

// blockStart and blockEnd delimit the machine block inside the document.
const (
	blockStart = "<!-- i18n-release:facts -->"
	blockEnd   = "<!-- /i18n-release:facts -->"
)

// requiredSections are the headings a reader needs to find in the document.
// They are prose, and the audit checks they exist because a register of numbers
// with no reading is not a register anybody can act on.
var requiredSections = []string{
	"## Áreas executadas",
	"## O que foi medido",
	"## Findings",
	"## Revisão humana do en-US",
	"## Limitações",
}

// version is the shape of the block this build writes and reads.
const version = "i18n-release/1"

// parseDocument reads the register out of a document.
func parseDocument(path string) (register, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return register{}, "", fmt.Errorf("i18n-release: read %s: %w", path, err)
	}
	text := string(raw)

	start := strings.Index(text, blockStart)
	end := strings.Index(text, blockEnd)
	if start < 0 || end < 0 || end < start {
		return register{}, text, fmt.Errorf("i18n-release: %s holds no machine block between %s and %s",
			path, blockStart, blockEnd)
	}
	block := strings.TrimSpace(text[start+len(blockStart) : end])
	block = strings.TrimPrefix(block, "```json")
	block = strings.TrimSuffix(block, "```")
	block = strings.TrimSpace(block)

	var document register
	if err := json.Unmarshal([]byte(block), &document); err != nil {
		return register{}, text, fmt.Errorf("i18n-release: %s holds a block that is not valid JSON: %w", path, err)
	}
	return document, text, nil
}

// renderBlock writes the machine block of a register, which the harness
// inserts into the document between the markers.
//
// The block is the *register*, not the measurement: the claims, the findings
// and the review are the author's, and a recorder that wrote the measurement
// into them would be answering its own question. Only the areas are written by
// an execution.
func renderBlock(document register) (string, error) {
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", err
	}
	return blockStart + "\n```json\n" + string(raw) + "\n```\n" + blockEnd + "\n", nil
}
