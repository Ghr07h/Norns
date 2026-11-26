package extension

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

// Enrich adds static metadata only to discovered extensions. Callers can keep
// names and paths when this returns an error; no partial metadata is applied.
func Enrich(items []Info) error {
	if len(items) == 0 {
		return nil
	}
	directory, err := executableDirectory()
	if err != nil {
		return err
	}
	return enrichFromDirectory(directory, items)
}

func enrichFromDirectory(directory string, items []Info) error {
	data, err := metadata.ReadFile(filepath.Join(directory, metadata.Filename), metadata.MaxFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	catalog, err := metadata.ParseCatalog(data)
	if err != nil {
		return err
	}
	byName := make(map[string]metadata.Metadata, len(catalog))
	for _, item := range catalog {
		byName[item.Name] = item
	}
	for index := range items {
		if item, ok := byName[items[index].Name]; ok {
			items[index].Version = item.Version
			items[index].Description = item.Description
			items[index].Requirements = &item.Requirements
			items[index].ReleaseFiles = &item.ReleaseFiles
		}
	}
	return nil
}
