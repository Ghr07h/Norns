package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/workspace"
)

func TestViewPublicationFailuresReturnIOErrorWithoutTemporaryFiles(t *testing.T) {
	viewCalls := []struct {
		name string
		call func(*Service, string) error
	}{
		{"status", func(service *Service, _ string) error {
			_, _, err := service.StatusView(context.Background(), nil, false)
			return err
		}},
		{"trace", func(service *Service, explorationID string) error {
			_, _, err := service.TraceView(context.Background(), explorationID, false)
			return err
		}},
		{"visual", func(service *Service, _ string) error {
			_, err := service.VisualView(context.Background(), false, "")
			return err
		}},
	}
	failures := []struct {
		name      string
		configure func(*failingFileOps, string)
	}{
		{"sync-file", func(files *failingFileOps, viewDir string) {
			files.failSyncFile = func(path string) bool { return filepath.Dir(path) == viewDir }
		}},
		{"rename", func(files *failingFileOps, viewDir string) {
			files.failRename = func(oldPath, newPath string) bool {
				return filepath.Dir(oldPath) == viewDir && filepath.Dir(newPath) == viewDir
			}
		}},
		{"sync-directory", func(files *failingFileOps, viewDir string) {
			files.failSyncDir = func(path string) bool { return path == viewDir }
		}},
	}

	for _, view := range viewCalls {
		for _, failure := range failures {
			t.Run(view.name+"/"+failure.name, func(t *testing.T) {
				service, explorationID := serviceWithExploration(t)
				files := &failingFileOps{base: workspace.OSFileOps{}}
				failure.configure(files, service.Paths.View)
				service.Files = files

				if err := view.call(service, explorationID); errorCode(err) != "IO_ERROR" || !files.failed {
					t.Fatalf("View did not report injected publication failure: %v", err)
				}
				entries, err := os.ReadDir(service.Paths.View)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".tmp") {
						t.Fatalf("temporary View remains after publication failure: %s", entry.Name())
					}
				}
			})
		}
	}
}

func serviceWithExploration(t *testing.T) (*Service, string) {
	t.Helper()
	service, _ := newTestService(t)
	info := "Subject"
	opened, err := service.OpenTask(context.Background(), OpenTaskRequest{Purpose: "Views", Concurrency: 1, Info: &info})
	if err != nil {
		t.Fatal(err)
	}
	origin := opened.Data.(map[string]any)["init_cognition_ref"].(string)
	return service, addExplorationForTest(t, service, origin, "View publication")
}
