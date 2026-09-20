// Command arena is the single binary of Goyim Arena. Per the master plan,
// subcommands include server, worker, migrate and explicitly approved
// operations; server, worker, migrate, version and help exist.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	// The embedded IANA timezone database keeps the optional profile
	// timezone validation (P05-T06) working in minimal containers that
	// ship no system tzdata.
	_ "time/tzdata"

	"github.com/AlexandreZanata/Goyim-Arena/internal/bootstrap"
	"github.com/AlexandreZanata/Goyim-Arena/internal/buildinfo"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/assets"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/clockseed"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/config"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbpool"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/locale"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/logging"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/profiling"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/securityheaders"
)

const usage = `arena is the command-line entrypoint of Goyim Arena.

Usage:

  arena <command> [arguments]

The commands are:

  server     run the HTTP server (ARENA_* configuration from the environment)
  worker     consume durable jobs until stopped (SIGTERM or SIGINT)
  migrate    apply or inspect database migrations (status, up)
  projections rebuild derived public statistics projections
  version    show the arena version; use --json for machine-readable output
  help       show this help

Run "arena <command> -h" for details about a command.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "arena:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout *os.File) error {
	if len(args) == 0 {
		fmt.Fprintln(stdout, usage)
		return nil
	}

	switch args[0] {
	case "server":
		return runServer(args[1:], stdout)
	case "worker":
		return runWorker(args[1:], stdout)
	case "migrate":
		return runMigrate(args[1:], stdout)
	case "projections":
		return runProjections(args[1:], stdout)
	case "version":
		return runVersion(args[1:], stdout)
	case "help", "-h", "-help", "--help":
		if len(args) > 1 {
			return fmt.Errorf("help takes no arguments (got %q)", args[1])
		}
		fmt.Fprintln(stdout, usage)
	default:
		return fmt.Errorf("unknown command %q\n\n%s\n\nRun \"arena help\" for usage.", args[0], usage)
	}
	return nil
}

// runServer boots the hardened HTTP server (P02-T05) and composes the module
// surfaces it serves (P18-T07A): typed configuration from the environment, the
// structured JSON logger, request ID correlation, the account browser journey
// mounted on the platform mux, and a graceful shutdown on SIGTERM/SIGINT. It
// is the process edge — the only place allowed to own signals and the real
// clock/randomness sources.
//
// With ARENA_DATABASE_URL set, the account journey is composed and served; the
// frontend build named by ARENA_ASSETS_DIR is required for it, because a page
// mounted without its manifest would render links to files that do not exist.
// Without the DSN the process serves the health routes only and says so in the
// log: an application that answers 404 on every page while reporting itself
// ready is worse than a probe that declares what it is.
func runServer(args []string, stdout *os.File) error {
	if len(args) > 0 {
		return fmt.Errorf("server takes no arguments\n\nUsage: arena server")
	}

	cfg := config.MustLoad()
	logger := logging.New(stdout, cfg.LogLevel())

	ids := clockseed.NewIDGenerator("req", clockseed.NewRandom(), clockseed.NewClock())
	locResolver := locale.NewResolver()

	// One clock and one entropy source for the whole process: a request
	// observed by two layers must carry the same instant, and a token minted by
	// a use case must come from the same source the composition was validated
	// against.
	clock := clockseed.NewClock()
	random := clockseed.NewRandom()

	var readyCheckers []httpserver.ReadyChecker
	var surfaces []httpserver.Surface
	if cfg.DatabaseURL().IsSet() {
		dsn := string(cfg.DatabaseURL().Unredacted())
		poolCfg := dbpool.FromConfig(cfg)
		pool, err := dbpool.New(context.Background(), dsn, poolCfg, logger, clock)
		if err != nil {
			return fmt.Errorf("initialize database pool: %w", err)
		}
		defer pool.Close()
		readyCheckers = append(readyCheckers, pool)

		// The account journey is composed whole or not at all: the database
		// and the frontend build are both required, and the boot is the only
		// place where noticing a missing one is cheap.
		manifest, err := assets.LoadFile(cfg.AssetsDir())
		if err != nil {
			return fmt.Errorf("compose the account journey: %w (run 'make build-web' or point ARENA_ASSETS_DIR at an existing build)", err)
		}
		account, err := bootstrap.ComposeAccount(bootstrap.Options{
			Env:    cfg.Env(),
			Logger: logger,
			Pool:   pool.Pool(),
			Clock:  clock,
			Random: random,
			Assets: manifest,
		})
		if err != nil {
			return err
		}
		surfaces = append(surfaces, account.Surface())
		logger.Info("http server: account journey mounted", slog.Int("routes", len(account.Routes())))
	} else {
		logger.Warn("http server: account journey not mounted (ARENA_DATABASE_URL is not set); only the health routes are served")
	}

	handler, err := httpserver.NewMuxWith(ids, locResolver, securityheaders.Config{Production: cfg.IsProduction()}, surfaces, readyCheckers...)
	if err != nil {
		return err
	}

	server, err := httpserver.New(httpserver.Options{
		Addr:    cfg.Addr(),
		Handler: handler,
		Logger:  logger,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var adminServer *httpserver.Server
	if adminAddr := cfg.AdminAddr(); adminAddr != "" {
		adminServer, err = httpserver.New(httpserver.Options{
			Addr:    adminAddr,
			Handler: profiling.Handler(),
			Logger:  logger,
		})
		if err != nil {
			return fmt.Errorf("initialize profiling server: %w", err)
		}
		if err := adminServer.Listen(); err != nil {
			return fmt.Errorf("listen profiling server: %w", err)
		}
		go func() {
			if err := adminServer.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("profiling server stopped", slog.String("error", err.Error()))
			}
		}()
		logger.Info("profiling server: listening", slog.String("addr", adminServer.Addr()))
	}

	if err := server.Listen(); err != nil {
		return err
	}
	logger.Info("http server: listening",
		slog.String("addr", server.Addr()),
		slog.String("env", string(cfg.Env())),
	)

	return server.Run(ctx)
}

// runVersion prints the reproducible build metadata (P01-T05). Without
// flags it renders one human-readable line; with --json it renders a single
// RFC 8259 object, so scripts can parse the output safely.
func runVersion(args []string, stdout *os.File) error {
	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			return fmt.Errorf("unknown flag %q\n\nUsage: arena version [--json]", arg)
		}
	}

	info := buildinfo.Current(os.Environ())
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(info)
	}

	fmt.Fprintf(stdout, "arena version %s\n", info.Version)
	return nil
}
