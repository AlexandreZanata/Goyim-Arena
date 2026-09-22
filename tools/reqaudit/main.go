// Command reqaudit is the traceability gate of the MVP requirements
// (P20-T02).
//
// It reads docs/REQUIREMENTS.md and resolves every reference it makes — the
// route against the served contract, the use case and the migration against the
// files that exist, the test against the function that is really there — plus
// the two directions of the coverage table and the ten business invariants.
//
// It is the gate of `make verify`: a matrix whose links rotted fails the merge
// instead of misleading the next reader. It never writes: the correction
// belongs to whoever changed the code or the document.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// Exit statuses, as a vocabulary the tests can name.
const (
	exitOK    = 0
	exitAudit = 1
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command, with its exit status as a value: the gate is a
// contract about exit codes, and a contract that only exists inside main is one
// no test can hold.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("reqaudit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root holding docs/REQUIREMENTS.md")
	if err := flags.Parse(args); err != nil {
		return exitAudit
	}

	report, err := Audit(*root)
	if err != nil {
		fmt.Fprintf(stderr, "reqaudit: %v\n", err)
		return exitAudit
	}

	fmt.Fprintf(stdout, "reqaudit: %d requirement(s), %d invariant(s), %d coverage row(s), %d non-requirement(s)\n",
		len(report.Requirements), len(report.Invariants), len(report.Coverage), len(report.NonRequirements))
	fmt.Fprintf(stdout, "reqaudit: the served contract declares %d operation(s), and every route cited by the matrix was resolved against it\n", report.Operations)
	if len(report.MissedMVPItems) == 0 {
		fmt.Fprintf(stdout, "reqaudit: every item of docs/MVP.md is covered\n")
	}

	for _, finding := range report.Findings {
		fmt.Fprintf(stderr, "reqaudit: %s\n", finding)
	}
	if len(report.Findings) > 0 {
		fmt.Fprintf(stderr, "reqaudit: %d violation(s) — a reference that does not resolve is a requirement that is not traced\n", len(report.Findings))
		return exitAudit
	}
	return exitOK
}
