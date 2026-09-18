package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/AlexandreZanata/Goyim-Arena/internal/platform/dbtest"
)

// TestWorkerUsageAndArgumentValidation covers the argument surface of the
// worker subcommand without touching a database.
func TestWorkerUsageAndArgumentValidation(t *testing.T) {
	t.Parallel()

	stdout, _, err := runForTest(t, "worker", "-h")
	if err != nil {
		t.Fatalf("worker -h error = %v", err)
	}
	for _, want := range []string{"arena worker", "ARENA_DATABASE_URL", "SIGTERM"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("worker -h output missing %q:\n%s", want, stdout)
		}
	}

	if _, _, err := runForTest(t, "worker", "extra"); err == nil {
		t.Error("worker with arguments must fail")
	}

	help, _, err := runForTest(t, "help")
	if err != nil {
		t.Fatalf("help error = %v", err)
	}
	if !strings.Contains(help, "worker") {
		t.Errorf("usage does not list the worker command:\n%s", help)
	}
}

// TestWorkerFailsFastWithoutDatabaseURL pins the fail-fast behavior: without
// ARENA_DATABASE_URL the worker stops before opening anything.
func TestWorkerFailsFastWithoutDatabaseURL(t *testing.T) {
	t.Parallel()

	_, _, err := runForTest(t, "worker")
	if err == nil {
		t.Skip("ARENA_* environment is configured; skipping the no-config assertion")
	}
	if !strings.Contains(err.Error(), "ARENA_") {
		t.Errorf("error should name the missing ARENA_* configuration, got: %v", err)
	}
}

// TestWorkerBootsAndStopsOnSIGTERM is the process-level lifecycle validation
// of P15-T02: the binary boots against a real database, logs the started
// record, and terminates cleanly (exit 0) well within the deadline after
// SIGTERM, logging the stopped record.
//
// The complementary property — a job already leased is finished and recorded
// instead of being abandoned — is proven at the runtime level in
// internal/jobs/application (TestWorkerGracefulShutdownCompletesInFlightJob),
// because no workload handler is registered yet: they arrive with P15-T03/T04.
func TestWorkerBootsAndStopsOnSIGTERM(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess lifecycle test skipped in -short mode")
	}

	db := dbtest.New(t)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	binary := filepath.Join(t.TempDir(), "arena")
	build := exec.Command("go", "build", "-o", binary, "./cmd/arena")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}

	logPath := filepath.Join(t.TempDir(), "worker.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	defer logFile.Close()

	command := exec.Command(binary, "worker")
	command.Env = []string{
		"ARENA_ENV=development",
		"ARENA_DATABASE_URL=" + db.DSN,
		"ARENA_DB_MAX_CONNS=4",
		"ARENA_DB_MIN_CONNS=1",
		"PATH=" + os.Getenv("PATH"),
	}
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	// Keep the log file readable while the process writes to it.
	waitForLog := func(fragment string, deadline time.Duration) string {
		limit := time.Now().Add(deadline)
		for time.Now().Before(limit) {
			content, _ := os.ReadFile(logPath)
			if strings.Contains(string(content), fragment) {
				return string(content)
			}
			time.Sleep(50 * time.Millisecond)
		}
		content, _ := os.ReadFile(logPath)
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("worker log never contained %q; log:\n%s", fragment, content)
		return ""
	}

	startedLog := waitForLog("job worker: started", 20*time.Second)
	if !strings.Contains(startedLog, `"concurrency"`) {
		t.Errorf("started record should carry the configured concurrency:\n%s", startedLog)
	}

	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal worker: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	select {
	case waitErr := <-done:
		if waitErr != nil {
			content, _ := os.ReadFile(logPath)
			t.Fatalf("worker exited with %v after SIGTERM; log:\n%s", waitErr, content)
		}
	case <-time.After(15 * time.Second):
		_ = command.Process.Kill()
		content, _ := os.ReadFile(logPath)
		t.Fatalf("worker did not stop within the deadline after SIGTERM; log:\n%s", content)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read worker log: %v", err)
	}
	if !strings.Contains(string(content), "job worker: stopped") {
		t.Errorf("worker did not log the stopped record:\n%s", content)
	}
}
