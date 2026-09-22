// Command stub is the upstream the Caddy gate speaks to (P19-T03).
//
// The ingress is a component whose whole job is to change what passes through
// it, so it cannot be measured against the application's real responses: an
// assertion about pass-through needs an upstream that answers with values the
// gate chose. This program is that upstream, and it is deliberately tiny — it
// has no dependency, no configuration and no state, because a fixture that can
// be configured is a fixture that can be configured wrong.
//
// What it is not: a test double of the arena binary. It implements the two
// endpoints the proxy health-checks (`/health/live`, `/health/ready`) and four
// canned responses the gate asserts on — a document that must not be cached, a
// hashed asset that must stay immutable, an echo of the forwarding headers, and
// a 404 for everything else, which is how "the admin API is not reachable
// through the site" is observed. Every response names it, because a server that
// names itself is the ordinary case and the edge's job is not to pass that on.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// The headers the gate reads back, in the order it prints them: the ones the
// ingress is supposed to determine itself, then the ones it is supposed to
// remove so that the application never sees a client's claim.
var echoedHeaders = []string{
	"X-Forwarded-For",
	"X-Forwarded-Proto",
	"X-Forwarded-Host",
	"CF-Connecting-IP",
	"X-Real-IP",
	"Server",
}

// compressibleBody is larger than the encoder's minimum length, which is why
// it exists: a few bytes make a compression assertion pass by never
// compressing anything.
var compressibleBody = func() string {
	var body strings.Builder
	body.WriteString("<!doctype html><title>stub compressible document</title>\n")
	for body.Len() < 4096 {
		body.WriteString("<p>the ingress compresses what is worth compressing, and this paragraph exists to be worth it.</p>\n")
	}
	return body.String()
}()

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "address to listen on")
	flag.Parse()

	mux := http.NewServeMux()

	// The two probes the reverse proxy asks about. They answer 200 with a body
	// that says nothing, because what a probe returns is not read by anyone
	// here — its status code is the whole answer.
	for _, path := range []string{"/health/live", "/health/ready"} {
		path := path
		mux.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writeStatus(writer, http.StatusOK, "ok\n")
		})
	}

	// A document: private, must not be stored by anything between the visitor
	// and the origin, and carrying an ETag the ingress must not invent.
	mux.HandleFunc("/document", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("ETag", `"document-1"`)
		writeStatus(writer, http.StatusOK, "<!doctype html><title>stub document</title>\n")
	})

	// A hashed asset: the address carries the content digest, so the response
	// is immutable for a year and its ETag is part of the contract the asset
	// server promises (internal/platform/assets).
	mux.HandleFunc("/assets/hashed-1b2f5c0361dd.js", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		writer.Header().Set("ETag", `"asset-1"`)
		writeStatus(writer, http.StatusOK, "console.log(\"stub asset\");\n")
	})

	// A document large enough for the encoder to consider, carrying the same
	// private cache directive and an ETag: the two halves of the compression
	// question — is it compressed, and what happens to the validator of the
	// identity response when it is.
	mux.HandleFunc("/compressible", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("ETag", `"compressible-1"`)
		writeStatus(writer, http.StatusOK, compressibleBody)
	})

	// The echo: every header the ingress is responsible for, named exactly, so
	// a missing one and an empty one are different observations. `Host` is read
	// from the request rather than from the header map, because Go moved it out
	// of the map and a probe that reads the map would report it missing no
	// matter what the ingress did.
	mux.HandleFunc("/echo", func(writer http.ResponseWriter, request *http.Request) {
		var body strings.Builder
		fmt.Fprintf(&body, "Host=%s\n", request.Host)
		for _, name := range echoedHeaders {
			fmt.Fprintf(&body, "%s=%s\n", name, strings.Join(request.Header.Values(name), ","))
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writeStatus(writer, http.StatusOK, body.String())
	})

	// Everything else, including the paths that would reach a control plane:
	// the gate asserts that a request for the Caddy admin API arrives here and
	// is answered by this fake's 404 rather than by Caddy's own endpoint.
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeStatus(writer, http.StatusNotFound, "stub: not found\n")
	})

	server := &http.Server{Addr: *addr, Handler: mux}
	fmt.Fprintf(os.Stderr, "stub: listening on %s\n", *addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("stub: %v", err)
	}
}

// writeStatus writes the status before the body, and names this upstream on the
// way out. It exists so that every handler in this file writes its code in
// exactly one place: a handler that forgot it would answer 200 for a 404, and
// the gate's admin-route assertion would then pass for the wrong reason.
//
// The `Server` header is here for the same reason: the assertion that the edge
// discloses no version is only worth something against an upstream that states
// one. Caddy forwards an upstream's own value unchanged, and Go's server emits
// none of its own, so a fixture that sent none would let a missing strip pass
// while measuring nothing.
func writeStatus(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("Server", "stub/1.0")
	writer.WriteHeader(status)
	if _, err := writer.Write([]byte(body)); err != nil {
		log.Printf("stub: write: %v", err)
	}
}
