// Command privacyaudit is the privacy and moderation audit gate (P20-T06).
//
// The phase asks for a review of the data lifecycle — export, delete,
// retention, analytics payload, logs, low counts, moderation appeals and
// reversals — run with two synthetic accounts hunting for data mixing, and it
// asks for three answers: the suite passes, the report holds no real PII, and
// the public exports are validated against an allowlist. This command is what
// makes the review re-runnable: it reads the audit register
// (docs/PRIVACY_AUDIT.md), refuses a register that is malformed, that names an
// account outside a reserved domain, that cites a path which does not exist,
// that leaves an area of the phase unaudited, that drifts from the code it
// claims to describe (the export key sets, the retention table, the
// reidentification threshold, the analytics vocabulary), that leaves a Crítica
// or Alta finding open, or that accepts a residual without an owner and a date.
// Then it runs the seven executions.
//
// Two modes, one difference:
//
//	privacyaudit -check   the register is well formed and its claims resolve.
//	                      Exit 0 or 1; used by this tool's tests and by any
//	                      caller that only wants the document judged.
//	privacyaudit          the register is well formed *and* every area ran
//	                      green. This is the phase gate: `make privacy-audit`
//	                      runs it.
//
// It never writes: the fix belongs to whoever owns the residual, in the commit
// that changes the document.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// Exit statuses, as a small vocabulary the tests can name.
const (
	// exitOK is a register whose claims resolve and whose executions passed.
	exitOK = 0
	// exitViolation is a refused register, a failed execution or a leak.
	exitViolation = 1
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command, with its exit status as a value: the gate is a
// contract about exit codes, and a contract that lives inside main is a
// contract no test can hold.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("privacyaudit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root holding the audit register")
	check := flags.Bool("check", false, "judge the register without running the area executions")
	timeout := flags.Duration("timeout", 15*time.Minute, "limit for one area's execution")
	if err := flags.Parse(args); err != nil {
		return exitViolation
	}

	report, err := Audit(Options{Root: *root, Run: !*check, Timeout: *timeout})
	if err != nil {
		fmt.Fprintf(stderr, "privacyaudit: %v\n", err)
		return exitViolation
	}

	counts := report.Counts()
	verdicts := make([]string, 0, len(counts))
	for verdict := range counts {
		verdicts = append(verdicts, verdict)
	}
	sort.Strings(verdicts)

	fmt.Fprintf(stdout, "privacyaudit: %d account(s), %d allowlist(s) covering %d key(s), %d area(s):",
		len(report.Document.Register.Accounts), len(report.Document.Register.Allowlists),
		report.Keys, len(report.Document.Register.Areas))
	for _, verdict := range verdicts {
		fmt.Fprintf(stdout, " %s=%d", verdict, counts[verdict])
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "privacyaudit: %d retention class(es), %d analytics event(s), low-count threshold %d\n",
		len(report.Document.Register.Retention), len(report.Document.Register.Analytics),
		report.Document.Register.LowCount.Threshold)

	for _, finding := range report.Document.Register.Findings {
		fmt.Fprintf(stdout, "privacyaudit: finding %s (%s, %s): %s\n",
			finding.ID, finding.Severity, finding.Status, finding.Title)
	}
	for _, execution := range report.Executions {
		fmt.Fprintf(stdout, "privacyaudit: %s: ok — %s\n", execution.Area, execution.Command)
	}

	if len(report.Violations) > 0 {
		for _, violation := range report.Violations {
			fmt.Fprintf(stderr, "privacyaudit: %s\n", violation)
		}
		fmt.Fprintf(stderr, "privacyaudit: %d violation(s) — the review is not a review while a claim resolves to nothing, an area is skipped or a run is red\n",
			len(report.Violations))
		return exitViolation
	}

	if *check {
		fmt.Fprintln(stdout, "privacyaudit: the register is well formed and every claim resolves; no execution was run")
		return exitOK
	}
	fmt.Fprintf(stdout, "privacyaudit: %d area(s) green — the audit of the phase stands\n", len(report.Executions))
	return exitOK
}
