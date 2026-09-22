// Tests of the workflow reader (P19-T08).
//
// The rules are written in paths, so the path a line lands on is the contract
// between the reader and the audit: a rule that looked at the wrong path would
// pass over a file it never read. These cases pin the paths down on the shapes
// a delivered workflow actually uses — a job, a step, a script block, a service
// and a flow list.
package main

import (
	"strings"
	"testing"
)

// readerFixture is every shape the reader has to understand, in one document.
const readerFixture = `name: verify
on:
  push:
    branches: [main]
  pull_request:
    types: [opened, ready_for_review]
permissions:
  contents: read
jobs:
  foundation:
    name: Foundation verification
    if: github.event.pull_request.draft == false
    timeout-minutes: 30
    services:
      postgres:
        image: postgres:18.4
        ports:
          - 54329:5432
    env:
      ARENA_DATABASE_URL: postgres://arena@127.0.0.1:54329/arena?sslmode=disable
    steps:
      - name: Checkout repository
        uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
      # A whole step written as a script, followed by its own keys.
      - run: |
          make verify
          make test-unit
        env:
          CI: "true"
`

func parseFixture(t *testing.T) workflow {
	t.Helper()
	wf, err := parseWorkflow("fixture.yml", []byte(readerFixture))
	if err != nil {
		t.Fatalf("parseWorkflow: %v", err)
	}
	return wf
}

// TestReaderAddressesEveryShape requires the paths the rules depend on.
func TestReaderAddressesEveryShape(t *testing.T) {
	wf := parseFixture(t)

	cases := []struct {
		path  string
		key   string
		value string
	}{
		{"jobs.foundation.timeout-minutes", "timeout-minutes", "30"},
		{"jobs.foundation.if", "if", "github.event.pull_request.draft == false"},
		{"jobs.foundation.env.ARENA_DATABASE_URL", "ARENA_DATABASE_URL", "postgres://arena@127.0.0.1:54329/arena?sslmode=disable"},
		{"jobs.foundation.services.postgres.image", "image", "postgres:18.4"},
		{"jobs.foundation.services.postgres.ports[0]", "", "54329:5432"},
		{"jobs.foundation.steps[0].name", "name", "Checkout repository"},
		{"jobs.foundation.steps[0].with.fetch-depth", "fetch-depth", "0"},
		{"jobs.foundation.steps[1].run", "run", "make verify\nmake test-unit"},
		{"jobs.foundation.steps[1].env.CI", "CI", "true"},
		{"permissions.contents", "contents", "read"},
		{"on.push.branches", "branches", "[main]"},
	}
	for _, testCase := range cases {
		got, ok := wf.at(testCase.path)
		if !ok {
			t.Errorf("%s is missing; the rules address it", testCase.path)
			continue
		}
		if got.Key != testCase.key || got.Value != testCase.value {
			t.Errorf("%s reads %q = %q; expected %q = %q", testCase.path, got.Key, got.Value, testCase.key, testCase.value)
		}
	}

	// The version comment beside a pinned action is what a reviewer checks.
	uses, ok := wf.at("jobs.foundation.steps[0].uses")
	if !ok {
		t.Fatal("jobs.foundation.steps[0].uses is missing")
	}
	if uses.Comment != "v7.0.1" {
		t.Errorf("the comment beside the pinned action reads %q; expected v7.0.1", uses.Comment)
	}

	// A flow list is read the same whether it is inline or a block.
	if got := wf.flowList("on.pull_request.types"); strings.Join(got, ",") != "opened,ready_for_review" {
		t.Errorf("on.pull_request.types reads %v; expected opened and ready_for_review", got)
	}

	if got := wf.jobIDs(); strings.Join(got, ",") != "foundation" {
		t.Errorf("jobIDs reads %v; expected foundation", got)
	}
	if got := wf.stepPaths("foundation"); strings.Join(got, ",") != "jobs.foundation.steps[0],jobs.foundation.steps[1]" {
		t.Errorf("stepPaths reads %v; expected the two steps in order", got)
	}

	// A step's whole body is one run block, and the deployment target it
	// invokes is what the wiring rules read.
	run, ok := wf.at("jobs.foundation.steps[1].run")
	if !ok {
		t.Fatal("the script step has no run block")
	}
	if got := strings.Join(invokedTargets(run.Value), ","); got != "verify,test-unit" {
		t.Errorf("invokedTargets reads %q; expected verify and test-unit", got)
	}
}
