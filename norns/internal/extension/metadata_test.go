package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

func TestMetadataOnlyEnrichesDiscoveredExtensions(t *testing.T) {
	directory := t.TempDir()
	writeExtensionFixture(t, directory, "lookup")
	writeExtensionFixture(t, directory, "without-metadata")
	items, err := listDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := enrichFromDirectory(directory, items); err != nil {
		t.Fatal("missing catalog must fall back silently:", err)
	}
	source := `[{"name":"lookup","version":"0.1.0","description":"Literal $SECRET","requirements":[]},{"name":"not-installed","version":"1","description":"Not installed","requirements":[]}]`
	filename := filepath.Join(directory, metadata.Filename)
	if err := os.WriteFile(filename, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRET", "must-not-be-expanded")
	if err := enrichFromDirectory(directory, items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "lookup" || items[0].Description != "Literal $SECRET" || items[0].Requirements == nil || len(*items[0].Requirements) != 0 || items[1].Version != "" || items[1].Requirements != nil {
		t.Fatalf("wrong metadata projection: %+v", items)
	}
	if err := os.WriteFile(filename, []byte(source[:len(source)-1]+",{}]"), 0o600); err != nil {
		t.Fatal(err)
	}
	bare, _ := listDirectory(directory)
	if err := enrichFromDirectory(directory, bare); err == nil || bare[0].Version != "" || bare[0].Requirements != nil {
		t.Fatal("malformed catalog must not apply partial metadata")
	}
}
