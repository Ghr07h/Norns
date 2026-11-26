// Package releasefiles stages explicitly declared extension files and publishes
// only Norns-owned artifacts, preserving files belonging to other products.
package releasefiles

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Ghr07h/norns/norns/internal/extension/metadata"
)

type File struct {
	Directory string
	Spec      metadata.ReleaseFile
	Target    string
}

func Plan(extensionsDir string, catalog []metadata.Metadata) ([]File, error) {
	files := []File{}
	seen := map[string]bool{}
	for _, extension := range catalog {
		for _, spec := range extension.ReleaseFiles {
			target := spec.PublishedName(extension.Name)
			key := strings.ToLower(target)
			if spec.ValidateFor(extension.Name) != nil || seen[key] {
				return nil, fmt.Errorf("extension %q: invalid or conflicting release file target", extension.Name)
			}
			directory := filepath.Join(extensionsDir, extension.Name)
			file, err := openSource(directory, spec.OriginFile)
			if err != nil {
				return nil, fmt.Errorf("extension %q: release file must be a regular file inside its source directory", extension.Name)
			}
			file.Close()
			seen[key] = true
			files = append(files, File{Directory: directory, Spec: spec, Target: target})
		}
	}
	return files, nil
}

func openSource(directory, origin string) (*os.File, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var before os.FileInfo
	parts := strings.Split(strings.TrimPrefix(origin, "./"), "/")
	for index := range parts {
		before, err = root.Lstat(filepath.Join(parts[:index+1]...))
		if err != nil || before.Mode()&os.ModeSymlink != 0 || (index < len(parts)-1 && !before.IsDir()) {
			return nil, errors.New("invalid release file source")
		}
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("release source is not a regular file")
	}
	file, err := root.Open(filepath.FromSlash(origin))
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		file.Close()
		return nil, errors.New("release file source changed")
	}
	return file, nil
}

func Stage(files []File, destination string) error {
	for _, item := range files {
		source, err := openSource(item.Directory, item.Spec.OriginFile)
		if err != nil {
			return fmt.Errorf("cannot read declared extension release file")
		}
		info, err := source.Stat()
		if err != nil {
			source.Close()
			return err
		}
		output, err := os.OpenFile(filepath.Join(destination, item.Target), os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(output, source)
		source.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func declaredTargets(directory string, required bool) (map[string]bool, error) {
	data, err := metadata.ReadFile(filepath.Join(directory, metadata.Filename), metadata.MaxFileBytes)
	if !required && errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, errors.New("cannot read extension release metadata")
	}
	catalog, err := metadata.ParseCatalog(data)
	if err != nil {
		return nil, errors.New("invalid extension release metadata")
	}
	result := map[string]bool{}
	for _, extension := range catalog {
		for _, spec := range extension.ReleaseFiles {
			name := spec.PublishedName(extension.Name)
			key := strings.ToLower(name)
			if result[key] {
				return nil, errors.New("conflicting extension release file targets")
			}
			result[key] = true
		}
	}
	return result, nil
}

func ownedCore(name string) bool {
	return name == "norns" || name == "norns.exe" || name == metadata.Filename || strings.HasPrefix(name, "norns-")
}

// Publish first validates ownership and collisions. Arbitrary existing files are
// never overwritten just because a new extension requests their names.
func Publish(source, destination string) error {
	current, err := os.ReadDir(destination)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	oldFiles, err := declaredTargets(destination, false)
	if err != nil {
		return err
	}
	newFiles, err := declaredTargets(source, true)
	if err != nil {
		return err
	}
	sourceFiles := map[string]bool{}
	for _, entry := range entries {
		key := strings.ToLower(entry.Name())
		if newFiles[key] {
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("staged extension release file is not a regular file")
			}
			sourceFiles[key] = true
		}
	}
	for key := range newFiles {
		if !sourceFiles[key] {
			return fmt.Errorf("declared extension release file is missing from staging")
		}
	}
	for _, entry := range current {
		key := strings.ToLower(entry.Name())
		if oldFiles[key] && entry.IsDir() {
			return fmt.Errorf("refusing to remove a directory in place of an extension release file")
		}
		if newFiles[key] && !oldFiles[key] {
			return fmt.Errorf("extension release file conflicts with an existing unowned artifact")
		}
	}
	for _, entry := range current {
		name := entry.Name()
		if oldFiles[strings.ToLower(name)] {
			if err := os.Remove(filepath.Join(destination, name)); err != nil {
				return err
			}
		} else if ownedCore(name) {
			if err := os.RemoveAll(filepath.Join(destination, name)); err != nil {
				return err
			}
		}
	}
	for _, entry := range entries {
		name := entry.Name()
		if ownedCore(name) || newFiles[strings.ToLower(name)] {
			if err := os.Rename(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
				return err
			}
		}
	}
	return nil
}
