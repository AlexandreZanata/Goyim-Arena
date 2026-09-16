// Package architecture_test enforces the backend dependency direction.
//
// Domain and application layers must depend only on the Go standard library
// (minus transport and persistence details) and on their own module packages.
// Adapters, platform, bootstrap, cross-module internals and unapproved
// external dependencies are forbidden in these layers, per
// docs/ARCHITECTURE.md (Ports and Adapters) and AGENTS.md. Adding an approved
// dependency to a layer requires an ADR and a matching change here.
//
// Since P02-T01, direct process-environment reads are additionally gated to
// the typed configuration package (and cmd/bootstrap when it appears):
// no other internal package may call os.Getenv, os.LookupEnv or os.Setenv.
package architecture_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/AlexandreZanata/Goyim-Arena"

const modulePrefix = modulePath + "/"

// forbiddenStdlib lists standard library packages that are transport,
// persistence or serialization details. Domain and application layers must
// not import them.
var forbiddenStdlib = map[string]string{
	"database/sql":      "persistence detail",
	"encoding/json":     "serialization detail",
	"html/template":     "transport detail",
	"net/http":          "transport detail",
	"net/http/httputil": "transport detail",
	"net/url":           "transport detail",
}

// forbiddenExternal prefixes are dependencies not approved for domain and
// application layers. Approved adapters (pgx, sqlc, goose, x/crypto, Stripe,
// Resend, Sentry, PostHog) live exclusively in adapter packages.
var forbiddenExternal = []string{
	"github.com/golang/",
	"github.com/getsentry/",
	"github.com/jackc/",
	"github.com/posthog/",
	"github.com/pressly/",
	"github.com/resend/",
	"github.com/sqlc-dev/",
	"github.com/stripe/",
	"golang.org/x/",
}

// businessModules are the module directories required by the plan and
// docs/ARCHITECTURE.md §4.
var businessModules = []string{
	"arenas", "arguments", "audit", "billing", "identity", "jobs",
	"moderation", "persuasion", "positions", "profiles", "transparency",
	"wallet",
}

// envReadAllowlist lists the internal packages allowed to touch the process
// environment directly, per the P02-T01 gate ("nenhum package lê os.Getenv
// fora de config/bootstrap"). cmd/bootstrap joins this list when it appears.
var envReadAllowlist = map[string]bool{
	"internal/platform/config": true,
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	// This test lives in internal/; the repository root is one level above.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	root := filepath.Dir(wd)

	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(data), "module "+modulePath) {
		t.Fatalf("go.mod does not declare module %s", modulePath)
	}
	return root
}

// forbiddenImportForDomainOrApplication reports whether the given import is
// forbidden for domain/application code of the given module, and why.
func forbiddenImportForDomainOrApplication(module, importPath string) (string, bool) {
	switch {
	case strings.HasPrefix(importPath, modulePrefix+"internal/"):
		rest := strings.TrimPrefix(importPath, modulePrefix+"internal/")
		parts := strings.Split(rest, "/")
		switch parts[0] {
		case "platform":
			return "domain/application must not import internal/platform", true
		case "bootstrap":
			return "domain/application must not import internal/bootstrap", true
		case module:
			if len(parts) >= 2 && parts[1] == "adapters" {
				return "domain/application must not import adapters; application defines ports and adapters implement them", true
			}
			return "", false
		default:
			return "cross-module imports are forbidden outside publicly exposed commands, queries and events", true
		}
	case strings.HasPrefix(importPath, modulePrefix):
		return "domain/application must not import cmd/", true
	}

	if reason, forbidden := forbiddenStdlib[importPath]; forbidden {
		return "domain/application must not import " + reason, true
	}
	for _, prefix := range forbiddenExternal {
		if strings.HasPrefix(importPath, prefix) {
			return "unapproved external dependency in domain/application (requires ADR and an adapter)", true
		}
	}
	return "", false
}

// envReadViolation inspects one expression for direct environment access and
// reports a violation message when the containing package is not allowlisted.
func envReadViolation(expression ast.Expr, pkgDir string) (string, bool) {
	call, isCall := expression.(*ast.CallExpr)
	if !isCall {
		return "", false
	}
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	ident, isIdent := selector.X.(*ast.Ident)
	if !isIdent || ident.Name != "os" {
		return "", false
	}
	switch selector.Sel.Name {
	case "Getenv", "LookupEnv", "Setenv", "Unsetenv", "Clearenv", "Expandenv":
	default:
		return "", false
	}
	if envReadAllowlist[pkgDir] {
		return "", false
	}
	return fmt.Sprintf(
		"%s calls os.%s directly — read the environment only in the typed config package (P02-T01 gate)",
		pkgDir, selector.Sel.Name,
	), true
}

func TestDependencyDirectionAndEnvironmentGate(t *testing.T) {
	root := repositoryRoot(t)
	internalDir := filepath.Join(root, "internal")
	fset := token.NewFileSet()

	var violations []string
	walkErr := filepath.WalkDir(internalDir, func(path string, dirEntry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		segments := strings.Split(filepath.ToSlash(relPath), "/")
		pkgDir := strings.Join(segments[:minInt(3, len(segments))], "/")

		source, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", filepath.ToSlash(relPath), err)
			return nil
		}

		// Environment gate applies to every internal package.
		ast.Inspect(source, func(node ast.Node) bool {
			if expression, isExpression := node.(ast.Expr); isExpression {
				if rule, forbidden := envReadViolation(expression, pkgDir); forbidden {
					violations = append(violations, fmt.Sprintf("%s — %s", filepath.ToSlash(relPath), rule))
				}
			}
			return true
		})

		// Dependency direction applies to domain and application layers.
		if len(segments) < 3 {
			return nil
		}
		layer := segments[2]
		if layer != "domain" && layer != "application" {
			return nil
		}
		module := segments[1]
		for _, importSpec := range source.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				t.Errorf("%s: unparsable import %s", filepath.ToSlash(relPath), importSpec.Path.Value)
				continue
			}
			if rule, forbidden := forbiddenImportForDomainOrApplication(module, importPath); forbidden {
				violations = append(violations, fmt.Sprintf(
					"%s (%s) imports %q — %s",
					filepath.ToSlash(relPath), layer, importPath, rule,
				))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/: %v", walkErr)
	}

	if len(violations) > 0 {
		t.Fatalf("architecture violations:\n%s", strings.Join(violations, "\n"))
	}
}

// minInt returns the smaller of two integers.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestBusinessModulesExist(t *testing.T) {
	root := repositoryRoot(t)
	moduleNamePattern := regexp.MustCompile(`const ModuleName = "([^"]+)"`)
	packageNamePattern := regexp.MustCompile(`(?m)^package (\w+)$`)

	seen := make(map[string]bool, len(businessModules))
	for _, module := range businessModules {
		moduleFile := filepath.Join(root, "internal", module, module+".go")
		data, err := os.ReadFile(moduleFile)
		if err != nil {
			t.Errorf("module %s: read %s: %v", module, filepath.ToSlash(moduleFile), err)
			continue
		}
		content := string(data)

		match := packageNamePattern.FindStringSubmatch(content)
		if match == nil || match[1] != module {
			t.Errorf("module %s: file %s.go must declare package %s", module, module, module)
		}
		match = moduleNamePattern.FindStringSubmatch(content)
		if match == nil {
			t.Errorf("module %s: missing ModuleName constant in %s.go", module, module)
			continue
		}
		if match[1] != module {
			t.Errorf("module %s: ModuleName is %q, want %q", module, match[1], module)
		}
		if seen[match[1]] {
			t.Errorf("module %s: duplicate ModuleName %q", module, match[1])
		}
		seen[match[1]] = true
	}
}
