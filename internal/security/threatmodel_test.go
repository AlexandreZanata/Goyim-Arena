package security_test

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var threatIDPattern = regexp.MustCompile(`\*\*(THR-[A-Z0-9-]+)\*\*`)
var matrixRowPattern = regexp.MustCompile(`^\| (THR-[A-Z0-9-]+) \| (Crítica|Alta|Média) \| (test|procedure): `)

func TestThreatModelHasExecutableEvidenceForEveryThreat(t *testing.T) {
	modelIDs := readThreatIDs(t, "../../docs/THREAT_MODEL.md")
	matrixIDs, evidence := readMatrix(t, "../../docs/THREAT_MODEL_TEST_MATRIX.md")

	if got, want := strings.Join(modelIDs, "\n"), strings.Join(matrixIDs, "\n"); got != want {
		t.Fatalf("threat IDs differ between model and matrix\nmodel:\n%s\nmatrix:\n%s", got, want)
	}

	for id, entry := range evidence {
		if entry.severity == "Crítica" && entry.kind != "test" {
			t.Errorf("critical threat %s must have automated test evidence, got %s", id, entry.kind)
		}
	}
}

func readThreatIDs(t *testing.T, path string) []string {
	t.Helper()
	contents := readFile(t, path)
	seen := make(map[string]bool)
	var ids []string
	for _, match := range threatIDPattern.FindAllStringSubmatch(contents, -1) {
		id := match[1]
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

type matrixEvidence struct {
	severity string
	kind     string
}

func readMatrix(t *testing.T, path string) ([]string, map[string]matrixEvidence) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open threat matrix: %v", err)
	}
	defer file.Close()

	seen := make(map[string]bool)
	evidence := make(map[string]matrixEvidence)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		matches := matrixRowPattern.FindStringSubmatch(scanner.Text())
		if len(matches) == 0 {
			continue
		}
		id := matches[1]
		if seen[id] {
			t.Fatalf("threat %s appears more than once in the matrix", id)
		}
		seen[id] = true
		evidence[id] = matrixEvidence{severity: matches[2], kind: matches[3]}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read threat matrix: %v", err)
	}

	ids := make([]string, 0, len(evidence))
	for id := range evidence {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, evidence
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
