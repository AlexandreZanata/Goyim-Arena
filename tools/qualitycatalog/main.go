// Command qualitycatalog is the executable catalog of the product's rules
// (P21-T02).
//
// It reads the catalog of approved rules and judges every claim it makes
// against the checkout: the document each rule cites, the packages it names,
// the tests it says exist, and the evidence slots its risk class demands. It
// never writes and it never repairs: a reference that does not resolve is the
// caller's to fix, and the exit status is the whole output a pipeline needs.
//
// The companion document, quality/catalog.schema.json, is held to the same
// loader by the tests: a schema that promised less than the loader enforces
// would be a second contract, and two contracts that disagree are worse than
// none.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Exit statuses, as a vocabulary the tests can name.
const (
	exitOK        = 0
	exitViolation = 1
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command, with its status as a value: the contract of a gate
// is about exit statuses, and a contract that only lives inside main is one no
// test can hold.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("qualitycatalog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root the catalog is judged against")
	catalogPath := flags.String("catalog", filepath.Join("quality", "catalog.json"), "catalog to judge, relative to the root")
	if err := flags.Parse(args); err != nil {
		return exitViolation
	}

	catalog, violations := ReadCatalog(*root, *catalogPath)
	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintf(stderr, "qualitycatalog: %s\n", violation)
		}
		fmt.Fprintf(stderr, "qualitycatalog: %d violation(s) — a rule whose evidence does not exist is a rule nobody enforces\n", len(violations))
		return exitViolation
	}

	fmt.Fprintf(stdout, "qualitycatalog: %d rule(s) of %s judged against the checkout\n", len(catalog.Rules), *catalogPath)
	return exitOK
}
