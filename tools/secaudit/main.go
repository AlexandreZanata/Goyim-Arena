// Command secaudit is the pre-release security audit gate (P20-T04).
//
// The phase asks that the threat model be exercised — the twelve areas of
// negative authorization, cache, CSRF, sessions, MFA, Stripe, double spend,
// IDOR, secrets, dependencies and container — and that the trust boundaries be
// reviewed by hand. This command is what makes the result re-runnable: it reads
// the audit register (docs/SECURITY_AUDIT.md), refuses a register that is
// malformed, that cites what does not exist, that leaves a threat of the model
// unaudited or re-graded, that skips an area, that defers to a job no workflow
// defines, that leaves a Crítica or Alta finding open, and that accepts a
// residual without an owner and a date. Then it runs the twelve executions.
//
// Two modes, one difference:
//
//	secaudit -check   the register is well formed and its claims resolve.
//	                  Exit 0 or 1; used by this tool's tests and by any caller
//	                  that only wants the document judged.
//	secaudit          the register is well formed *and* every area ran green.
//	                  This is the phase gate: `make security-audit` runs it.
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
	flags := flag.NewFlagSet("secaudit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root holding the audit register")
	check := flags.Bool("check", false, "judge the register without running the area executions")
	timeout := flags.Duration("timeout", 15*time.Minute, "limit for one area's execution")
	if err := flags.Parse(args); err != nil {
		return exitViolation
	}

	report, err := Audit(Options{Root: *root, Run: !*check, Timeout: *timeout})
	if err != nil {
		fmt.Fprintf(stderr, "secaudit: %v\n", err)
		return exitViolation
	}

	counts := report.Counts()
	verdicts := make([]string, 0, len(counts))
	for verdict := range counts {
		verdicts = append(verdicts, verdict)
	}
	sort.Strings(verdicts)

	fmt.Fprintf(stdout, "secaudit: %d threat(s) of the model, %d of them audited:",
		report.Threats, len(report.Document.Register.Threats))
	for _, verdict := range verdicts {
		fmt.Fprintf(stdout, " %s=%d", verdict, counts[verdict])
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "secaudit: %d area(s) of the phase, %d run, %d file(s) scanned for secrets\n",
		len(report.Document.Register.Areas), len(report.Executions), report.Tracked)

	for _, finding := range report.Document.Register.Findings {
		fmt.Fprintf(stdout, "secaudit: finding %s (%s, %s): %s\n",
			finding.ID, finding.Severity, finding.Status, finding.Title)
	}
	for _, execution := range report.Executions {
		fmt.Fprintf(stdout, "secaudit: %s: ok — %s\n", execution.Area, execution.Command)
	}

	if len(report.Violations) > 0 {
		for _, violation := range report.Violations {
			fmt.Fprintf(stderr, "secaudit: %s\n", violation)
		}
		fmt.Fprintf(stderr, "secaudit: %d violation(s) — the audit is not an audit while a claim resolves to nothing, an area is skipped or a run is red\n",
			len(report.Violations))
		return exitViolation
	}

	if *check {
		fmt.Fprintln(stdout, "secaudit: the register is well formed and every claim resolves; no execution was run")
		return exitOK
	}
	fmt.Fprintf(stdout, "secaudit: %d area(s) green — the audit of the phase stands\n", len(report.Executions))
	return exitOK
}
