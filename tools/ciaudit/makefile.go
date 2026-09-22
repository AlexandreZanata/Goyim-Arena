package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Reading of the Makefile the CI invokes (P19-T08).
//
// Makefile is the part of the Makefile the CI depends on: its targets, their
// prerequisites and their recipes. Reading it is what lets the auditor refuse a
// workflow that invokes a target nobody wrote, and a gate that `make verify`
// lists but no job reaches.
type Makefile struct {
	Path    string
	targets map[string]makeTarget
}

type makeTarget struct {
	Prerequisites []string
	Recipe        string
}

// targetNamePattern is the shape of a target that can also be named on a
// command line: `fmt-check`, `image-verify`, `test-e2e`.
var targetNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// readMakefile reads the targets of a Makefile. A file that cannot be read is
// an error, never an empty Makefile: a target set nobody could read would make
// every "this target does not exist" finding a guess.
func readMakefile(path string) (Makefile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Makefile{}, err
	}
	mk := Makefile{Path: path, targets: map[string]makeTarget{}}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	current := ""
	for i, line := range lines {
		if strings.HasPrefix(line, "\t") {
			if current == "" {
				return Makefile{}, fmt.Errorf("%s:%d: a recipe line belongs to no target", path, i+1)
			}
			if recipe := strings.TrimSpace(line); recipe != "" {
				existing := mk.targets[current]
				if existing.Recipe != "" {
					existing.Recipe += "\n"
				}
				existing.Recipe += recipe
				mk.targets[current] = existing
			}
			continue
		}
		if strings.HasPrefix(line, " ") {
			// A continuation of the previous recipe or a conditional body;
			// neither declares a target.
			continue
		}
		name, prerequisites, ok := parseTargetLine(line)
		if !ok {
			current = ""
			continue
		}
		current = name
		mk.targets[name] = makeTarget{Prerequisites: prerequisites, Recipe: mk.targets[name].Recipe}
	}
	return mk, nil
}

// parseTargetLine reads `target: prerequisite …` and refuses everything that is
// an assignment (`GO ?= go`, `IMAGE ?= …`) or an entry whose name could not be
// named on a command line (`.PHONY:`, `a b: c`).
func parseTargetLine(line string) (string, []string, bool) {
	line = strings.TrimRight(line, " \t")
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, false
	}
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '=', '?', '!', '+':
			// An assignment, or a conditional modifier; `:` after these is
			// part of a value, never the end of a target name.
			return "", nil, false
		case ':':
			if i+1 < len(line) && line[i+1] == '=' {
				return "", nil, false
			}
			name := strings.TrimSpace(line[:i])
			if !targetNamePattern.MatchString(name) {
				return "", nil, false
			}
			rest := line[i+1:]
			if idx := strings.Index(rest, "#"); idx >= 0 {
				rest = rest[:idx]
			}
			var prerequisites []string
			for _, field := range strings.Fields(rest) {
				if strings.ContainsAny(field, "=:$()|") {
					continue
				}
				prerequisites = append(prerequisites, field)
			}
			return name, prerequisites, true
		}
	}
	return "", nil, false
}

// exists reports whether the Makefile declares a target.
func (mk Makefile) exists(name string) bool {
	_, ok := mk.targets[name]
	return ok
}

// prerequisites lists the targets a target depends on.
func (mk Makefile) prerequisites(name string) []string {
	return mk.targets[name].Prerequisites
}

// recipe is the joined body of a target.
func (mk Makefile) recipe(name string) string {
	return mk.targets[name].Recipe
}
