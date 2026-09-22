// The measured facts of the reproducible release verification (P20-T07).
//
// The phase asks for a clean checkout of the current commit, dependencies
// installed from the lockfiles only, the dependencies brought up, `make verify`,
// the image build and a smoke — each command passing **twice** — and for the
// results and the real limitations to be recorded in
// docs/RELEASE_CHECKLIST.md.
//
// A checklist a person types is a claim. So the checksum is a separate step
// from the writing: tools/releaseverify/verify.sh runs the exercise and writes
// these facts as JSON, and this tool renders the document from them. The numbers
// in the document are then the numbers the run measured, and the only thing a
// human hand puts into the file is prose.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// factsVersion is the schema version of the facts file and of the document's
// machine-readable block.
const factsVersion = 1

// Facts is everything the run measured.
type Facts struct {
	// Version is the schema version.
	Version int `json:"version"`
	// VerifiedOn is the date of the run, YYYY-MM-DD.
	VerifiedOn string `json:"verified_on"`
	// Commit is the commit the clean checkout was made at, full SHA.
	Commit string `json:"commit"`
	// Branch is the branch the run was started from.
	Branch string `json:"branch"`
	// Checkout is how the clean tree was obtained and what it looked like.
	Checkout Checkout `json:"checkout"`
	// Host is the toolchain the run used.
	Host Host `json:"host"`
	// Lockfiles are the lockfiles the dependencies were installed from.
	Lockfiles []Lockfile `json:"lockfiles"`
	// Commands are the commands the phase names, each with every run.
	Commands []Command `json:"commands"`
	// Image is the built image and the smoke it passed.
	Image Image `json:"image"`
	// Repository is the state of the repository after the run.
	Repository Repository `json:"repository"`
	// Governance is the release gate's verdict, which is about decisions and
	// not about code.
	Governance Governance `json:"governance"`
	// Limits are the things this verification does not establish.
	Limits []string `json:"limits"`
	// Next are the steps that follow this verification.
	Next []string `json:"next"`
}

// Checkout is the clean tree the verification ran in.
type Checkout struct {
	// Kind is how the tree was obtained.
	Kind string `json:"kind"`
	// Clean reports whether the tree was clean when the run started.
	Clean bool `json:"clean"`
	// LocalTrackedFiles is how many files of the ignored local directory the
	// commit tracks. It has to be zero: the local plan never ships.
	LocalTrackedFiles int `json:"local_tracked_files"`
	// Generated is the path of the document this run produced, relative to
	// the repository root.
	Generated string `json:"generated"`
	// Dirty is what the repository holds after the document was written: the
	// generated document and nothing else.
	Dirty []string `json:"dirty"`
	// Overlay is what the run copied on top of the commit: the files this
	// task itself adds, so that the tree verified is the tree that gets
	// committed and not the commit before it. Empty when the commit is the
	// whole tree, which is the usual case for a later run.
	Overlay []OverlayEntry `json:"overlay"`
}

// OverlayEntry is one file copied over the clean checkout, with the digest that
// was copied. Declaring it is what keeps the overlay from being a place where a
// green run hides a change nobody reviewed — and the digest is what lets a
// reader tell whether the file the document describes is still the file in the
// tree.
type OverlayEntry struct {
	// Path is the file, relative to the repository root.
	Path string `json:"path"`
	// SHA256 is the digest of the copy that was verified.
	SHA256 string `json:"sha256"`
}

// Host is the toolchain the run used. Pinning it in the document is what lets a
// later reader tell a changed result from a changed machine.
type Host struct {
	// OS is the operating system, with its kernel release.
	OS string `json:"os"`
	// Arch is the machine architecture.
	Arch string `json:"arch"`
	// CPUs is the number of processors.
	CPUs int `json:"cpus"`
	// Go is the Go toolchain version.
	Go string `json:"go"`
	// Node is the Node version.
	Node string `json:"node"`
	// NPM is the npm version.
	NPM string `json:"npm"`
	// Docker is the Docker client and server version.
	Docker string `json:"docker"`
	// Sqlc is the sqlc version the generated code is produced with.
	Sqlc string `json:"sqlc"`
	// Postgres is the PostgreSQL server version the gates ran against.
	Postgres string `json:"postgres"`
}

// Lockfile is one lockfile and what it locks.
type Lockfile struct {
	// Path is the lockfile, relative to the repository root.
	Path string `json:"path"`
	// SHA256 is the file's digest, so a reader can tell whether the run
	// installed the dependencies the commit pins.
	SHA256 string `json:"sha256"`
	// Installs is the command that installs from it.
	Installs string `json:"installs"`
	// Note states what it locks.
	Note string `json:"note"`
}

// Run is one execution of a command.
type Run struct {
	// Seconds is how long it took. It is a pointer because zero is a legal
	// duration — a cached command answers in under a millisecond — while an
	// absent one is how a run somebody wrote instead of made looks, and the
	// two have to be told apart.
	Seconds *float64 `json:"seconds"`
	// ExitCode is the status the command answered with.
	ExitCode int `json:"exit_code"`
}

// Duration is the run's measurement, or zero when none was recorded.
func (r Run) Duration() float64 {
	if r.Seconds == nil {
		return 0
	}
	return *r.Seconds
}

// Command is one of the commands the phase names, with every run of it.
type Command struct {
	// Key is the stable identifier the rules are keyed on.
	Key string `json:"key"`
	// Command is the command as it is typed.
	Command string `json:"command"`
	// Runs is one entry per execution. The phase asks for two.
	Runs []Run `json:"runs"`
	// Note states what the command establishes.
	Note string `json:"note"`
}

// OK reports whether every run of the command succeeded.
func (c Command) OK() bool {
	for _, run := range c.Runs {
		if run.ExitCode != 0 {
			return false
		}
	}
	return len(c.Runs) > 0
}

// TotalSeconds is the sum of the command's runs.
func (c Command) TotalSeconds() float64 {
	total := 0.0
	for _, run := range c.Runs {
		total += run.Duration()
	}
	return total
}

// Image is the production image the run built and smoked.
type Image struct {
	// Reference is the tag the build used.
	Reference string `json:"reference"`
	// ID is the image identifier, sha256:….
	ID string `json:"id"`
	// Digest is the manifest digest.
	Digest string `json:"digest"`
	// SizeBytes is the size of the image.
	SizeBytes int64 `json:"size_bytes"`
	// Smoke is the name of the smoke the image passed.
	Smoke string `json:"smoke"`
	// SmokeDetail is what the smoke established.
	SmokeDetail string `json:"smoke_detail"`
	// SmokeOK is the smoke's own verdict.
	SmokeOK bool `json:"smoke_ok"`
}

// Repository is the state of the repository after the run.
type Repository struct {
	// TrackedFiles is how many files the commit tracks.
	TrackedFiles int `json:"tracked_files"`
	// Fsck is the result of `git fsck`.
	Fsck string `json:"fsck"`
	// FsckErrors is how many errors `git fsck` reported.
	FsckErrors int `json:"fsck_errors"`
	// MainBehindOrigin records whether the working branch is behind its remote
	// when the run finished, which is a fact about publishing, not about code.
	MainBehindOrigin bool `json:"main_behind_origin"`
}

// Governance is the release gate: the decisions the owner makes, which no
// amount of verification can substitute.
type Governance struct {
	// Command is the gate that judged the decisions.
	Command string `json:"command"`
	// Blocked reports whether the gate refused.
	Blocked bool `json:"blocked"`
	// Open are the decisions still open, named the way the gate names them.
	Open []string `json:"open"`
	// Pending are the steps the gate lists for the decisions already taken.
	Pending []string `json:"pending"`
	// Note states what the gate is about.
	Note string `json:"note"`
}

// ReadFacts reads the facts a run wrote.
func ReadFacts(path string) (Facts, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Facts{}, err
	}
	var facts Facts
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&facts); err != nil {
		return Facts{}, fmt.Errorf("%s: the facts are not readable: %w", path, err)
	}
	if facts.Version != factsVersion {
		return Facts{}, fmt.Errorf("%s: the facts declare version %d and this tool writes version %d",
			path, facts.Version, factsVersion)
	}
	return facts, nil
}

// WriteFacts writes the facts, indented, for the renderer to read.
func WriteFacts(path string, facts Facts) error {
	raw, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
