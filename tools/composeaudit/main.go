// Command composeaudit is the gate of the production Compose topology
// (P19-T02). It judges the configuration Docker Compose *creates*, not the file
// as written, and it judges the file as committed for the one question the
// rendered document cannot answer.
//
// It is a Go tool, like the rest of the pipeline, so the audit needs no service
// of its own — `tools/composeaudit/verify.sh` does the Docker work and hands
// the artifacts to it:
//
//	make compose-verify      # builds, renders, audits, brings the stack up
//	composeaudit -file compose.production.yaml -document rendered.json -application-image repo@sha256:...
//
// The report is the measurement, always; the exit status is the gate. It exits
// 1 when a rule is broken or when an artifact cannot be read at all, because a
// gate that could not look has not approved anything. It never prints an
// environment value: the rendered document has inlined the operator's
// environment files, and a gate that echoes them is a leak with a green tick.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	file := flag.String("file", "compose.production.yaml", "Compose file as it is committed")
	document := flag.String("document", "", "rendered document written by `docker compose config --format json` (required)")
	applicationImage := flag.String("application-image", "", "reference of the application image, pinned by digest (required)")
	flag.Parse()

	if strings.TrimSpace(*document) == "" || strings.TrimSpace(*applicationImage) == "" {
		fmt.Fprintln(os.Stderr, "composeaudit: -document and -application-image are required: the file alone does not say what Compose will create")
		os.Exit(2)
	}

	report, err := Audit(Options{
		File:             *file,
		Document:         *document,
		ApplicationImage: *applicationImage,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "composeaudit: the topology could not be measured: %v\n", err)
		os.Exit(1)
	}

	report.write(os.Stdout)
	if len(report.Violations) > 0 {
		os.Exit(1)
	}
}

func (report Report) write(out io.Writer) {
	fmt.Fprintf(out, "composeaudit: project %s, services (%d): %s\n", report.Project, len(report.Services), strings.Join(report.Services, ", "))
	fmt.Fprintf(out, "composeaudit: ingress %s, database %s, application %s\n", orNone(report.Ingress), orNone(report.Database), report.ApplicationImage)
	fmt.Fprintf(out, "composeaudit: networks (%d): %s\n", len(report.Networks), strings.Join(report.Networks, ", "))
	if len(report.PublishedPorts) == 0 {
		fmt.Fprintln(out, "composeaudit: no port is published")
	} else {
		fmt.Fprintf(out, "composeaudit: published ports (%d): %s\n", len(report.PublishedPorts), strings.Join(report.PublishedPorts, ", "))
	}
	fmt.Fprintf(out, "composeaudit: the database port %d is published nowhere\n", databasePort)

	if len(report.Violations) == 0 {
		fmt.Fprintln(out, "composeaudit: ok — digest-pinned images, one public surface, no database port, isolated networks, probes, limits and restart declared")
		return
	}
	fmt.Fprintf(out, "composeaudit: FAILED — the topology breaks %d rule(s):\n", len(report.Violations))
	for _, violation := range report.Violations {
		fmt.Fprintf(out, "composeaudit:   %s\n", violation)
	}
}
