// Command ciaudit refuses the CI workflows that do not verify what the plan
// requires (P19-T08).
//
// It is the gate `make audit-ci` runs, inside `make verify`, and its exit
// status is the answer: 0 when every gate of the phase is wired to a job, 1
// when one is missing or the wiring broke a rule. Findings print as
// `path:line: rule: detail`, so the fix is a matter of opening the file, and
// the report prints what it read first — a workflow set that shrank is a
// finding of its own.
//
// It never writes: the fix belongs to the author, in the commit that changed
// the workflow.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root holding .github/workflows and the Makefile")
	flag.Parse()

	report, err := Audit(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ciaudit: %v\n", err)
		os.Exit(1)
	}

	for _, name := range report.Workflows {
		fmt.Printf("ciaudit: read %s\n", name)
	}
	fmt.Printf("ciaudit: %d gate(s) required by the phase\n", report.Gates)

	for _, finding := range report.Findings {
		fmt.Fprintf(os.Stderr, "ciaudit: %s\n", finding)
	}
	if len(report.Findings) > 0 {
		fmt.Fprintf(os.Stderr, "ciaudit: %d violation(s) — every gate the phase requires must run in the CI, skip drafts and never skip anything else\n", len(report.Findings))
		os.Exit(1)
	}
	fmt.Printf("ciaudit: %s is clean (0 violations)\n", *root)
}
