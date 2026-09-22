// Command handoffaudit judges the README handoff (P20-T08).
//
// Two modes, and both of them are the phase's validation made mechanical:
//
//	handoffaudit check        reads README.md and refuses every claim the tree
//	                          contradicts — a `make` target that does not exist,
//	                          a variable outside the environment template, a
//	                          path that is not there, a subcommand the binary
//	                          does not have — plus the declarations the
//	                          walkthrough needs;
//	handoffaudit walkthrough  checks out the commit into a worktree, copies the
//	                          document and this tool over it (declared, with
//	                          digests) and runs the commands the document itself
//	                          declares, then starts the server the document
//	                          declares and smokes the surfaces it names.
//
// The exit code is the contract: zero when the document and the tree agree and
// the walkthrough ran, non-zero with the violation named otherwise. Nothing
// here rewrites the document: the correction belongs to whoever changed the
// code or wrote the page.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = `handoffaudit checks the README against the tree (P20-T08).

Usage:

  handoffaudit check       [-root .] [-file README.md]
  handoffaudit walkthrough [-root .] [-file README.md]

check reads the document and the files that judge it (Makefile, .env.example,
the binary's usage) and exits non-zero with each violation named.

walkthrough checks out the commit into a worktree, follows the quickstart and
the serve blocks the document declares, and smokes the surfaces the document
names. It needs git, Go, npm, Docker with a reachable daemon, and the address
the serve block uses free on this machine.`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, errUsage) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

var errUsage = errors.New("handoffaudit: usage")

func run(args []string, stdout, stderr io.Writer) error {
	command := "check"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}

	switch command {
	case "check":
		set := flag.NewFlagSet("check", flag.ContinueOnError)
		set.SetOutput(stderr)
		root := set.String("root", ".", "repository root")
		file := set.String("file", "", "document to judge (default README.md under the root)")
		if err := set.Parse(args); err != nil {
			return errUsage
		}
		return check(*root, documentPath(*root, *file), stdout, stderr)

	case "walkthrough":
		set := flag.NewFlagSet("walkthrough", flag.ContinueOnError)
		set.SetOutput(stderr)
		root := set.String("root", ".", "repository root")
		file := set.String("file", "", "document to follow (default README.md under the root)")
		if err := set.Parse(args); err != nil {
			return errUsage
		}
		return runWalkthrough(*root, documentPath(*root, *file), stdout, stderr)

	case "help", "-h", "--help":
		fmt.Fprintln(stdout, usage)
		return nil

	default:
		fmt.Fprintln(stderr, usage)
		return errUsage
	}
}

// documentPath is the document the modes read.
func documentPath(root, file string) string {
	if file != "" {
		return file
	}
	return filepath.Join(root, "README.md")
}

// check runs every rule and reports what they found.
func check(root, file string, stdout, stderr io.Writer) error {
	facts, err := loadFacts(root, file)
	if err != nil {
		return err
	}
	violations := audit(facts)
	fmt.Fprintf(stdout, "handoffaudit: %s: %d rule(s), %d heading(s), %d fenced block(s)\n",
		file, len(catalogue), len(facts.doc.headings), len(facts.doc.blocks))
	for _, found := range violations {
		fmt.Fprintf(stderr, "handoffaudit: %s: %s\n", found.Rule, found.Detail)
	}
	if len(violations) > 0 {
		return fmt.Errorf("handoffaudit: the document and the tree disagree, with %d violation(s) named above", len(violations))
	}
	fmt.Fprintf(stdout, "handoffaudit: the document and the tree agree\n")
	return nil
}

// httpStatus asks a URL for its status code.
func httpStatus(url string) (int, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}
