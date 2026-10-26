package traceability

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

type manifest struct {
	Requirements []requirementCoverage `json:"requirements"`
}

type requirementCoverage struct {
	Source string   `json:"source"`
	Key    string   `json:"key"`
	Tests  []string `json:"tests"`
}

var coverageKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

func TestRequirementsManifestReferencesKnownTests(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testdata", "requirements.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document manifest
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	knownTests := testFunctions(t, filepath.Join(root, "internal"))
	seenSources := map[string]bool{}
	seenKeys := map[string]bool{}
	for _, item := range document.Requirements {
		if strings.TrimSpace(item.Source) == "" {
			t.Error("requirement source must not be empty")
		}
		if seenSources[item.Source] {
			t.Errorf("duplicate requirement source %s", item.Source)
		}
		seenSources[item.Source] = true
		if !coverageKeyPattern.MatchString(item.Key) {
			t.Errorf("invalid requirement key %q", item.Key)
		}
		if seenKeys[item.Key] {
			t.Errorf("duplicate requirement key %s", item.Key)
		}
		seenKeys[item.Key] = true
		if len(item.Tests) == 0 {
			t.Errorf("requirement %s has no tests", item.Source)
		}
		for _, name := range item.Tests {
			if !knownTests[name] {
				t.Errorf("requirement %s references missing test %s", item.Source, name)
			}
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate traceability test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
}

func testFunctions(t *testing.T, root string) map[string]bool {
	t.Helper()
	known := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Test") {
				known[function.Name.Name] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return known
}
