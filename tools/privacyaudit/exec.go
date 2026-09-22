// Running the audit (P20-T06).
//
// The areas of the phase are declared as commands in the register, written the
// way a person would type them, and this file is what runs them. A run is not a
// proof of correctness — it is the difference between a control somebody claims
// and a control somebody ran.
package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// noTestsMarker is what the Go test runner prints when a -run filter matched
// nothing. An area whose command matched no test would otherwise exit zero and
// report a control nobody exercised, which is the failure this gate exists to
// refuse.
const noTestsMarker = "no tests to run"

// Execution is one area's command and what it answered.
type Execution struct {
	// Area is the area the command belongs to.
	Area string
	// Command is the command as the register declares it.
	Command string
	// Output is what the command printed, kept short: the gate is a verdict,
	// and a failing run is read in full only when it fails.
	Output string
}

// runAreas runs each area's command from the repository root and returns what
// each one said. An area that fails becomes a violation named after the area,
// with the command's own output as the detail — the gate must never say
// "failed" without saying what failed.
//
// The commands go through a shell because they are written the way a person
// types them (`go test ...`); they come from a versioned, reviewed document,
// and they are reviewed as code.
func runAreas(root string, areas []Area, timeout time.Duration) ([]Execution, []Violation) {
	executions := make([]Execution, 0, len(areas))
	var violations []Violation
	for _, area := range areas {
		if strings.TrimSpace(area.Execution) == "" {
			continue
		}
		output, err := runCommand(root, area.Execution, timeout)
		executions = append(executions, Execution{
			Area: area.Key, Command: area.Execution, Output: tail(output),
		})
		if strings.Contains(output, noTestsMarker) {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-empty",
				Detail: fmt.Sprintf("%s matched no test: %s", area.Execution, tail(output)),
			})
			continue
		}
		if err != nil {
			violations = append(violations, Violation{
				Subject: area.Key, Rule: "area-run",
				Detail: fmt.Sprintf("%s failed: %v\n%s", area.Execution, err, tail(output)),
			})
		}
	}
	return executions, violations
}

// runCommand runs one command from root, bounded by timeout.
func runCommand(root, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("timed out after %s", timeout)
	}
	return string(output), err
}

// tail keeps the end of a command's output: a failing suite says why at the
// end, and a gate that prints a megabyte of log is a gate nobody reads.
func tail(output string) string {
	const limit = 4000
	trimmed := strings.TrimRight(output, "\n")
	if len(trimmed) <= limit {
		return trimmed
	}
	return "…\n" + trimmed[len(trimmed)-limit:]
}
