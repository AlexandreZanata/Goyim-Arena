package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// committedFile is the artifact this audit exists for. The tests mutate the
// real file rather than a copy of it: a rule that only holds for a fixture
// holds for nothing.
const committedFile = "../../deploy/caddy/Caddyfile"

// siteAddressVariable and upstreamVariable are the placeholders the audit
// expects, named once so that a renamed variable is a visible edit here.
const (
	siteAddressVariable = "COMPOSE_SITE_ADDRESS"
	upstreamVariable    = "COMPOSE_APP_UPSTREAM"
)

// source reads the committed file.
func source(t *testing.T) string {
	t.Helper()

	raw, err := os.ReadFile(committedFile)
	if err != nil {
		t.Fatalf("read the committed Caddyfile: %v", err)
	}
	return string(raw)
}

// written writes a mutation where the audit can read it.
func written(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "Caddyfile")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write the mutated Caddyfile: %v", err)
	}
	return path
}

// auditFile runs the audit over one file.
func auditFile(t *testing.T, path string) Report {
	t.Helper()

	report, err := Audit(Options{
		File:                path,
		SiteAddressVariable: siteAddressVariable,
		UpstreamVariable:    upstreamVariable,
	})
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	return report
}

// replaceOnce substitutes one exact occurrence, and fails when the old text is
// not there or is there more than once. That failure mode is the point: an
// earlier gate in this repository reported three mutations as "the rule did not
// fire" when in truth the mutation had never applied, and a test that cannot
// tell those two apart reports the wrong thing about the code.
func replaceOnce(t *testing.T, content, old, new string) string {
	t.Helper()

	switch count := strings.Count(content, old); {
	case count == 0:
		t.Fatalf("the mutation does not apply: %q is not in the file", old)
	case count > 1:
		t.Fatalf("the mutation is ambiguous: %q appears %d times", old, count)
	}
	return strings.Replace(content, old, new, 1)
}

// replaceLine rewrites the one line whose trimmed content starts with prefix.
// It exists for the lines too long to quote exactly — the trusted proxy list is
// one line of twenty-two prefixes — where quoting the text would make the test
// a copy of the file instead of a change to it.
func replaceLine(t *testing.T, content, prefix, replacement string) string {
	t.Helper()

	lines := strings.Split(content, "\n")
	matches := 0
	for index, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), prefix) {
			continue
		}
		matches++
		lines[index] = replacement
	}
	if matches != 1 {
		t.Fatalf("the mutation is ambiguous: %d line(s) start with %q", matches, prefix)
	}
	return strings.Join(lines, "\n")
}

// removeBlock deletes one block, from the line that opens it through the line
// whose brace closes it. Brace counting is done on trimmed lines, which is what
// makes it usable for a block whose body contains braces of its own.
func removeBlock(t *testing.T, content, opener string) string {
	t.Helper()

	lines := strings.Split(content, "\n")
	start, depth := -1, 0
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 {
			if trimmed != opener {
				continue
			}
			start = index
		}
		depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		if start >= 0 && depth == 0 {
			return strings.Join(append(append([]string{}, lines[:start]...), lines[index+1:]...), "\n")
		}
	}
	t.Fatalf("the mutation does not apply: no block opens with %q", opener)
	return content
}

// TestTheCommittedOriginPasses is the baseline: the file this repository
// commits breaks no rule. Every probe below is the difference between this run
// and one broken rule.
func TestTheCommittedOriginPasses(t *testing.T) {
	t.Parallel()

	report := auditFile(t, committedFile)
	if len(report.Violations) > 0 {
		t.Fatalf("the committed origin breaks %d rule(s): %v", len(report.Violations), report.Violations)
	}
	if report.SiteAddress != "{$"+siteAddressVariable+":arena.invalid}" {
		t.Errorf("site address = %q, want the environment placeholder", report.SiteAddress)
	}
	if report.Upstream != "{$"+upstreamVariable+":app:8080}" {
		t.Errorf("upstream = %q, want the environment placeholder", report.Upstream)
	}
	// The list is Cloudflare's published set: fifteen IPv4 and seven IPv6
	// ranges. The count is asserted rather than the values, because the values
	// are Cloudflare's to change and the audit's job is to read the list, not
	// to be its source.
	if len(report.TrustedPrefixes) != 22 {
		t.Errorf("trusted prefixes = %d, want the 22 ranges Cloudflare publishes (15 IPv4 and 7 IPv6)", len(report.TrustedPrefixes))
	}
	if len(report.ClientIPHeaders) != 1 || report.ClientIPHeaders[0] != "CF-Connecting-IP" {
		t.Errorf("client IP headers = %v, want exactly the one header the CDN writes", report.ClientIPHeaders)
	}
	if report.TLSProtocols != "tls1.2 tls1.3" {
		t.Errorf("TLS protocols = %q, want the floor Cloudflare's Full (strict) mode requires", report.TLSProtocols)
	}
	// The policy the application delivers is five fields plus the cache
	// directive the error responses add: the headers P16-T01 carries on every
	// response, and the no-store that keeps an outage from being cached.
	if len(report.ErrorPolicy) != 6 {
		t.Errorf("the error policy carries %v, want the application's fields plus Cache-Control", report.ErrorPolicy)
	}
}

// TestEachRuleRefusesItsOwnMutation runs one mutation per rule. `only` marks
// the probes whose attribution is exact — the mutation breaks that rule and no
// other — so that a rule which fired for the wrong reason is caught.
func TestEachRuleRefusesItsOwnMutation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		rule   string
		only   bool
		mutate func(t *testing.T, content string) string
	}{
		{
			name: "an admin endpoint on a wildcard address",
			rule: "admin_loopback",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "admin localhost:2019", "admin :2019")
			},
		},
		{
			name: "no admin endpoint declared at all",
			rule: "admin_declared",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\tadmin localhost:2019\n", "")
			},
		},
		{
			name: "the admin endpoint switched off",
			rule: "admin_loopback",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "admin localhost:2019", "admin off")
			},
		},
		{
			name: "a trusted prefix that trusts everybody",
			rule: "trusted_proxies_declared",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceLine(t, content, "trusted_proxies static", "\t\ttrusted_proxies static 0.0.0.0/0")
			},
		},
		{
			name: "no trusted prefix at all",
			rule: "trusted_proxies_declared",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceLine(t, content, "trusted_proxies static", "\t\ttrusted_proxies static")
			},
		},
		{
			name: "a trusted list this audit cannot read",
			rule: "trusted_proxies_declared",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceLine(t, content, "trusted_proxies static", "\t\ttrusted_proxies file /etc/caddy/trusted-proxies")
			},
		},
		{
			name: "a prefix that is not a prefix",
			rule: "trusted_proxies_declared",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceLine(t, content, "trusted_proxies static", "\t\ttrusted_proxies static 203.0.113.0/24 not-a-prefix")
			},
		},
		{
			name: "the client IP taken from a header a client may pad",
			rule: "client_ip_narrowed",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "client_ip_headers CF-Connecting-IP", "client_ip_headers X-Forwarded-For")
			},
		},
		{
			name: "no client IP header declared",
			rule: "client_ip_narrowed",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\tclient_ip_headers CF-Connecting-IP\n", "")
			},
		},
		{
			name: "a handshake that is served whatever certificate comes first",
			rule: "sni_strict",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "strict_sni_host on", "strict_sni_host insecure_off")
			},
		},
		{
			name: "a listener that also accepts QUIC",
			rule: "protocols_bounded",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "protocols h1 h2\n", "protocols h1 h2 h3\n")
			},
		},
		{
			name: "a listener with no protocols declared",
			rule: "protocols_bounded",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\tprotocols h1 h2\n", "")
			},
		},
		{
			name: "a listener with no header budget",
			rule: "edge_timeouts",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\tread_header 10s\n", "")
			},
		},
		{
			name: "a listener that holds an idle connection forever",
			rule: "edge_timeouts",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\tidle 2m\n", "")
			},
		},
		{
			name: "a certificate that is not a mounted secret",
			rule: "tls_from_files",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "tls /run/secrets/origin_certificate /run/secrets/origin_key {", "tls /etc/caddy/origin_certificate /run/secrets/origin_key {")
			},
		},
		{
			name: "a tls directive with one file",
			rule: "tls_from_files",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "tls /run/secrets/origin_certificate /run/secrets/origin_key {", "tls /run/secrets/origin_certificate {")
			},
		},
		{
			name: "a TLS floor below what the CDN requires",
			rule: "tls_protocols",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "protocols tls1.2 tls1.3", "protocols tls1.1 tls1.2")
			},
		},
		{
			name: "a site address written into the file",
			rule: "site_address_from_environment",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "{$COMPOSE_SITE_ADDRESS:arena.invalid} {", "arena.invalid {")
			},
		},
		{
			name: "a response that names the software",
			rule: "server_header_removed",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\theader -Server\n", "")
			},
		},
		{
			name: "an encoding nobody reviewed",
			rule: "compression_bounded",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "encode zstd gzip", "encode zstd gzip deflate")
			},
		},
		{
			name: "no compression at all",
			rule: "compression_bounded",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\tencode zstd gzip\n", "")
			},
		},
		{
			name: "an upstream written into the file",
			rule: "upstream_from_environment",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "reverse_proxy {$COMPOSE_APP_UPSTREAM:app:8080} {", "reverse_proxy app:8080 {")
			},
		},
		{
			name: "the client's own chain forwarded to the application",
			rule: "forwarded_truthful",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "header_up X-Forwarded-For {client_ip}", "header_up +X-Forwarded-For")
			},
		},
		{
			name: "the client's CF-Connecting-IP left in place",
			rule: "forwarded_truthful",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\theader_up -CF-Connecting-IP\n", "")
			},
		},
		{
			name: "the client's X-Real-IP left in place",
			rule: "forwarded_truthful",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\theader_up -X-Real-IP\n", "")
			},
		},
		{
			name: "no upstream health check",
			rule: "upstream_health",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\thealth_uri /health/ready\n", "")
			},
		},
		{
			name: "no upstream transport budget",
			rule: "upstream_timeouts",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\ttransport http {\n\t\t\t\tdial_timeout 5s\n\t\t\t\tresponse_header_timeout 15s\n\t\t\t\tkeepalive 30s\n\t\t\t\tkeepalive_idle_conns 4\n\t\t\t}\n", "")
			},
		},
		{
			name: "an upstream that can be dialed forever",
			rule: "upstream_timeouts",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\t\tdial_timeout 5s\n", "")
			},
		},
		{
			name: "a policy value that drifted from the application's",
			rule: "error_policy",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, `X-Content-Type-Options "nosniff"`, `X-Content-Type-Options "sniff"`)
			},
		},
		{
			name: "a policy field the application delivers and the edge omits",
			rule: "error_policy",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\tX-Content-Type-Options \"nosniff\"\n", "")
			},
		},
		{
			name: "a header only the edge sets",
			rule: "error_policy",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\tCache-Control \"no-store\"\n", "\t\t\tCache-Control \"no-store\"\n\t\t\tCross-Origin-Opener-Policy \"same-origin\"\n")
			},
		},
		{
			name: "an error response an intermediary may cache",
			rule: "error_policy",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, `Cache-Control "no-store"`, `Cache-Control "public, max-age=60"`)
			},
		},
		{
			name: "no error handler at all",
			rule: "error_policy",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return removeBlock(t, content, "handle_errors {")
			},
		},
		{
			name: "a cache directive on the proxied path",
			rule: "no_cache_override",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\theader -Server\n", "\t\theader -Server\n\t\theader Cache-Control \"public, max-age=60\"\n")
			},
		},
		{
			name: "a cache inside a route",
			rule: "no_cache_override",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\tencode zstd gzip\n", "\t\tencode zstd gzip\n\t\tcache {\n\t\t\tmatch_path /assets/*\n\t\t}\n")
			},
		},
		{
			name: "a route that reaches the admin endpoint",
			rule: "no_admin_route",
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\theader -Server\n",
					"\t\theader -Server\n\t\t@config path /config/*\n\t\treverse_proxy @config localhost:2019\n")
			},
		},
		{
			name: "an outage that names the server",
			rule: "error_server_hidden",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return replaceOnce(t, content, "\t\t\t-Server\n", "")
			},
		},
		{
			name: "a second site nobody reviewed",
			rule: "site_declared",
			only: true,
			mutate: func(t *testing.T, content string) string {
				return content + "\nother.invalid {\n\trespond \"somewhere else\"\n}\n"
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := written(t, testCase.mutate(t, source(t)))
			report := auditFile(t, path)
			if !hasRule(report, testCase.rule) {
				t.Fatalf("the mutation broke no %q rule: %v", testCase.rule, report.Violations)
			}
			if testCase.only && len(report.Violations) != 1 {
				t.Fatalf("the mutation breaks %d rule(s), and the probe attributes one: %v", len(report.Violations), report.Violations)
			}
		})
	}
}

func hasRule(report Report, rule string) bool {
	for _, violation := range report.Violations {
		if violation.Rule == rule {
			return true
		}
	}
	return false
}

// TestThePlaceholderIsNotABrace covers the one parsing rule the whole audit
// rests on. `{$COMPOSE_SITE_ADDRESS}` and `{client_ip}` begin with a brace and
// are values; `{` on its own opens a block. A parser that confused them would
// read this file as one block per placeholder and every rule below would be
// answering questions about a tree that does not exist.
func TestThePlaceholderIsNotABrace(t *testing.T) {
	t.Parallel()

	content := source(t)
	statements, err := parse([]byte(content))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}

	sites := siteBlocks(statements)
	if len(sites) != 1 {
		t.Fatalf("the committed file parses as %d site(s), want one: the placeholders were read as blocks", len(sites))
	}
	if len(sites[0].children) == 0 {
		t.Fatal("the site parsed as empty: its placeholders consumed the block")
	}
	if !strings.Contains(strings.Join(sites[0].args, " "), "{$COMPOSE_SITE_ADDRESS") {
		t.Fatalf("the site address did not survive parsing: %v", sites[0].args)
	}

	// The other direction: a real block delimiter is still one.
	withBlock := "arena.invalid {\n\trespond \"x\"\n}\n"
	parsed, err := parse([]byte(withBlock))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if len(parsed) != 1 || len(parsed[0].children) != 1 {
		t.Fatalf("a block was not read as a block: %+v", parsed)
	}
}

// TestTheAuditRefusesWhatItCannotRead is the fail-closed half: an input the
// audit cannot read is an error rather than an empty report, because a gate
// that returns "no violations" for a file it never understood is a gate that
// passes when the file is missing.
func TestTheAuditRefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{name: "an empty file", content: ""},
		{name: "a file that declares nothing but a comment", content: "# nothing here\n"},
		{name: "a quoted value that is never closed", content: "arena.invalid {\n\trespond \"unterminated\n}\n"},
		{name: "a brace that closes nothing", content: "}\n"},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Audit(Options{
				File:                written(t, testCase.content),
				SiteAddressVariable: siteAddressVariable,
				UpstreamVariable:    upstreamVariable,
			}); err == nil {
				t.Fatal("Audit() accepted a file it cannot read; a gate that cannot read its input must say so")
			}
		})
	}

	if _, err := Audit(Options{File: filepath.Join(t.TempDir(), "absent"), SiteAddressVariable: siteAddressVariable, UpstreamVariable: upstreamVariable}); err == nil {
		t.Fatal("Audit() accepted a file that does not exist")
	}
}
