// Command caddyaudit answers the configuration questions of the Caddy origin
// (P19-T03). See audit.go for what it reads and why.
//
// It is the static half of the gate: it reads the committed file and names the
// rule it refuses. The behaviour half — the headers that arrive, the ones that
// leave, the request that is compressed, the response the process writes when
// the application never answered — needs a running Caddy and lives in
// verify.sh, because a rule about a value cannot see a value that a run has to
// produce.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	file := flag.String("file", "deploy/caddy/Caddyfile", "the Caddyfile as committed")
	siteAddressVariable := flag.String("site-address-variable", "COMPOSE_SITE_ADDRESS", "the environment placeholder the site address must come from")
	upstreamVariable := flag.String("upstream-variable", "COMPOSE_APP_UPSTREAM", "the environment placeholder the upstream must come from")
	flag.Parse()

	report, err := Audit(Options{
		File:                *file,
		SiteAddressVariable: *siteAddressVariable,
		UpstreamVariable:    *upstreamVariable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "caddyaudit: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("caddyaudit: site %s, upstream %s\n", report.SiteAddress, report.Upstream)
	fmt.Printf("caddyaudit: %d trusted prefix(es), client IP from %v\n", len(report.TrustedPrefixes), report.ClientIPHeaders)
	fmt.Printf("caddyaudit: TLS floor %s\n", report.TLSProtocols)
	fmt.Printf("caddyaudit: the edge's own error responses carry %d policy field(s), compared against internal/platform/securityheaders\n", len(report.ErrorPolicy))

	if len(report.Violations) > 0 {
		fmt.Fprintf(os.Stderr, "caddyaudit: FAILED — the origin breaks %d rule(s):\n", len(report.Violations))
		for _, violation := range report.Violations {
			fmt.Fprintf(os.Stderr, "caddyaudit:   %s\n", violation)
		}
		os.Exit(1)
	}

	fmt.Printf("caddyaudit: ok — the certificate is a mounted secret, only the CDN's claim about a visitor is believed, neither the proxied response nor the error response names a version, the upstream is told the address this process determined and nothing the client picked, and the policy the application delivers is the policy this process emits\n")
}
