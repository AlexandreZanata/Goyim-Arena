// Command drillaudit is the disaster-and-load drill of the release (P20-T05).
//
// The phase asks that a backup be restored in an isolated environment, that the
// application come up on what came back, that the financial integrity be
// checked, that RPO and RTO be measured, and that a load baseline be registered
// without a critical error — with the email and payment providers unavailable
// along the way. The exercise itself is `tools/drillaudit/verify.sh`, because it
// owns containers, ports and processes; this command owns the parts that must be
// values instead of narrative:
//
//	snapshot  read the financial state of a cluster and write it down;
//	verify    read it again after the recovery and judge the two readings;
//	report    render docs/DISASTER_DRILL.md from what was measured;
//	check     judge that document: the ledger came back equal, nothing was
//	          created, the RPO is inside the bound the server declares and
//	          inside the target the release publishes, the RTO is inside its
//	          target, every load threshold is the one the versioned workload
//	          declares and none was crossed, and both provider outages were
//	          exercised in both directions.
//
// It never writes outside the paths it is given, and it never invents a number:
// everything it prints came from a cluster, a command or a file it read.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Exit statuses, as a small vocabulary the tests and the exercise can name.
const (
	// exitOK is a drill whose claims hold.
	exitOK = 0
	// exitRefused is a refused comparison, a refused report or a read that
	// could not be made: for a gate, "nothing could be read" is not a pass.
	exitRefused = 1
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command, with its exit status as a value: the gate is a
// contract about exit codes, and a contract that lives inside main is a
// contract no test can hold.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: drillaudit <snapshot|verify|report|check> [flags]")
		return exitRefused
	}
	switch args[0] {
	case "snapshot":
		return runSnapshot(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "report":
		return runReport(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "drillaudit: unknown command %q\n", args[0])
		return exitRefused
	}
}

// open connects to the DSN with the standard library's own driver name.
func open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// displayName renders a DSN for a report without its credential: a report is a
// versioned document, and a document is not a place for a password.
func displayName(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Host == "" {
		return "the cluster"
	}
	user := ""
	if parsed.User != nil {
		user = parsed.User.Username() + "@"
	}
	return fmt.Sprintf("%s%s%s", user, parsed.Host, parsed.Path)
}

func runSnapshot(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dsn := flags.String("dsn", "", "the DSN of the cluster to read")
	out := flags.String("out", "", "where to write the reading (default: stdout)")
	if err := flags.Parse(args); err != nil {
		return exitRefused
	}
	if *dsn == "" {
		fmt.Fprintln(stderr, "drillaudit snapshot: -dsn is required")
		return exitRefused
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(stderr, "drillaudit snapshot: %v\n", err)
		return exitRefused
	}
	defer db.Close()

	reading, err := ReadLedger(ctx, db, displayName(*dsn))
	if err != nil {
		fmt.Fprintf(stderr, "drillaudit snapshot: %v\n", err)
		return exitRefused
	}
	for _, violation := range reading.Ledger.Violations() {
		fmt.Fprintf(stderr, "drillaudit snapshot: the cluster already breaks an invariant: %s\n", violation)
		return exitRefused
	}
	if code := writeJSON(*out, reading, stdout, stderr); code != exitOK {
		return code
	}
	fmt.Fprintf(stdout, "drillaudit: the baseline holds %d account(s), %d wallet(s), %d operation(s) and %d transaction(s) (totals FREE_INK=%d PURCHASED_INK=%d)\n",
		reading.Ledger.Accounts, len(reading.Ledger.Wallets), reading.Ledger.Operations, reading.Ledger.Transactions,
		reading.Ledger.FreeINK, reading.Ledger.PurchasedINK)
	return exitOK
}

// runVerify reads the restored cluster and judges it against the baseline: the
// invariants of the schema on what came back, and the equality of the two
// readings. What it writes is the comparison, so the report can carry it
// verbatim.
func runVerify(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dsn := flags.String("dsn", "", "the DSN of the restored cluster")
	baselinePath := flags.String("baseline", "", "the reading taken before the loss")
	out := flags.String("out", "", "where to write the comparison (default: stdout)")
	if err := flags.Parse(args); err != nil {
		return exitRefused
	}
	if *dsn == "" || *baselinePath == "" {
		fmt.Fprintln(stderr, "drillaudit verify: -dsn and -baseline are required")
		return exitRefused
	}

	var baseline Reading
	if err := readJSON(*baselinePath, &baseline); err != nil {
		fmt.Fprintf(stderr, "drillaudit verify: %v\n", err)
		return exitRefused
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := open(ctx, *dsn)
	if err != nil {
		fmt.Fprintf(stderr, "drillaudit verify: %v\n", err)
		return exitRefused
	}
	defer db.Close()

	restored, err := ReadLedger(ctx, db, displayName(*dsn))
	if err != nil {
		fmt.Fprintf(stderr, "drillaudit verify: %v\n", err)
		return exitRefused
	}

	violations := append(restored.Ledger.Violations(), Compare(baseline.Ledger, restored.Ledger)...)
	comparison := Comparison{Baseline: baseline, Restored: restored, Violations: violations}
	if code := writeJSON(*out, comparison, stdout, stderr); code != exitOK {
		return code
	}

	for _, violation := range violations {
		fmt.Fprintf(stderr, "drillaudit verify: %s\n", violation)
	}
	fmt.Fprintf(stdout, "drillaudit: %d transaction(s) hashing to %s came back, against %s before the loss, with %d violation(s)\n",
		restored.Ledger.Transactions, orNone(restored.Ledger.LedgerDigest), orNone(baseline.Ledger.LedgerDigest), len(violations))
	if len(violations) > 0 {
		return exitRefused
	}
	return exitOK
}

// Comparison is what one verify produced.
type Comparison struct {
	Baseline   Reading     `json:"baseline"`
	Restored   Reading     `json:"restored"`
	Violations []Violation `json:"violations"`
}

// runReport renders the report from the measured facts and the comparison.
func runReport(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	factsPath := flags.String("facts", "", "the facts the exercise measured")
	comparisonPath := flags.String("comparison", "", "the comparison verify produced")
	out := flags.String("out", "", "where to write the report (default: stdout)")
	if err := flags.Parse(args); err != nil {
		return exitRefused
	}
	if *factsPath == "" || *comparisonPath == "" {
		fmt.Fprintln(stderr, "drillaudit report: -facts and -comparison are required")
		return exitRefused
	}

	var facts Facts
	if err := readJSON(*factsPath, &facts); err != nil {
		fmt.Fprintf(stderr, "drillaudit report: %v\n", err)
		return exitRefused
	}
	var comparison Comparison
	if err := readJSON(*comparisonPath, &comparison); err != nil {
		fmt.Fprintf(stderr, "drillaudit report: %v\n", err)
		return exitRefused
	}
	// The comparison is what the report carries, and the facts carry everything
	// else: the report can never state a financial figure the exercise did not
	// read out of a cluster.
	facts.Baseline = comparison.Baseline
	facts.Restored = comparison.Restored
	facts.Violations = comparison.Violations

	rendered := Render(facts)
	if *out == "" || *out == "-" {
		fmt.Fprint(stdout, rendered)
		return exitOK
	}
	if err := os.WriteFile(*out, []byte(rendered), 0o644); err != nil {
		fmt.Fprintf(stderr, "drillaudit report: %v\n", err)
		return exitRefused
	}
	fmt.Fprintf(stdout, "drillaudit: wrote %s (%d bytes)\n", *out, len(rendered))
	return exitOK
}

// runCheck is the gate: it judges the rendered report.
func runCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", documentPath, "the report to judge")
	if err := flags.Parse(args); err != nil {
		return exitRefused
	}

	document, err := readReport(*file)
	if err != nil {
		fmt.Fprintf(stderr, "drillaudit: %v\n", err)
		return exitRefused
	}
	violations := Check(document)
	for _, violation := range violations {
		fmt.Fprintf(stderr, "drillaudit: %s: %s\n", *file, violation)
	}
	if len(violations) > 0 {
		fmt.Fprintf(stderr, "drillaudit: %d violation(s) — the exercise is not evidence while a number is missing, a threshold was crossed or the ledger did not come back equal\n", len(violations))
		return exitRefused
	}
	facts := document.Facts
	fmt.Fprintf(stdout, "drillaudit: RPO %ds (bound %ds) · RTO %ds to writable and %ds to the app (target %ds) · %d transaction(s) equal to the baseline · %d load threshold(s), none crossed\n",
		facts.RPO.ObservedSeconds, facts.RPO.BoundSeconds, facts.RTO.ToWritableSeconds, facts.RTO.ToAppSeconds,
		facts.RTO.TargetSeconds, facts.Baseline.Ledger.Transactions, len(facts.Load.Thresholds))
	return exitOK
}

// writeJSON writes value to path, or to stdout when path is empty.
func writeJSON(path string, value any, stdout, stderr io.Writer) int {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "drillaudit: %v\n", err)
		return exitRefused
	}
	encoded = append(encoded, '\n')
	if path == "" || path == "-" {
		if _, err := stdout.Write(encoded); err != nil {
			fmt.Fprintf(stderr, "drillaudit: %v\n", err)
			return exitRefused
		}
		return exitOK
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		fmt.Fprintf(stderr, "drillaudit: %v\n", err)
		return exitRefused
	}
	return exitOK
}

// readJSON reads a JSON document from disk.
func readJSON(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
