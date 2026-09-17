// Command arena is the single binary of Goyim Arena. Per the master plan,
// subcommands include server, worker, migrate and explicitly approved
// operations; for now server, migrate, version and help exist.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/AlexandreZanata/Goyim-Arena/internal/buildinfo"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/clockseed"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/config"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbpool"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/httpserver"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/locale"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/logging"
)

const usage = `arena is the command-line entrypoint of Goyim Arena.

Usage:

  arena <command> [arguments]

The commands are:

  server     run the HTTP server (ARENA_* configuration from the environment)
  migrate    apply or inspect database migrations (status, up)
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
	case "migrate":
		return runMigrate(args[1:], stdout)
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

// runServer boots the hardened HTTP server (P02-T05): typed configuration
// from the environment, the structured JSON logger, request ID correlation
// with the health routes, and a graceful shutdown on SIGTERM/SIGINT. It is
// the process edge — the only place allowed to own signals and the real
// clock/randomness sources.
func runServer(args []string, stdout *os.File) error {
	if len(args) > 0 {
		return fmt.Errorf("server takes no arguments\n\nUsage: arena server")
	}

	cfg := config.MustLoad()
	logger := logging.New(stdout, cfg.LogLevel())

	ids := clockseed.NewIDGenerator("req", clockseed.NewRandom(), clockseed.NewClock())
	locResolver := locale.NewResolver()

	var readyCheckers []httpserver.ReadyChecker
	if cfg.DatabaseURL().IsSet() {
		dsn := string(cfg.DatabaseURL().Unredacted())
		poolCfg := dbpool.FromConfig(cfg)
		dbClock := clockseed.NewClock()
		pool, err := dbpool.New(context.Background(), dsn, poolCfg, logger, dbClock)
		if err != nil {
			return fmt.Errorf("initialize database pool: %w", err)
		}
		defer pool.Close()
		readyCheckers = append(readyCheckers, pool)
	}

	handler, err := httpserver.NewMux(ids, locResolver, readyCheckers...)
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
