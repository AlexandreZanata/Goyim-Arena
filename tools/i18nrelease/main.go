// Command i18nrelease audits the internationalization of the release (P20-T09).
//
// Four modes, one per half of the task:
//
//	i18nrelease -measure                 measure the tree and print it as JSON
//	i18nrelease -run -document D         execute every area, record it in D
//	i18nrelease -check -document D       judge the register against the tree
//	i18nrelease -block                   print the machine block alone
//
// `-run` is the audit itself: it executes the gates the phase names, refuses a
// red one, and records the execution. It never fills in a claim — the numbers
// of the catalogs, the findings and the review are written by the author of the
// register, and `-check`, which is what `make i18n-audit` runs, compares them
// with a measurement taken now. A tool that wrote its own claims would be
// comparing itself with itself.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "i18nrelease: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("i18nrelease", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root to audit")
	document := flags.String("document", filepath.Join("docs", "I18N_AUDIT.md"), "register to read or record into")
	measure := flags.Bool("measure", false, "measure the tree and print what it holds")
	execute := flags.Bool("run", false, "execute every area and record the executions in the register")
	check := flags.Bool("check", false, "judge the register against the tree")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	selected := 0
	for _, enabled := range []bool{*measure, *execute, *check} {
		if enabled {
			selected++
		}
	}
	if selected != 1 {
		return fmt.Errorf("select exactly one of -measure, -run, -check")
	}

	switch {
	case *measure:
		facts, err := Measure(*root)
		if err != nil {
			return err
		}
		rendered, err := renderFacts(facts)
		if err != nil {
			return err
		}
		fmt.Print(rendered)
		return nil
	case *execute:
		return record(*root, *document)
	default:
		return judge(*root, *document)
	}
}

// renderFacts prints a measurement as the register's claims shape, so the
// author of the register copies numbers instead of computing them.
func renderFacts(facts Facts) (string, error) {
	var builder strings.Builder
	fmt.Fprintf(&builder, "commit: %s\nmeasured: %s\n", facts.Commit, facts.Measured)
	fmt.Fprintf(&builder, "locales: %v\nnamespaces: %v\n", facts.Catalog.Locales, facts.Catalog.Namespaces)
	for _, locale := range sortedKeys(facts.Catalog.Keys) {
		fmt.Fprintf(&builder, "keys[%s]: %d\n", locale, facts.Catalog.Keys[locale])
	}
	fmt.Fprintf(&builder, "placeholder_divergence: %d\n", len(facts.Catalog.PlaceholderDivergence))
	fmt.Fprintf(&builder, "empty_messages: %d\n", len(facts.Catalog.Empty))
	fmt.Fprintf(&builder, "orphan_keys: %d %v\n", len(facts.Catalog.Orphans), facts.Catalog.Orphans)
	fmt.Fprintf(&builder, "unknown_references: %d %v\n", len(facts.Catalog.Unknown), facts.Catalog.Unknown)
	fmt.Fprintf(&builder, "referenced_keys: %d (literals %d, prefixes %d)\n",
		facts.Catalog.Referenced, facts.Catalog.Literals, facts.Catalog.Prefixes)
	fmt.Fprintf(&builder, "email_goldens: %d of %d (missing %v)\n", facts.Emails.Present, facts.Emails.Expected, facts.Emails.Missing)
	for _, locale := range sortedKeys(facts.Catalog.Keys) {
		fmt.Fprintf(&builder, "error_codes[%s]: %v\n", locale, facts.Codes.Codes[locale])
	}
	fmt.Fprintf(&builder, "error_code_drift: %v\n", facts.Codes.Drift)
	fmt.Fprintf(&builder, "plural_variants: %d %v\n", facts.Plural.Variants, facts.Plural.Keys)
	fmt.Fprintf(&builder, "public_cacheable_surfaces: %d\n", facts.Cache.PublicSurfaces)
	fmt.Fprintf(&builder, "vary_by_raw_accept_language: %v\n", facts.Cache.RawAcceptLanguage)
	fmt.Fprintf(&builder, "hreflang: %d %v\n", len(facts.Search.Hreflang), facts.Search.Hreflang)
	fmt.Fprintf(&builder, "to_locale_string: %d %v\n", len(facts.Search.ToLocaleString), facts.Search.ToLocaleString)
	fmt.Fprintf(&builder, "pseudo: enabled=%s disabled=%s in_production=%t\n",
		facts.Pseudo.Enabled, facts.Pseudo.Disabled, facts.Pseudo.InProductionCatalog)
	fmt.Fprintf(&builder, "journey_locales: %v\njourney_specs: %v\n", facts.Journeys.Locales, facts.Journeys.Specs)
	fmt.Fprintf(&builder, "content_language: harness=%s seed=%s\n",
		facts.Journeys.ContentLanguageHarness, facts.Journeys.ContentLanguageSeed)
	return builder.String(), nil
}

// judge measures the tree, reads the register and reports every property that
// does not hold. It never rewrites anything: the fix belongs to the author.
func judge(root, document string) error {
	facts, err := Measure(root)
	if err != nil {
		return err
	}
	registered, text, err := parseDocument(filepath.Join(root, document))
	if err != nil {
		return err
	}
	violations := Judge(facts, registered, text, root)
	for _, violation := range violations {
		fmt.Fprintf(os.Stderr, "i18nrelease: %s\n", violation)
	}
	if len(violations) > 0 {
		return fmt.Errorf("%d violation(s) — the register and the tree disagree", len(violations))
	}
	fmt.Printf("i18nrelease: %s agrees with the tree (%d areas, %d finding(s), review %s)\n",
		document, len(registered.Areas), len(registered.Findings), registered.Review.State)
	return nil
}

// record executes every area the phase names, refuses a red one, and writes the
// executions into the register.
//
// It refuses to record a failure: an area that answered non-zero stops the run
// with its own output on the terminal, because an audit that recorded a red
// gate and went on would be a register of intentions.
func record(root, document string) error {
	path := filepath.Join(root, document)
	registered, text, err := parseDocument(path)
	if err != nil {
		return err
	}

	recorded := make([]Area, 0, len(areaCatalogue))
	for _, area := range areaCatalogue {
		fmt.Fprintf(os.Stderr, "i18nrelease: %s: %s\n", area.ID, area.Command)
		started := time.Now()
		command := exec.Command("bash", "-lc", area.Command)
		command.Dir = root
		command.Stdout = os.Stderr
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("area %s is red (%s): %w", area.ID, area.Command, err)
		}
		area.Status = StatusPass
		area.Seconds = time.Since(started).Seconds()
		recorded = append(recorded, area)
	}
	registered.Areas = recorded
	registered.Date = time.Now().UTC().Format("2006-01-02")

	block, err := renderBlock(registered)
	if err != nil {
		return err
	}
	start := strings.Index(text, blockStart)
	end := strings.Index(text, blockEnd)
	if start < 0 || end < 0 {
		return fmt.Errorf("%s holds no machine block", document)
	}
	updated := text[:start] + strings.TrimSuffix(block, "\n") + text[end+len(blockEnd):]
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	fmt.Printf("i18nrelease: %d area(s) executed and recorded in %s\n", len(recorded), document)
	return nil
}
