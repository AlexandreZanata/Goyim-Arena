// Command releaseverify is the reproducible release verification of the backend
// (P20-T07).
//
// The phase asks for a clean checkout of the current commit, dependencies
// installed from the lockfiles only, the dependencies brought up, `make verify`,
// the image build and a smoke, every command passing twice, a clean working
// tree, no tracked local directory and `git fsck` without error — and for the
// results and the real limitations to be recorded in docs/RELEASE_CHECKLIST.md.
//
// Two halves, one difference: the exercise is tools/releaseverify/verify.sh,
// which runs the commands and writes the measured facts as JSON; this command
// renders the document from those facts and then judges it.
//
//	releaseverify render -facts facts.json -out docs/RELEASE_CHECKLIST.md
//	releaseverify check  -file docs/RELEASE_CHECKLIST.md
//
// `check` is the gate: it refuses a document whose commands did not run twice, a
// run that was red, a tree that was dirty, a tracked local file, a `git fsck`
// error, a lockfile that is missing or bypassed, an image nobody built or smoked,
// a release gate reported as refusing with nothing named, and a checklist with
// no declared limitation. It never writes.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Exit statuses, as a small vocabulary the tests can name.
const (
	// exitOK is a document the phase accepts.
	exitOK = 0
	// exitViolation is a document the rules refused.
	exitViolation = 1
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole command, with its exit status as a value.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "releaseverify: expected a mode: render or check")
		return exitViolation
	}
	switch args[0] {
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "releaseverify: unknown mode %q: expected render or check\n", args[0])
		return exitViolation
	}
}

// runRender turns measured facts into the document.
func runRender(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	factsPath := flags.String("facts", "", "the facts file the run wrote")
	outPath := flags.String("out", documentPath, "where to write the document")
	if err := flags.Parse(args); err != nil {
		return exitViolation
	}
	if *factsPath == "" {
		fmt.Fprintln(stderr, "releaseverify: render needs -facts")
		return exitViolation
	}

	facts, err := ReadFacts(*factsPath)
	if err != nil {
		fmt.Fprintf(stderr, "releaseverify: %v\n", err)
		return exitViolation
	}
	document, err := Render(facts)
	if err != nil {
		fmt.Fprintf(stderr, "releaseverify: %v\n", err)
		return exitViolation
	}
	if err := os.WriteFile(*outPath, []byte(document), 0o644); err != nil {
		fmt.Fprintf(stderr, "releaseverify: %v\n", err)
		return exitViolation
	}
	fmt.Fprintf(stdout, "releaseverify: %s rendered from %s (%d command(s), %d lockfile(s))\n",
		*outPath, *factsPath, len(facts.Commands), len(facts.Lockfiles))
	return exitOK
}

// runCheck judges the delivered document.
func runCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root the document belongs to")
	file := flags.String("file", documentPath, "the checklist to judge")
	if err := flags.Parse(args); err != nil {
		return exitViolation
	}

	violations, err := CheckFile(*root, *file)
	if err != nil {
		fmt.Fprintf(stderr, "releaseverify: %v\n", err)
		return exitViolation
	}

	resolved := *file
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(*root, resolved)
	}
	document, err := ReadDocument(resolved)
	if err != nil {
		fmt.Fprintf(stderr, "releaseverify: %v\n", err)
		return exitViolation
	}
	facts := document.Facts
	fmt.Fprintf(stdout, "releaseverify: commit %s on %s, %d command(s), %d lockfile(s), %d limitation(s)\n",
		short(facts.Commit), facts.Branch, len(facts.Commands), len(facts.Lockfiles), len(facts.Limits))
	fmt.Fprintf(stdout, "releaseverify: image %s (%s), smoke %s\n",
		facts.Image.Reference, short(facts.Image.Digest), facts.Image.Smoke)
	if facts.Governance.Blocked {
		open := append([]string(nil), facts.Governance.Open...)
		sort.Strings(open)
		fmt.Fprintf(stdout, "releaseverify: the release gate refuses, with %d open decision(s): %v\n",
			len(open), open)
	} else {
		fmt.Fprintln(stdout, "releaseverify: the release gate does not refuse")
	}

	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintf(stderr, "releaseverify: %s\n", violation)
		}
		fmt.Fprintf(stderr, "releaseverify: %d violation(s) — a checklist is not a checklist while a command ran once, a run was red, the tree was dirty or a limitation is missing\n",
			len(violations))
		return exitViolation
	}
	fmt.Fprintln(stdout, "releaseverify: every command passed twice, the tree is clean, the local plan is untracked and fsck is quiet")
	return exitOK
}

// short clips an identifier for the summary line.
func short(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
