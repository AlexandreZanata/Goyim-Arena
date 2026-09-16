package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestRunVersionRejectsArguments(t *testing.T) {
	_, _, err := runForTest(t, "version", "extra")
	assertError(t, err, `version takes no arguments`)
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
