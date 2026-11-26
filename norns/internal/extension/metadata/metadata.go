// Package metadata validates static extension descriptions. It never executes
// extensions, expands variables, or inspects runtime configuration.
package metadata

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const Filename = "norns.exts.meta.json"
const MaxFileBytes = 1024 * 1024
const MaxSourceBytes = 16 * 1024

type Metadata struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Description  string        `json:"description"`
	Requirements []string      `json:"requirements"`
	ReleaseFiles []ReleaseFile `json:"release_files"`
}

type ReleaseFile struct {
	OriginFile string `json:"origin_file"`
	TargetFile string `json:"target_file"`
}

func (f ReleaseFile) TargetName() string {
	if f.TargetFile != "" {
		return f.TargetFile
	}
	return path.Base(f.OriginFile)
}

func (f ReleaseFile) PublishedName(extensionName string) string {
	return "norns.exts." + extensionName + "." + f.TargetName()
}

func (f ReleaseFile) ValidateFor(extensionName string) error {
	if len(extensionName) > 128 || !validName.MatchString(extensionName) || f.Validate() != nil || !ValidTargetName(f.PublishedName(extensionName)) || f.PublishedName(extensionName) == Filename {
		return invalid
	}
	return nil
}

func (f ReleaseFile) Validate() error {
	origin := strings.TrimPrefix(f.OriginFile, "./")
	if len(origin) > 1024 || !fs.ValidPath(origin) || origin == "." || strings.ContainsAny(origin, `\:`) {
		return invalid
	}
	if !ValidTargetName(f.TargetName()) {
		return invalid
	}
	return nil
}

// ValidTargetName checks portable single filenames. Product isolation is provided
// by PublishedName; ValidateFor also reserves the shared metadata filename.
func ValidTargetName(name string) bool {
	if name == "" || len(name) > 255 || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, `/\:*?"<>|`) {
		return false
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var invalid = errors.New("invalid extension metadata")

func (m Metadata) validate() error {
	if len(m.Name) > 128 || !validName.MatchString(m.Name) || strings.TrimSpace(m.Version) == "" || utf8.RuneCountInString(m.Version) > 64 || strings.TrimSpace(m.Description) == "" || utf8.RuneCountInString(m.Description) > 2048 || m.Requirements == nil || len(m.Requirements) > 32 {
		return invalid
	}
	for _, requirement := range m.Requirements {
		if strings.TrimSpace(requirement) == "" || utf8.RuneCountInString(requirement) > 1024 {
			return invalid
		}
	}
	if len(m.ReleaseFiles) > 64 {
		return invalid
	}
	seen := map[string]bool{}
	for _, file := range m.ReleaseFiles {
		target := strings.ToLower(file.PublishedName(m.Name))
		if file.ValidateFor(m.Name) != nil || seen[target] {
			return invalid
		}
		seen[target] = true
	}
	return nil
}

// ReadFile accepts only a bounded regular file, and checks its identity before
// reading so a symbolic link cannot redirect a metadata read to another file.
func ReadFile(filename string, limit int64) ([]byte, error) {
	before, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, invalid
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, invalid
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, invalid
	}
	return data, nil
}

func ParseSource(data []byte, expectedName string) (Metadata, error) {
	var result Metadata
	if len(data) > MaxSourceBytes || decode(data, &result) != nil || result.validate() != nil || result.Name != expectedName {
		return Metadata{}, invalid
	}
	if result.ReleaseFiles == nil {
		result.ReleaseFiles = []ReleaseFile{}
	}
	return result, nil
}

func ParseCatalog(data []byte) ([]Metadata, error) {
	var result []Metadata
	if len(data) > MaxFileBytes || decode(data, &result) != nil || result == nil || len(result) > 256 {
		return nil, invalid
	}
	seen := map[string]bool{}
	for index, item := range result {
		if item.validate() != nil || seen[item.Name] {
			return nil, invalid
		}
		seen[item.Name] = true
		if item.ReleaseFiles == nil {
			result[index].ReleaseFiles = []ReleaseFile{}
		}
	}
	return result, nil
}

func decode(data []byte, value any) error {
	if !utf8.Valid(data) {
		return invalid
	}
	// encoding/json otherwise silently accepts duplicate object keys.
	check := json.NewDecoder(bytes.NewReader(data))
	if checkKeys(check, 0) != nil {
		return invalid
	}
	if _, err := check.Token(); err != io.EOF {
		return invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func checkKeys(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return invalid
	}
	token, err := decoder.Token()
	if err != nil {
		return invalid
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return invalid
			}
			switch name {
			case "name", "version", "description", "requirements", "release_files", "origin_file", "target_file":
			default:
				return invalid
			}
			seen[name] = true
			if err := checkKeys(decoder, depth+1); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for decoder.More() {
			if err := checkKeys(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = decoder.Token()
	return err
}
