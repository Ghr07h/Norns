package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStructuredViewGoldenFiles(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	service, explorationID := goldenViewWorkspace(t)
	fixtures := []struct {
		name string
		load func() ([]byte, error)
	}{
		{"status.yaml", func() ([]byte, error) {
			content, _, err := service.StatusView(context.Background(), nil, false)
			return content, err
		}},
		{"status.json", func() ([]byte, error) {
			content, _, err := service.StatusView(context.Background(), nil, true)
			return content, err
		}},
		{"trace.yaml", func() ([]byte, error) {
			content, _, err := service.TraceView(context.Background(), explorationID, false)
			return content, err
		}},
		{"trace.json", func() ([]byte, error) {
			content, _, err := service.TraceView(context.Background(), explorationID, true)
			return content, err
		}},
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate golden test source")
	}
	directory := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "testdata", "views"))
	if os.Getenv("NORNS_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			actual, err := fixture.load()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, fixture.name)
			if os.Getenv("NORNS_UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(path, actual, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			expected, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, expected) {
				t.Fatalf("View differs from %s\n--- expected ---\n%s\n--- actual ---\n%s", path, expected, actual)
			}
		})
	}
}

func goldenViewWorkspace(t *testing.T) (*Service, string) {
	t.Helper()
	service, _ := newTestService(t)
	info := "Subject information"
	opened, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Investigate", Concurrency: 2, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	originID := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	global, err := service.AddCognition(context.Background(), AddCognitionRequest{Title: "Global context", Content: "Shared context", Origin: "user"})
	if err != nil {
		t.Fatal(err)
	}
	exploration, err := service.AddExploration(context.Background(), AddExplorationRequest{Title: "Inspect", Content: "Inspect evidence", Sources: []string{originID, resultID(t, global)}})
	if err != nil {
		t.Fatal(err)
	}
	explorationID := resultID(t, exploration)
	if _, err := service.AddCognition(context.Background(), AddCognitionRequest{Title: "Finding", Content: "Evidence found", Origin: "exploration", ExplorationID: &explorationID, Relevant: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AssignExploration(context.Background(), explorationID, "executor-1", "output://one"); err != nil {
		t.Fatal(err)
	}
	return service, explorationID
}
