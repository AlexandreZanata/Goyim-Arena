package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func runForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	stdoutFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(stdoutFile.Name()) })

	err = run(args, stdoutFile)

	if _, seekErr := stdoutFile.Seek(0, 0); seekErr != nil {
		t.Fatalf("seek temp file: %v", seekErr)
	}
	if _, copyErr := stdout.ReadFrom(stdoutFile); copyErr != nil {
		t.Fatalf("read temp file: %v", copyErr)
	}
	if closeErr := stdoutFile.Close(); closeErr != nil {
		t.Fatalf("close temp file: %v", closeErr)
	}

	return stdout.String(), stderr.String(), err
}

func assertError(t *testing.T, err error, wantSubstring string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantSubstring)
	}
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("expected error containing %q, got %q", wantSubstring, err.Error())
	}
}

func assertStdout(t *testing.T, stdout, want string) {
	t.Helper()

	if stdout != want {
		t.Fatalf("stdout mismatch\n got: %q\nwant: %q", stdout, want)
	}
}

func TestRunVersionReportsDevelopmentVersion(t *testing.T) {
	stdout, _, err := runForTest(t, "version")
	if err != nil {
		t.Fatalf("run version: %v", err)
	}
	assertStdout(t, stdout, "arena version dev\n")
}

func TestRunVersionRejectsUnknownFlags(t *testing.T) {
	_, _, err := runForTest(t, "version", "extra")
	assertError(t, err, `unknown flag "extra"`)
	assertError(t, err, `Usage: arena version [--json]`)
}

func TestRunVersionJSONEmitsSingleValidObject(t *testing.T) {
	stdout, _, err := runForTest(t, "version", "--json")
	if err != nil {
		t.Fatalf("run version --json: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("version --json output is not valid JSON: %v\n%s", err, stdout)
	}
	for _, key := range []string{"version", "commit", "date"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("JSON missing key %q: %s", key, stdout)
		}
	}
	if payload["version"] != "dev" {
		t.Fatalf("version = %v, want dev", payload["version"])
	}
	if payload["commit"] != "unknown" {
		t.Fatalf("commit = %v, want unknown", payload["commit"])
	}
	if payload["date"] != "unknown" {
		t.Fatalf("date = %v, want unknown", payload["date"])
	}
}

func TestRunWithoutSubcommandPrintsHelp(t *testing.T) {
	stdout, _, err := runForTest(t)
	if err != nil {
		t.Fatalf("run without arguments: %v", err)
	}
	if !strings.Contains(stdout, "Usage:") || !strings.Contains(stdout, "version") {
		t.Fatalf("expected usage help, got %q", stdout)
	}
}

func TestRunHelpAndFlagsPrintHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "-help", "--help"} {
		stdout, _, err := runForTest(t, arg)
		if err != nil {
			t.Fatalf("run %q: %v", arg, err)
		}
		if !strings.Contains(stdout, "Usage:") {
			t.Fatalf("run %q: expected usage help, got %q", arg, stdout)
		}
	}
}

func TestRunHelpRejectsArguments(t *testing.T) {
	_, _, err := runForTest(t, "help", "extra")
	assertError(t, err, `help takes no arguments`)
}

func TestRunUnknownCommandFailsWithSuggestion(t *testing.T) {
	_, _, err := runForTest(t, "serve")
	assertError(t, err, `unknown command "serve"`)
	assertError(t, err, `Run "arena help" for usage.`)
}

func TestModulePathMatchesMasterPlan(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	const wantModule = "module github.com/AlexandreZanata/Goyim-Arena"
	if !strings.Contains(string(data), wantModule) {
		t.Fatalf("go.mod does not declare %q:\n%s", wantModule, data)
	}
}

func TestRunHelpListsServerCommand(t *testing.T) {
	stdout, _, err := runForTest(t)
	if err != nil {
		t.Fatalf("run without arguments: %v", err)
	}
	if !strings.Contains(stdout, "server") {
		t.Fatalf("expected help to list the server command, got %q", stdout)
	}
}

// TestRunServerRejectsArguments guards the guard: argument validation must
// run before configuration loading, so a typo never reaches the loader.
func TestRunServerRejectsArguments(t *testing.T) {
	_, _, err := runForTest(t, "server", "extra")
	assertError(t, err, "server takes no arguments")
	assertError(t, err, "Usage: arena server")
}

// TestServerBootsServesAndStopsOnSIGTERM is the subprocess validation
// required by P02-T05: the binary boots from a clean environment, answers
// /health/live and /health/ready with 200, logs the listening record as a
// single JSON object, and terminates within the deadline after SIGTERM,
// logging the graceful shutdown record.
func TestServerBootsServesAndStopsOnSIGTERM(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess lifecycle test skipped in -short mode")
	}

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

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close() // Reserve the port number only; the server binds it.

	logPath := filepath.Join(t.TempDir(), "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	defer logFile.Close()

	command := exec.Command(binary, "server")
	command.Env = []string{
		"ARENA_ADDR=" + address,
		"ARENA_ENV=development",
		"PATH=" + os.Getenv("PATH"), // go build may need the toolchain.
	}
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	baseURL := "http://" + address
	client := &http.Client{Timeout: 2 * time.Second}

	deadline := time.Now().Add(10 * time.Second)
	var liveResponse *http.Response
	for liveResponse == nil {
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			_ = command.Wait()
			log, _ := os.ReadFile(logPath)
			t.Fatalf("server did not answer /health/live in time; log:\n%s", log)
		}
		response, err := client.Get(baseURL + "/health/live")
		if err == nil {
			liveResponse = response
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	body, err := io.ReadAll(liveResponse.Body)
	_ = liveResponse.Body.Close()
	if err != nil {
		t.Fatalf("read /health/live body: %v", err)
	}
	if liveResponse.StatusCode != http.StatusOK {
		t.Fatalf("/health/live status = %d, want 200", liveResponse.StatusCode)
	}
	var livePayload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &livePayload); err != nil || livePayload.Status != "live" {
		t.Fatalf("/health/live body = %q (parse error: %v), want {\"status\":\"live\"}", body, err)
	}
	if got := liveResponse.Header.Get("X-Request-Id"); got == "" {
		t.Error("/health/live response is missing the X-Request-Id correlation header")
	}

	// P02-T08: the negotiated interface locale is echoed to clients. A
	// Brazilian Portuguese Accept-Language wins over the default; an
	// unmapped locale falls back to the configured default.
	if got := liveResponse.Header.Get("X-Interface-Locale"); got != "pt-BR" {
		t.Errorf("/health/live X-Interface-Locale default = %q, want pt-BR", got)
	}

	acceptRequest, err := http.NewRequest(http.MethodGet, baseURL+"/health/live", nil)
	if err != nil {
		t.Fatalf("build accept-language request: %v", err)
	}
	acceptRequest.Header.Set("Accept-Language", "en-US")
	acceptResponse, err := client.Do(acceptRequest)
	if err != nil {
		t.Fatalf("GET /health/live with Accept-Language: %v", err)
	}
	_, _ = io.Copy(io.Discard, acceptResponse.Body)
	_ = acceptResponse.Body.Close()
	if got := acceptResponse.Header.Get("X-Interface-Locale"); got != "en-US" {
		t.Errorf("X-Interface-Locale with Accept-Language en-US = %q, want en-US", got)
	}

	esRequest, err := http.NewRequest(http.MethodGet, baseURL+"/health/live", nil)
	if err != nil {
		t.Fatalf("build es-ES request: %v", err)
	}
	esRequest.Header.Set("Accept-Language", "es-ES")
	esResponse, err := client.Do(esRequest)
	if err != nil {
		t.Fatalf("GET /health/live with es-ES: %v", err)
	}
	_, _ = io.Copy(io.Discard, esResponse.Body)
	_ = esResponse.Body.Close()
	if got := esResponse.Header.Get("X-Interface-Locale"); got != "pt-BR" {
		t.Errorf("X-Interface-Locale with unsupported es-ES = %q, want pt-BR fallback", got)
	}

	readyResponse, err := client.Get(baseURL + "/health/ready")
	if err != nil {
		t.Fatalf("GET /health/ready: %v", err)
	}
	readyBody, err := io.ReadAll(readyResponse.Body)
	_ = readyResponse.Body.Close()
	if err != nil {
		t.Fatalf("read /health/ready body: %v", err)
	}
	if readyResponse.StatusCode != http.StatusOK {
		t.Fatalf("/health/ready status = %d, want 200", readyResponse.StatusCode)
	}
	var readyPayload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(readyBody, &readyPayload); err != nil || readyPayload.Status != "ready" {
		t.Fatalf("/health/ready body = %q (parse error: %v), want {\"status\":\"ready\"}", readyBody, err)
	}

	// SIGTERM must terminate the process within the deadline, gracefully.
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server exited with error after SIGTERM: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-done
		t.Fatal("server did not terminate within 5s of SIGTERM")
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read server log: %v", err)
	}
	records := parseJSONLogRecords(t, string(log))
	if !containsRecord(records, "msg", "http server: listening") {
		t.Errorf("log is missing the listening record:\n%s", log)
	}
	if !containsRecord(records, "msg", "http server: graceful shutdown complete") {
		t.Errorf("log is missing the graceful shutdown record:\n%s", log)
	}
}

// parseJSONLogRecords decodes every line as a JSON object, failing on any
// non-JSON line: the whole server log must stay machine-readable.
func parseJSONLogRecords(t *testing.T, log string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(log), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not a JSON object: %q (%v)\nfull log:\n%s", line, err, log)
		}
		records = append(records, record)
	}
	return records
}

func containsRecord(records []map[string]any, key, want string) bool {
	for _, record := range records {
		if value, ok := record[key].(string); ok && value == want {
			return true
		}
	}
	return false
}
