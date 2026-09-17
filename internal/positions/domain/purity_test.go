package domain_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPositionsDomainCarriesNoPlanOrPaymentWeight is the P09-T02 structural
// proof that a position has no weight by plan or payment: no identifier in
// the package references plans, payments, subscriptions, prices or similar
// concepts. Comments may explain the rule; the code cannot depend on them.
func TestPositionsDomainCarriesNoPlanOrPaymentWeight(t *testing.T) {
	forbidden := []string{
		"plan", "payment", "stripe", "subscription", "premium",
		"price", "billing", "paid", "tier", "member",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read domain directory: %v", err)
	}

	fileSet := token.NewFileSet()
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++

		file, err := parser.ParseFile(fileSet, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			lower := strings.ToLower(identifier.Name)
			for _, marker := range forbidden {
				if strings.Contains(lower, marker) {
					t.Errorf("PRODUCT VIOLATION: %s declares identifier %q containing %q", name, identifier.Name, marker)
				}
			}
			return true
		})
	}

	if checked == 0 {
		t.Fatal("no domain source files were checked; the proof would pass vacuously")
	}
}
