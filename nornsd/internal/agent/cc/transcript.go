package cc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"github.com/Ghr07h/norns/nornsd/internal/root"
)

type TranscriptResult struct {
	Status           string   `json:"status"`
	SessionID        string   `json:"session_id"`
	RuntimeDirectory string   `json:"runtime_directory"`
	Copied           []string `json:"copied"`
	Missing          []string `json:"missing"`
}
type transcriptManifest struct {
	Version   int               `json:"version"`
	SessionID string            `json:"session_id"`
	RuntimeID string            `json:"runtime_id"`
	Files     map[string]string `json:"files"`
	Missing   []string          `json:"missing"`
}

func ProjectKey(path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", err
	}
	canonical = norm.NFC.String(canonical)
	var b strings.Builder
	for _, r := range canonical {
		if r > 127 || !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			b.WriteByte('-')
		} else {
			b.WriteRune(r)
		}
	}
	sanitized := b.String()
	if len(sanitized) <= 200 {
		return sanitized, nil
	}
	var hash int32
	for _, r := range canonical {
		hash = hash*31 + int32(r)
	}
	number := int64(hash)
	if number < 0 {
		number = -number
	}
	return sanitized[:200] + "-" + strconv.FormatInt(number, 36), nil
}

func ArchiveTranscript(subjectPath, sessionID, runtimeID string) (TranscriptResult, error) {
	runtimeRel := filepath.ToSlash(filepath.Join(".nornsd", "runtimes", runtimeID))
	result := TranscriptResult{SessionID: sessionID, RuntimeDirectory: runtimeRel, Copied: []string{}, Missing: []string{}}
	runtimeRoot := filepath.Join(subjectPath, ".nornsd", "runtimes", runtimeID)
	manifestPath := filepath.Join(runtimeRoot, "transcript.manifest.json")
	if b, err := os.ReadFile(manifestPath); err == nil {
		var manifest transcriptManifest
		if json.Unmarshal(b, &manifest) == nil && manifest.RuntimeID == runtimeID {
			result.SessionID = manifest.SessionID
			result.Copied = manifestKeys(manifest.Files)
			result.Missing = manifest.Missing
			if len(result.Copied) == 0 {
				result.Status = "missing"
			} else if len(result.Missing) > 0 {
				result.Status = "partial"
			} else {
				result.Status = "complete"
			}
			return result, nil
		}
	}
	configRoot := os.Getenv("CLAUDE_CONFIG_DIR")
	if configRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return result, err
		}
		configRoot = filepath.Join(home, ".claude")
	}
	key, err := ProjectKey(subjectPath)
	if err != nil {
		return result, err
	}
	project := filepath.Join(configRoot, "projects", key)
	sources := []struct {
		name, path string
		dir        bool
	}{{sessionID + ".jsonl", filepath.Join(project, sessionID+".jsonl"), false}, {sessionID, filepath.Join(project, sessionID), true}}
	stage := filepath.Join(subjectPath, ".nornsd", "staging", "transcript-"+runtimeID+"-"+uuid.NewString())
	defer os.RemoveAll(stage)
	if err = os.MkdirAll(stage, 0700); err != nil {
		return result, err
	}
	manifest := transcriptManifest{Version: 1, SessionID: sessionID, RuntimeID: runtimeID, Files: map[string]string{}, Missing: []string{}}
	for _, source := range sources {
		info, statErr := os.Lstat(source.path)
		if os.IsNotExist(statErr) {
			manifest.Missing = append(manifest.Missing, source.name)
			continue
		}
		if statErr != nil {
			return result, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("transcript source %s is a symbolic link", source.name)
		}
		target := filepath.Join(stage, source.name)
		if source.dir {
			if !info.IsDir() {
				return result, fmt.Errorf("transcript source %s is not a directory", source.name)
			}
			if err = root.CopyTreeNoLinks(source.path, target); err != nil {
				return result, err
			}
		} else {
			if !info.Mode().IsRegular() {
				return result, fmt.Errorf("transcript source %s is not a regular file", source.name)
			}
			if err = copyFile(source.path, target); err != nil {
				return result, err
			}
		}
		hashes, hashErr := hashTree(target, source.name)
		if hashErr != nil {
			return result, hashErr
		}
		for k, v := range hashes {
			manifest.Files[k] = v
		}
	}
	if err = os.MkdirAll(runtimeRoot, 0700); err != nil {
		return result, err
	}
	for _, source := range sources {
		staged := filepath.Join(stage, source.name)
		if _, statErr := os.Stat(staged); statErr != nil {
			continue
		}
		destination := filepath.Join(runtimeRoot, source.name)
		if _, existsErr := os.Stat(destination); existsErr == nil {
			return result, fmt.Errorf("transcript destination already exists without a committed manifest: %s", source.name)
		}
		if err = os.Rename(staged, destination); err != nil {
			return result, err
		}
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return result, err
	}
	b = append(b, '\n')
	temporary := manifestPath + "." + uuid.NewString() + ".tmp"
	if err = os.WriteFile(temporary, b, 0600); err != nil {
		return result, err
	}
	if err = os.Rename(temporary, manifestPath); err != nil {
		_ = os.Remove(temporary)
		return result, err
	}
	result.Copied = manifestKeys(manifest.Files)
	result.Missing = manifest.Missing
	if len(result.Copied) == 0 {
		result.Status = "missing"
	} else if len(result.Missing) > 0 {
		result.Status = "partial"
	} else {
		result.Status = "complete"
	}
	return result, nil
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	return errorsJoin(copyErr, syncErr, closeErr)
}
func hashTree(path, prefix string) (map[string]string, error) {
	result := map[string]string{}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		result[filepath.ToSlash(prefix)] = "sha256:" + hex.EncodeToString(sum[:])
		return result, nil
	}
	err = filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in transcript")
		}
		b, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(path, current)
		sum := sha256.Sum256(b)
		result[filepath.ToSlash(filepath.Join(prefix, rel))] = "sha256:" + hex.EncodeToString(sum[:])
		return nil
	})
	return result, err
}
func manifestKeys(files map[string]string) []string {
	seen := map[string]bool{}
	var result []string
	for path := range files {
		top := strings.Split(path, "/")[0]
		if !seen[top] {
			seen[top] = true
			result = append(result, top)
		}
	}
	sort.Strings(result)
	return result
}
func errorsJoin(values ...error) error {
	for _, err := range values {
		if err != nil {
			return err
		}
	}
	return nil
}
