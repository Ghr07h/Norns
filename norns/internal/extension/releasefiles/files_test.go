package releasefiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func catalog(t *testing.T, directory string, files ...metadata.ReleaseFile) {
	t.Helper()
	data, err := json.Marshal([]metadata.Metadata{{Name: "lookup", Version: "0.1.0", Description: "Lookup", Requirements: []string{}, ReleaseFiles: files}})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(directory, metadata.Filename), string(data))
}

func TestStageDeclaredSingleFiles(t *testing.T) {
	extensions, staging := t.TempDir(), t.TempDir()
	write(t, filepath.Join(extensions, "lookup/configs/example.yaml"), "public template")
	write(t, filepath.Join(extensions, "lookup/notes.txt"), "notes")
	files := []metadata.ReleaseFile{{OriginFile: "configs/example.yaml", TargetFile: "lookup.example.yaml"}, {OriginFile: "./notes.txt", TargetFile: ""}}
	plan, err := Plan(extensions, []metadata.Metadata{{Name: "lookup", ReleaseFiles: files}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Stage(plan, staging); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"norns.exts.lookup.lookup.example.yaml": "public template", "norns.exts.lookup.notes.txt": "notes"} {
		if data, err := os.ReadFile(filepath.Join(staging, name)); err != nil || string(data) != want {
			t.Fatalf("%s: %s %v", name, data, err)
		}
	}
}

func TestPlanRejectsInvalidSourcesAndDuplicateTargets(t *testing.T) {
	extensions := t.TempDir()
	write(t, filepath.Join(extensions, "lookup/config.yaml"), "example")
	write(t, filepath.Join(extensions, "other/config.yaml"), "example")
	for _, origin := range []string{"../outside", "configs/../../outside", "/etc/passwd", `..\outside`, "missing", "."} {
		if _, err := Plan(extensions, []metadata.Metadata{{Name: "lookup", ReleaseFiles: []metadata.ReleaseFile{{OriginFile: origin, TargetFile: "example.yaml"}}}}); err == nil {
			t.Fatalf("accepted source %q", origin)
		}
	}
	if _, err := Plan(extensions, []metadata.Metadata{
		{Name: "lookup", ReleaseFiles: []metadata.ReleaseFile{{OriginFile: "config.yaml", TargetFile: "same.yaml"}}},
		{Name: "lookup", ReleaseFiles: []metadata.ReleaseFile{{OriginFile: "config.yaml", TargetFile: "SAME.yaml"}}},
	}); err == nil {
		t.Fatal("case-insensitive duplicate target accepted")
	}
	plan, err := Plan(extensions, []metadata.Metadata{
		{Name: "lookup", ReleaseFiles: []metadata.ReleaseFile{{OriginFile: "config.yaml", TargetFile: "same.yaml"}}},
		{Name: "other", ReleaseFiles: []metadata.ReleaseFile{{OriginFile: "config.yaml", TargetFile: "same.yaml"}}},
	})
	if err != nil || len(plan) != 2 || plan[0].Target != "norns.exts.lookup.same.yaml" || plan[1].Target != "norns.exts.other.same.yaml" {
		t.Fatalf("different plugins should have separate filenames: %+v %v", plan, err)
	}
	if runtime.GOOS != "windows" {
		external := t.TempDir()
		write(t, filepath.Join(external, "secret"), "private")
		for link, target := range map[string]string{"file-link": filepath.Join(external, "secret"), "dir-link": external} {
			if err := os.Symlink(target, filepath.Join(extensions, "lookup", link)); err != nil {
				t.Fatal(err)
			}
		}
		for _, origin := range []string{"file-link", "dir-link/secret"} {
			if _, err := Plan(extensions, []metadata.Metadata{{Name: "lookup", ReleaseFiles: []metadata.ReleaseFile{{OriginFile: origin, TargetFile: "template.yaml"}}}}); err == nil {
				t.Fatalf("accepted symbolic link %s", origin)
			}
		}
	}
}

func TestPublishRemovesOnlyPreviouslyDeclaredFiles(t *testing.T) {
	source, destination := t.TempDir(), t.TempDir()
	catalog(t, destination, metadata.ReleaseFile{OriginFile: "old.yaml", TargetFile: "old-template.yaml"})
	write(t, filepath.Join(destination, "norns.exts.lookup.old-template.yaml"), "old")
	write(t, filepath.Join(destination, "unrelated.txt"), "preserve")
	write(t, filepath.Join(destination, "nornsd"), "preserve")
	catalog(t, source, metadata.ReleaseFile{OriginFile: "new.yaml", TargetFile: "new-template.yaml"})
	write(t, filepath.Join(source, "norns.exts.lookup.new-template.yaml"), "new")
	write(t, filepath.Join(source, "norns"), "new binary")
	write(t, filepath.Join(source, "nornsd.example.yaml"), "other product")
	if err := Publish(source, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "norns.exts.lookup.old-template.yaml")); !os.IsNotExist(err) {
		t.Fatal("old attachment retained")
	}
	for _, name := range []string{"unrelated.txt", "nornsd"} {
		if data, _ := os.ReadFile(filepath.Join(destination, name)); string(data) != "preserve" {
			t.Fatal("unowned artifact changed")
		}
	}
	if _, err := os.Stat(filepath.Join(source, "nornsd.example.yaml")); err != nil {
		t.Fatal("publisher consumed another product's artifact")
	}
	if data, _ := os.ReadFile(filepath.Join(destination, "norns.exts.lookup.new-template.yaml")); string(data) != "new" {
		t.Fatal("new file not published")
	}
}

func TestPublishRejectsCollisionsBeforeChangingArtifacts(t *testing.T) {
	for _, missing := range []bool{false, true} {
		source, destination := t.TempDir(), t.TempDir()
		catalog(t, source, metadata.ReleaseFile{OriginFile: "template.yaml", TargetFile: "template.yaml"})
		if !missing {
			write(t, filepath.Join(source, "norns.exts.lookup.template.yaml"), "new")
		}
		write(t, filepath.Join(destination, "norns.exts.lookup.template.yaml"), "private existing file")
		write(t, filepath.Join(destination, "norns"), "old binary")
		if err := Publish(source, destination); err == nil {
			t.Fatal("unsafe publication accepted")
		}
		if data, _ := os.ReadFile(filepath.Join(destination, "norns.exts.lookup.template.yaml")); string(data) != "private existing file" {
			t.Fatal("unowned file changed")
		}
		if data, _ := os.ReadFile(filepath.Join(destination, "norns")); string(data) != "old binary" {
			t.Fatal("binary changed before validation completed")
		}
	}
}
