package workspace

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Ghr07h/norns/norns/internal/domain"
)

type Paths struct {
	Root     string
	Norns    string
	Database string
	View     string
	History  string
	Lock     string
	ResetLog string
}

type FileOps interface {
	Rename(oldPath, newPath string) error
	SyncFile(path string) error
	SyncDir(path string) error
	RemoveAll(path string) error
}

type OSFileOps struct{}

func (OSFileOps) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (OSFileOps) RemoveAll(path string) error          { return os.RemoveAll(path) }
func (OSFileOps) SyncFile(path string) error           { return syncFile(path) }
func (OSFileOps) SyncDir(path string) error            { return syncDir(path) }

func Resolve(root string) (Paths, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return Paths{}, domain.Wrap("IO_ERROR", "cannot resolve current directory", err)
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, domain.Wrap("IO_ERROR", "cannot resolve workspace path", err)
	}
	norns := filepath.Join(root, ".norns")
	return Paths{Root: root, Norns: norns, Database: filepath.Join(norns, "norns.sqlite"), View: filepath.Join(norns, "view"), History: filepath.Join(norns, "history"), Lock: filepath.Join(root, ".norns.lock"), ResetLog: filepath.Join(root, ".norns-reset.json")}, nil
}

func busyWithCause(message, resource string, cause error) *domain.Error {
	e := domain.Wrap("WORKSPACE_BUSY", message, cause)
	e.Details["resource"] = resource
	return e
}

func TimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Second)
}

func SyncFile(path string) error {
	return OSFileOps{}.SyncFile(path)
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func SyncDir(path string) error {
	return OSFileOps{}.SyncDir(path)
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
