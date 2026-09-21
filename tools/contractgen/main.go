// Command contractgen emits the TypeScript transport contracts consumed by
// the browser frontend (P18-T02) from the versioned OpenAPI document.
//
// It is wired to make generate / make generate-check:
//
//	contractgen -check    exit 0 when the generated file matches; 1 on drift
//	contractgen           (re)write the generated artifact only when it changed
//
// The output is deterministic, readonly by convention and never edited by
// hand: the source of truth is api/openapi.json.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const usage = `contractgen emits the TypeScript API contracts from api/openapi.json.

Usage:

  contractgen [-check] [-contract FILE] [-out FILE]

Flags:

  -check
	validate that the generated file matches the contract; exit 1 on drift
	instead of writing anything.
  -contract
	OpenAPI document to read (default api/openapi.json).
  -out
	TypeScript file to write (default web/src/contracts/generated.ts).
`

// Default paths fixed by convention, matching docs/FRONTEND.md section 6.
const (
	defaultContract = "api/openapi.json"
	defaultOut      = "web/src/contracts/generated.ts"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

// run is the testable entry point.
func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("contractgen", flag.ContinueOnError)
	check := flags.Bool("check", false, "verify the generated file instead of writing it")
	contractPath := flags.String("contract", defaultContract, "OpenAPI document to read")
	outPath := flags.String("out", defaultOut, "TypeScript file to write")
	// Flag errors are reported through the returned error, which carries
	// the usage block; the default stderr dump would duplicate it.
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("invalid flags\n\n%s", usage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n\n%s", flags.Arg(0), usage)
	}
	if *contractPath == "" || *outPath == "" {
		return fmt.Errorf("contract and out paths must not be empty\n\n%s", usage)
	}

	source, err := Render(*contractPath)
	if err != nil {
		return err
	}

	if *check {
		drifted, err := HasDrift(*outPath, source)
		if err != nil {
			return err
		}
		if drifted {
			return fmt.Errorf("%s is stale; run 'make generate'", *outPath)
		}
		fmt.Fprintf(stdout, "contractgen: %s is up to date\n", *outPath)
		return nil
	}

	changed, err := WriteIfChanged(*outPath, source)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(stdout, "contractgen: wrote %s\n", *outPath)
		return nil
	}
	fmt.Fprintf(stdout, "contractgen: %s already up to date\n", *outPath)
	return nil
}
