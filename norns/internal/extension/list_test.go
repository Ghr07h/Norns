package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestListSortsSiblingExtensionsWithoutExecuting(t *testing.T) {
	directory := t.TempDir()
	alpha := writeExtensionFixture(t, directory, "list-alpha")
	bravo := writeExtensionFixture(t, directory, "list-bravo")
	zulu := writeExtensionFixture(t, directory, "list-zulu")
	writeExtensionFixture(t, directory, "Bad")
	writeExtensionFixture(t, directory, "1bad")
	writeExtensionFixture(t, directory, "bad_name")
	if err := os.Mkdir(filepath.Join(directory, extensionFixtureName("list-directory")), 0o700); err != nil {
		t.Fatal(err)
	}
	items, err := listDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	want := []Info{{Name: "list-alpha", Path: alpha}, {Name: "list-bravo", Path: bravo}, {Name: "list-zulu", Path: zulu}}
	if len(items) != len(want) {
		t.Fatalf("got %+v, want %+v", items, want)
	}
	for index, item := range items {
		if item != want[index] {
			t.Fatalf("got %+v, want %+v", items, want)
		}
		found, err := findInDirectory(directory, item.Name)
		if err != nil || found != item.Path {
			t.Fatalf("list and dispatch disagree: item=%+v found=%s err=%v", item, found, err)
		}
	}
}

func TestListReturnsAbsolutePathsAndEmptySlice(t *testing.T) {
	workspace := t.TempDir()
	t.Chdir(workspace)
	if err := os.Mkdir("relative", 0o700); err != nil {
		t.Fatal(err)
	}
	writeExtensionFixture(t, "relative", "list-relative")
	items, err := listDirectory("relative")
	want := filepath.Join(workspace, "relative", extensionFixtureName("list-relative"))
	if err != nil || len(items) != 1 || items[0].Path != want || !filepath.IsAbs(items[0].Path) {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	items, err = listDirectory(t.TempDir())
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty list must be non-nil: items=%+v err=%v", items, err)
	}
}

func TestDiscoveryIgnoresEnvironmentAndWorkingDirectory(t *testing.T) {
	plugins, pathDir, workspace := t.TempDir(), t.TempDir(), t.TempDir()
	name := "untrusted-external"
	for _, directory := range []string{plugins, pathDir, workspace} {
		writeExtensionFixture(t, directory, name)
	}
	t.Setenv("NORNS_PLUGIN_PATH", plugins)
	t.Setenv("PATH", pathDir)
	t.Chdir(workspace)
	if _, err := Find(name); err == nil {
		t.Fatal("extension outside the executable directory was found")
	}
	items, err := List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == name {
			t.Fatalf("external extension leaked into list: %+v", item)
		}
	}
}

func extensionFixtureName(name string) string {
	filename := "norns-" + name
	if runtime.GOOS == "windows" {
		filename += ".exe"
	}
	return filename
}

func writeExtensionFixture(t *testing.T, directory, name string) string {
	t.Helper()
	filename := filepath.Join(directory, extensionFixtureName(name))
	// Deliberately not an executable image: listing must only inspect files.
	if err := os.WriteFile(filename, []byte("not executable code"), 0o700); err != nil {
		t.Fatal(err)
	}
	return filename
}
