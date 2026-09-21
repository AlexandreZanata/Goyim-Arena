// Command webaudit measures the delivered frontend build against the budgets
// and the dependency rules the frontend declares (P18-T08).
//
// It is a Go tool, like the rest of the pipeline, so the measurement needs
// neither Node nor a browser and the runtime image stays as it is:
//
//	make audit-web          # builds the frontend and measures it
//	webaudit -build web/dist
//
// The report is the measurement, always; the exit status is the gate. It writes
// to stdout so a CI log keeps the numbers next to the verdict, and exits 1 when
// the build breaks a rule or cannot be measured at all.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	build := flag.String("build", "web/dist", "build directory holding manifest.json and the assets it declares")
	flag.Parse()

	report, err := Audit(*build)
	if err != nil {
		fmt.Fprintf(os.Stderr, "webaudit: the build could not be measured: %v\n", err)
		os.Exit(1)
	}

	report.write(os.Stdout)
	if len(report.Violations) > 0 {
		os.Exit(1)
	}
}

// plural picks the word a count deserves, so a report reads "1 module" and
// "11 modules" instead of inviting a reader to doubt the measurement.
func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

// write prints the measurement and the verdict. A failure lists every violation
// instead of the first one: a build that breaks three rules should not need
// three runs to discover them.
func (report Report) write(out io.Writer) {
	fmt.Fprintf(out, "webaudit: build %s (%d assets, %d modules, %d stylesheets)\n",
		report.Build, report.Assets, report.Modules, report.Styles)
	fmt.Fprintf(out, "webaudit: initial JavaScript of a public page (gzip, one stream per module, budget %d B):\n",
		pageJavaScriptBudget)
	for _, page := range report.Pages {
		fmt.Fprintf(out, "webaudit:   %-28s %7d B  %2d %s\n", page.Name, page.Bytes, page.Modules, plural(page.Modules, "module", "modules"))
	}
	fmt.Fprintf(out, "webaudit: initial CSS (gzip, one stream per stylesheet, budget %d B): %d B\n",
		initialCSSBudget, report.CSSBytes)

	if len(report.Violations) == 0 {
		fmt.Fprintln(out, "webaudit: ok — the delivered build respects the budgets and the dependency rules")
		return
	}
	fmt.Fprintf(out, "webaudit: FAILED — the delivered build breaks %d rule(s):\n", len(report.Violations))
	for _, violation := range report.Violations {
		fmt.Fprintf(out, "webaudit:   %s\n", violation)
	}
}
