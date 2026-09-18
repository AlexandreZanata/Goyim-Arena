package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	jobsrepo "github.com/AlexandreZanata/Goyim-Arena/internal/jobs/adapters/postgres"
	jobsapp "github.com/AlexandreZanata/Goyim-Arena/internal/jobs/application"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/clockseed"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/config"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbpool"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/logging"
)

const workerUsage = `run the durable job worker of Goyim Arena.

Usage:

  arena worker    consume queued jobs until stopped (SIGTERM or SIGINT)

The worker reads ARENA_* configuration from the environment
(ARENA_DATABASE_URL is required) and consumes app.jobs with a bounded pool:
each worker leases one job at a time, runs its versioned handler under a
deadline shorter than the lease, records success or a redacted failure, and
retries with exponential backoff until the attempt budget is spent.

A shutdown signal stops new claims and lets in-flight jobs finish and be
recorded, so a stop never abandons a lease.`

// runWorker implements `arena worker` (P15-T02). It is a process edge: it owns
// the signal handling, the real clock and the real entropy source, and it
// composes the queue adapters with the runtime.
func runWorker(args []string, stdout *os.File) error {
	if len(args) > 0 {
		if args[0] == "-h" || args[0] == "-help" || args[0] == "--help" {
			fmt.Fprintln(stdout, workerUsage)
			return nil
		}
		return fmt.Errorf("worker takes no arguments (got %q)\n\n%s", args[0], workerUsage)
	}

	cfg := config.MustLoad()
	dsn := string(cfg.DatabaseURL().Unredacted())
	if dsn == "" {
		return fmt.Errorf("ARENA_DATABASE_URL is required for arena worker (set it to the PostgreSQL DSN)")
	}

	logger := logging.New(stdout, cfg.LogLevel())

	clock := clockseed.NewClock()
	pool, err := dbpool.New(context.Background(), dsn, dbpool.FromConfig(cfg), logger, clock)
	if err != nil {
		return fmt.Errorf("initialize database pool: %w", err)
	}
	defer pool.Close()

	repo := jobsrepo.NewRepository(pool.Pool())
	workerCfg := jobsapp.DefaultWorkerConfig()

	enqueue, err := jobsapp.NewEnqueueUseCase(repo, clock)
	if err != nil {
		return err
	}
	lease, err := jobsapp.NewLeaseUseCase(repo, clock)
	if err != nil {
		return err
	}
	complete, err := jobsapp.NewCompleteUseCase(repo, clock)
	if err != nil {
		return err
	}
	fail, err := jobsapp.NewFailUseCase(repo, clock)
	if err != nil {
		return err
	}
	recoverLeases, err := jobsapp.NewRecoverExpiredLeasesUseCase(repo, clock)
	if err != nil {
		return err
	}

	// Handlers are registered by the phases that own each workload (email
	// delivery arrives with P15-T03/T04, scheduled maintenance with P15-T05).
	// Until then the registry is empty and a job of an unregistered type is
	// recorded as JOB_UNKNOWN_TYPE rather than being guessed at.
	registry := jobsapp.NewHandlerMap()
	_ = enqueue // Enqueue is composed for producers wired in later phases.

	worker, err := jobsapp.NewWorker(jobsapp.WorkerDeps{
		Lease:    lease,
		Complete: complete,
		Fail:     fail,
		Recover:  recoverLeases,
		Registry: registry,
		Clock:    clock,
		Random:   clockseed.NewRandom(),
		Logger:   logger,
		Config:   workerCfg,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("job worker: started",
		slog.Int("concurrency", workerCfg.Concurrency),
		slog.Duration("lease", workerCfg.LeaseDuration),
		slog.Duration("handler_timeout", workerCfg.HandlerTimeout),
		slog.Int("handlers", registry.Len()),
	)
	if registry.Len() == 0 {
		logger.Warn("job worker: no handlers registered yet; unregistered workloads fail as JOB_UNKNOWN_TYPE")
	}

	runErr := worker.Run(ctx)

	stats := worker.Stats()
	logger.Info("job worker: stopped",
		slog.Int64("leased", stats.Leased),
		slog.Int64("succeeded", stats.Succeeded),
		slog.Int64("failed", stats.Failed),
		slog.Int64("dead", stats.Dead),
		slog.Int64("reclaimed", stats.Reclaimed),
	)
	return runErr
}
