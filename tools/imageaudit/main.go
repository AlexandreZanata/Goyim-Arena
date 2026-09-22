// Command imageaudit is the gate of the production image (P19-T01). It judges
// the recipe and the built artifact: the Dockerfile, the build context's
// .dockerignore, the image configuration and the exported filesystem.
//
// It is a Go tool, like the rest of the pipeline, so the audit needs neither a
// scanner nor a running daemon of its own — `tools/imageaudit/verify.sh` does
// the Docker work and hands the artifacts to it:
//
//	make image-verify        # builds the image, smokes it read-only, audits it
//	imageaudit -config config.json -rootfs rootfs.tar
//
// The report is the measurement, always; the exit status is the gate. It exits
// 1 when a rule is broken or when an artifact cannot be read at all, because a
// gate that could not look has not approved anything.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	dockerfile := flag.String("dockerfile", "Dockerfile", "Dockerfile of the image")
	dockerignore := flag.String("dockerignore", ".dockerignore", ".dockerignore of the build context")
	config := flag.String("config", "", "docker inspect document of the built image (required)")
	rootfs := flag.String("rootfs", "", "tar of the container filesystem, as `docker export` writes it (required)")
	flag.Parse()

	if strings.TrimSpace(*config) == "" || strings.TrimSpace(*rootfs) == "" {
		fmt.Fprintln(os.Stderr, "imageaudit: -config and -rootfs are required: the recipe alone does not say what was built")
		os.Exit(2)
	}

	report, err := Audit(Options{
		Dockerfile:   *dockerfile,
		Dockerignore: *dockerignore,
		Config:       *config,
		Rootfs:       *rootfs,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "imageaudit: the image could not be measured: %v\n", err)
		os.Exit(1)
	}

	report.write(os.Stdout)
	if len(report.Violations) > 0 {
		os.Exit(1)
	}
}

func (report Report) write(out io.Writer) {
	fmt.Fprintf(out, "imageaudit: stages (%d): %s\n", len(report.Stages), strings.Join(report.Stages, ", "))
	fmt.Fprintf(out, "imageaudit: user %s, entrypoint %s\n", orNone(report.User), orNone(report.Entrypoint))
	fmt.Fprintf(out, "imageaudit: exported filesystem holds %d paths, %d bytes to scan\n", report.RootfsFiles, report.RootfsBytes)

	if len(report.Violations) == 0 {
		fmt.Fprintln(out, "imageaudit: ok — pinned bases, no root user, no toolchain, no cache, no source, no credential")
		return
	}
	fmt.Fprintf(out, "imageaudit: FAILED — the image breaks %d rule(s):\n", len(report.Violations))
	for _, violation := range report.Violations {
		fmt.Fprintf(out, "imageaudit:   %s\n", violation)
	}
}

func orNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
