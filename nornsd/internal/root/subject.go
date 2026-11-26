package root

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ghr07h/norns/nornsd/internal/domain"
)

type SubjectIdentity struct {
	FormatVersion int       `json:"format_version"`
	SubjectID     string    `json:"subject_id"`
	DirectoryName string    `json:"directory_name"`
	CreatedAt     time.Time `json:"created_at"`
}

func DirectoryName(now time.Time, id string) string {
	return id + "_" + now.UTC().Format("20060102T150405.000000000Z")
}

func (o *Owner) MaterializeSubject(ctx context.Context, subject domain.Subject) (string, error) {
	workspace, err := o.SubjectPath(subject.DirectoryName)
	if err != nil {
		return "", err
	}
	if _, err := uuid.Parse(subject.ID); err != nil || !strings.HasPrefix(subject.DirectoryName, subject.ID+"_") {
		return "", fmt.Errorf("invalid Subject directory identity")
	}
	metadata := o.SubjectData(subject.ID)
	identity := SubjectIdentity{2, subject.ID, subject.DirectoryName, subject.CreatedAt.UTC()}
	existing, readErr := ReadSubjectIdentity(workspace)
	if readErr == nil {
		if existing.SubjectID != subject.ID || existing.DirectoryName != subject.DirectoryName {
			return "", domain.NewError("WORKSPACE_UNAVAILABLE", "subject identity conflicts with database", http.StatusConflict)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	} else {
		if _, err := os.Lstat(workspace); err == nil {
			return "", domain.NewError("WORKSPACE_UNAVAILABLE", "unregistered workspace already exists", http.StatusConflict)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		stage := filepath.Join(o.Control, "staging", "subject-"+subject.ID+"-"+uuid.NewString())
		defer os.RemoveAll(stage)
		for _, dir := range []string{"logs", "runtimes", "staging"} {
			if err = os.MkdirAll(filepath.Join(stage, dir), 0700); err != nil {
				return "", err
			}
		}
		if err = writeJSONAtomic(filepath.Join(stage, "subject.json"), identity, 0600); err != nil {
			return "", err
		}
		for _, name := range []string{"raw.jsonl", "readable.jsonl"} {
			if err = os.WriteFile(filepath.Join(stage, "logs", name), nil, 0600); err != nil {
				return "", err
			}
		}
		if err = os.Rename(stage, metadata); err != nil {
			return "", err
		}
	}
	if info, statErr := os.Lstat(workspace); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Subject workspace must be a directory, not a symlink")
		}
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}
	if err = os.MkdirAll(workspace, 0700); err != nil {
		return "", err
	}
	if err = o.Store.MarkMaterialized(ctx, subject.ID, time.Now().UTC()); err != nil {
		return "", err
	}
	return workspace, nil
}
func ReadSubjectIdentity(workspace string) (SubjectIdentity, error) {
	return ReadSubjectDataIdentity(SubjectDataPath(workspace))
}
func ReadSubjectDataIdentity(metadata string) (SubjectIdentity, error) {
	var id SubjectIdentity
	for _, path := range []string{metadata, filepath.Join(metadata, "subject.json")} {
		if info, err := os.Lstat(path); err != nil {
			return id, err
		} else if info.Mode()&os.ModeSymlink != 0 {
			return id, fmt.Errorf("Subject identity cannot be a symlink")
		}
	}
	b, err := os.ReadFile(filepath.Join(metadata, "subject.json"))
	if err != nil {
		return id, err
	}
	if err = json.Unmarshal(b, &id); err != nil {
		return id, err
	}
	if id.FormatVersion != 2 || id.SubjectID == "" || id.DirectoryName == "" {
		return id, fmt.Errorf("invalid subject identity")
	}
	return id, nil
}

func EnsureWithin(base, target string) (string, error) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes allowed root")
	}
	return targetAbs, nil
}

func CopyTreeNoLinks(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory")
	}
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is not allowed: %s", path)
		}
		if rel == "." {
			return os.MkdirAll(target, 0700)
		}
		dst := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0700)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0600)
	})
}
