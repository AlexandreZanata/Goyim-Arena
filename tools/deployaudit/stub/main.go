// Command stub is the release stand-in of the deployment fire drill (P19-T07).
//
// It exists for one reason. The claim "a release that does not become healthy is
// not promoted" can only be measured by promoting a release that does not become
// healthy, and the honest way to produce one is not to build a broken copy of
// the application: that artifact would be reviewed by nobody and would say
// nothing about the pipeline. So this is built instead, and it is deliberately
// small enough to read in a minute.
//
// What it is: a real artifact, built and pushed by digest through the same
// registry path a release takes, run by the real production compose document
// under the same read-only, capability-dropped, non-root container, and judged
// by the real readiness and smoke probes of `deploy/deploy.sh`. What it is not:
// the application. It carries no migrations, serves no product page, and the
// version it reports says so out loud.
//
// The mode is baked into each image (STAND_IN_MODE) rather than passed at run
// time, because the mode is a property of the artifact: a release that cannot
// become ready is one whose own configuration says so, and the pipeline's
// rollback has to restore a *healthy* previous release while that configuration
// is still in the operator's file. A mode that lived in the environment file
// would ride along into the rollback and make the incident path look like a
// refusal.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	// modeVar is the variable the mode is baked into by the drill's Dockerfile.
	modeVar = "STAND_IN_MODE"

	// modeNotReady answers 503 to everything, the readiness probe included: a
	// release that boots and never becomes healthy.
	modeNotReady = "not-ready"

	// modeReady answers both health probes and serves its own page everywhere
	// else: a release that is promoted successfully and then rolled back.
	modeReady = "ready"

	// modePageMissing is ready and has lost a page. The readiness probe passes
	// and the smoke probe does not, which is the case the smoke step exists
	// for.
	modePageMissing = "page-missing"

	// version is what `arena version` prints. The stand-in names itself, so
	// neither the pipeline's log nor `docker logs` can be read as if the
	// application had been promoted.
	version = "0.0.0-stand-in"

	// listenAddr is where the topology expects the serving process to listen;
	// the application's image declares ARENA_ADDR=0.0.0.0:8080 the same way.
	listenAddr = "0.0.0.0:8080"
)

// standInPage is what the serving mode returns for any page. It is a marker:
// the drill asserts that the application's own page came back after a rollback,
// and this is what the stand-in's page must not be mistaken for.
const standInPage = "<!doctype html>\n" +
	"<title>stand-in release</title>\n" +
	"<p>this page belongs to the deployment fire drill, not to the arena\n"

func main() {
	mode := normalizeMode(os.Getenv(modeVar))
	args := os.Args[1:]
	if len(args) == 0 {
		usage("a command is required")
	}
	switch args[0] {
	case "version":
		fmt.Printf("arena version %s\n", version)
	case "migrate":
		if len(args) != 2 || args[1] != "up" {
			usage("the stand-in knows only 'migrate up'")
		}
		// Exit 0 and apply nothing. The stand-in carries no migrations of its
		// own, and applying the application's would make the drill's claim that
		// a rollback never moves the schema a lie.
		fmt.Fprintln(os.Stderr, "stand-in: migrate up applies nothing; this release carries no migrations")
	case "server":
		serve(mode)
	case "worker":
		// A worker idles. One that exited would be restarted in a loop and
		// would say nothing about the release that serves.
		for {
			time.Sleep(time.Hour)
		}
	default:
		usage("unknown command " + args[0])
	}
}

// normalizeMode is the safe default and the only place a mode becomes usable:
// only the three modes the drill bakes are modes, and anything else — an empty
// value, a misspelling, a stray space, a different case — is "not ready".
// Forgiving a near miss would let an image built with a typo claim a readiness
// it was never told to report, which is the one outcome this file must not
// produce.
func normalizeMode(mode string) string {
	switch mode {
	case modeReady:
		return modeReady
	case modePageMissing:
		return modePageMissing
	default:
		return modeNotReady
	}
}

// newHandler maps a mode and a path to a response. It is the whole behaviour of
// the stand-in, and it is a function so that the mapping is a unit test rather
// than something the drill is the first to notice.
func newHandler(mode string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if mode == modeNotReady {
			http.Error(w, "stand-in release: not ready\n", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/health/live", "/health/ready":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "ok")
		default:
			if mode == modePageMissing {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, standInPage)
		}
	})
	return mux
}

func serve(mode string) {
	fmt.Fprintf(os.Stderr, "stand-in: serving %s in mode %s\n", listenAddr, mode)
	if err := http.ListenAndServe(listenAddr, newHandler(mode)); err != nil {
		fmt.Fprintf(os.Stderr, "stand-in: %v\n", err)
		os.Exit(1)
	}
}

func usage(problem string) {
	fmt.Fprintf(os.Stderr, "stand-in: %s\nusage: arena <version|migrate up|server|worker>\n", problem)
	os.Exit(2)
}
