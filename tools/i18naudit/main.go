// Command i18naudit scans a tree for documents that hardcode the interface
// language, its direction, or the words a person reads (P18-T10).
//
// It is the gate that `make audit-i18n` runs, and its exit status is the
// answer: 0 when the tree holds no violation, 1 when it holds at least one —
// printed as `path:line: rule: detail` so the fix is a matter of opening the
// file. It never rewrites anything: the change belongs to the author, in the
// commit that added the document.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "directory to scan")
	skip := flag.String("skip", "tools/i18naudit/fixtures", "comma-separated directory prefixes to skip, relative to the root (the fixtures of this tool are documents written to violate the rules)")
	flag.Parse()

	options := Options{}
	for _, prefix := range splitList(*skip) {
		options.Skip = append(options.Skip, prefix)
	}

	findings, err := Audit(*root, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "i18naudit: %v\n", err)
		os.Exit(1)
	}

	// Finding renders as `path:line: rule: detail`, which is the line a
	// reviewer opens and the property that broke.
	for _, finding := range findings {
		fmt.Fprintf(os.Stderr, "i18naudit: %s\n", finding)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "i18naudit: %d violation(s) — a document a person reads must come from the catalog, declare its locale and keep its direction logical\n", len(findings))
		os.Exit(1)
	}
	fmt.Printf("i18naudit: %s is clean (%d violations)\n", *root, 0)
}

// splitList parses a comma-separated flag value, ignoring empty entries so an
// empty flag means "skip nothing".
func splitList(value string) []string {
	var entries []string
	start := 0
	for index := 0; index <= len(value); index++ {
		last := index == len(value)
		if !last && value[index] != ',' {
			continue
		}
		if entry := value[start:index]; entry != "" {
			entries = append(entries, entry)
		}
		start = index + 1
	}
	return entries
}
