// Command governanceaudit is the release gate of the launch decisions
// (P20-T01).
//
// The phase asks that each blocking human decision be either decided in a
// versioned document or blocked in a way that prevents the release. This
// command is what makes the second half true: it reads the register
// (docs/GOVERNANCE.md), refuses a register that is malformed, and — unless
// -check is given — fails while any item is blocked, printing the decision that
// is needed, who owes it and what it prevents.
//
// Two modes, one difference:
//
//	governanceaudit -check     the register is well formed (schema, required
//	                           items, prose in agreement). Exit 0 or 1; used by
//	                           this tool's tests and by any caller that only
//	                           wants the document judged.
//	governanceaudit            the register is well formed *and* nothing is
//	                           blocked. This is the release gate: `make
//	                           release-gate` runs it, and it stays red until the
//	                           owner decides what is missing.
//
// It never writes: the decision belongs to the owner of the product, in the
// commit that changes the document.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// Exit statuses, as a small vocabulary the tests can name.
const (
	// exitOK is a closed register, or a well-formed one judged with -check.
	exitOK = 0
	// exitBlocked is the status of a well-formed register that still holds
	// open decisions. It is deliberately the same as a violation: for a
	// release gate, "the document is fine but the release is not authorized"
	// is a failure.
	exitBlocked = 1
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command, with its exit status as a value: the gate is a
// contract about exit codes, and a contract that only exists inside main is a
// contract no test can hold.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("governanceaudit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root holding docs/GOVERNANCE.md")
	file := flags.String("file", "docs/GOVERNANCE.md", "the governance register, relative to -root")
	check := flags.Bool("check", false, "judge the register without failing on blocked items")
	if err := flags.Parse(args); err != nil {
		return exitBlocked
	}

	path := *root + string(os.PathSeparator) + *file
	report, err := Audit(path)
	if err != nil {
		fmt.Fprintf(stderr, "governanceaudit: %v\n", err)
		return exitBlocked
	}

	for _, item := range report.Items {
		state := item.Status
		if item.Status == statusBlocked {
			state = "BLOQUEIO"
		}
		fmt.Fprintf(stdout, "governanceaudit: %s — %s\n", item.ID, state)
	}
	fmt.Fprintf(stdout, "governanceaudit: %d decision(s) of the phase, %d blocked, %d pending step(s)\n",
		len(report.Items), len(report.Blocked), countPending(report.Items))

	for _, item := range report.Items {
		for _, step := range item.Pending {
			fmt.Fprintf(stdout, "governanceaudit: pending (%s): %s\n", item.ID, step)
		}
	}

	for _, finding := range report.Findings {
		fmt.Fprintf(stderr, "governanceaudit: %s\n", finding)
	}
	if len(report.Findings) > 0 {
		fmt.Fprintf(stderr, "governanceaudit: %d violation(s) — the register is not a register until every item is present, complete and agreeing with its prose\n", len(report.Findings))
		return exitBlocked
	}

	if len(report.Blocked) == 0 {
		fmt.Fprintf(stdout, "governanceaudit: every launch decision of the phase is decided\n")
		return exitOK
	}
	if *check {
		fmt.Fprintf(stdout, "governanceaudit: register is well formed; %d decision(s) remain open\n", len(report.Blocked))
		return exitOK
	}

	for _, item := range report.Blocked {
		fmt.Fprintf(stderr, "governanceaudit: BLOQUEIO %s — precisa: %s (decisor: %s); impede: %s\n",
			item.ID, item.Blocker.Needs, item.Blocker.Owner, item.Blocker.Blocks)
	}
	fmt.Fprintf(stderr, "governanceaudit: %d open decision(s) — a release is not authorized while the register is open\n", len(report.Blocked))
	return exitBlocked
}

func countPending(items []Item) int {
	total := 0
	for _, item := range items {
		total += len(item.Pending)
	}
	return total
}
