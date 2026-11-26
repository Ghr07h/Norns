package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Ghr07h/norns/norns/internal/domain"
	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

type Info struct {
	Name         string                  `json:"name"`
	Path         string                  `json:"path"`
	Version      string                  `json:"version,omitempty"`
	Description  string                  `json:"description,omitempty"`
	Requirements *[]string               `json:"requirements,omitempty"`
	ReleaseFiles *[]metadata.ReleaseFile `json:"release_files,omitempty"`
}

// List discovers extensions beside the actual Norns executable without executing
// them. It uses the same directory and file checks as command dispatch.
func List() ([]Info, error) {
	directory, err := executableDirectory()
	if err != nil {
		return nil, err
	}
	return listDirectory(directory)
}

func listDirectory(directory string) ([]Info, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, domain.Wrap("IO_ERROR", "cannot resolve Norns extension directory", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, domain.Wrap("IO_ERROR", "cannot read Norns extension directory", err)
	}
	paths := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filename := entry.Name()
		if runtime.GOOS == "windows" {
			filename = strings.ToLower(filename)
			if !strings.HasSuffix(filename, ".exe") {
				continue
			}
			filename = strings.TrimSuffix(filename, ".exe")
		}
		name, prefixed := strings.CutPrefix(filename, "norns-")
		if !prefixed || !IsValidName(name) {
			continue
		}
		path, err := findInDirectory(directory, name)
		if err != nil {
			continue
		}
		paths[name] = path
	}
	result := make([]Info, 0, len(paths))
	for name, path := range paths {
		result = append(result, Info{Name: name, Path: path})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
