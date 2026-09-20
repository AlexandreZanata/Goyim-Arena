// Tests of the composition root (P18-T07A) that do not need PostgreSQL: the
// refusals are decided before anything is constructed, so they are decided
// before anything connects.
package bootstrap_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/Goyim-Arena/internal/bootstrap"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/assets"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/clockseed"
	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/config"
)

// manifestFixture is the asset build the composition needs. It is loaded from
// disk — not built in memory — because reading the manifest back is part of
// what the composition does at boot.
func manifestFixture(t *testing.T) assets.Manifest {
	t.Helper()
	manifest, err := assets.LoadFile("testdata/assets")
	if err != nil {
		t.Fatalf("assets.LoadFile(testdata/assets) error = %v", err)
	}
	return manifest
}

// lazyPool is a pool handle that never connects: pgxpool dials on demand, and
// every test in this file is refused before the first query. The address is
// deliberately unreachable so a test that started querying would fail loudly
// instead of silently passing against a development database.
func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://arena:arena-local-dev@127.0.0.1:1/arena?sslmode=disable")
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// completeOptions is a composition that lacks nothing but the field each case
// removes.
func completeOptions(t *testing.T) bootstrap.Options {
	t.Helper()
	return bootstrap.Options{
		Env:    config.EnvDevelopment,
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Pool:   lazyPool(t),
		Clock:  clockseed.NewClock(),
		Random: clockseed.NewRandom(),
		Assets: manifestFixture(t),
	}
}

func TestComposeAccountRefusesAnIncompleteComposition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		remove  func(options *bootstrap.Options)
		dropped string
	}{
		{
			name:    "without a logger",
			remove:  func(options *bootstrap.Options) { options.Logger = nil },
			dropped: "logger",
		},
		{
			name:    "without a clock",
			remove:  func(options *bootstrap.Options) { options.Clock = nil },
			dropped: "clock",
		},
		{
			name:    "without entropy",
			remove:  func(options *bootstrap.Options) { options.Random = nil },
			dropped: "entropy source",
		},
		{
			name:    "without a database pool",
			remove:  func(options *bootstrap.Options) { options.Pool = nil },
			dropped: "postgres pool",
		},
		{
			name:    "without the frontend build",
			remove:  func(options *bootstrap.Options) { options.Assets = assets.Manifest{} },
			dropped: "asset manifest",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := completeOptions(t)
			testCase.remove(&options)

			surface, err := bootstrap.ComposeAccount(options)
			if err == nil {
				t.Fatalf("ComposeAccount() built a surface from an incomplete composition (%d routes)", len(surface.Routes()))
			}
			if !errors.Is(err, bootstrap.ErrIncompleteComposition) {
				t.Errorf("ComposeAccount() error = %v, want it to wrap ErrIncompleteComposition", err)
			}
			if !strings.Contains(err.Error(), testCase.dropped) {
				t.Errorf("ComposeAccount() error = %q, want it to name the missing %q", err, testCase.dropped)
			}
		})
	}
}

// TestComposeAccountNamesEveryMissingDependencyAtOnce is the difference between
// one boot failure and four: an operator fixing the environment sees the whole
// list, not the first entry.
func TestComposeAccountNamesEveryMissingDependencyAtOnce(t *testing.T) {
	t.Parallel()

	options := completeOptions(t)
	options.Pool = nil
	options.Clock = nil
	options.Assets = assets.Manifest{}

	_, err := bootstrap.ComposeAccount(options)
	if err == nil {
		t.Fatal("ComposeAccount() built a surface without pool, clock and assets")
	}
	for _, missing := range []string{"clock", "postgres pool", "asset manifest"} {
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("ComposeAccount() error = %q, want it to name %q", err, missing)
		}
	}
	if strings.Contains(err.Error(), "logger") {
		t.Errorf("ComposeAccount() error = %q, but the logger was provided", err)
	}
}

func TestComposeAccountRefusesAnUnknownEnvironment(t *testing.T) {
	t.Parallel()

	options := completeOptions(t)
	options.Env = config.Env("staging")

	_, err := bootstrap.ComposeAccount(options)
	if err == nil {
		t.Fatal("ComposeAccount() accepted an environment outside development, test and production")
	}
	if !errors.Is(err, bootstrap.ErrIncompleteComposition) || !strings.Contains(err.Error(), "staging") {
		t.Errorf("ComposeAccount() error = %v, want a refusal naming the environment", err)
	}
}

// TestComposeAccountRefusesProductionWithoutAnEmailProvider is the fail-closed
// half of the local sink: a registration whose confirmation link has no
// delivery path is not served at all, instead of being served and silently
// dropping the message.
func TestComposeAccountRefusesProductionWithoutAnEmailProvider(t *testing.T) {
	t.Parallel()

	options := completeOptions(t)
	options.Env = config.EnvProduction

	surface, err := bootstrap.ComposeAccount(options)
	if err == nil {
		t.Fatalf("ComposeAccount() served production without an email provider (%d routes)", len(surface.Routes()))
	}
	if !errors.Is(err, bootstrap.ErrIncompleteComposition) {
		t.Errorf("ComposeAccount() error = %v, want it to wrap ErrIncompleteComposition", err)
	}
	if !strings.Contains(err.Error(), "email provider") {
		t.Errorf("ComposeAccount() error = %q, want it to name the missing provider", err)
	}
}
